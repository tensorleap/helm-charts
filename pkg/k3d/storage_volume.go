package k3d

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	k3d "github.com/k3d-io/k3d/v5/pkg/types"
	"github.com/tensorleap/helm-charts/pkg/docker"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
)

// STORAGE_VOLUME_NAME is the Docker named volume holding the app data
// (mongodb, elasticsearch, minio, keycloak, job-shared) instead of
// <data-dir>/storage on the host, on every OS so installs behave the same.
// A bind-mounted host dir crosses Docker Desktop's VM file-sharing bridge on
// macOS and Windows: slow for database I/O, and the bridge's change notifier
// stalls under load and takes Docker Desktop down ("service fs failed:
// injecting event blocked for 60s"). On Linux a volume is a plain dir under
// Docker's data root, same disk and speed as a bind.
const STORAGE_VOLUME_NAME = "tensorleap-storage-volume"

// storageVolumeMountPath is where the volume is mounted inside the k3s node:
// the dir the charts' hostPath PersistentVolumes point at, so the charts need
// no change. It shadows that dir of the host data dir bind.
const storageVolumeMountPath = local.DEFAULT_DATA_DIR + "/" + local.STORAGE_DIR_NAME

// storageVolumeScratchPath is where one-off containers see the volume.
const storageVolumeScratchPath = "/storage"

// storageStagingDir, inside the volume, is where app data lands before it is
// moved into place, so a failed transfer leaves no half store behind.
const storageStagingDir = ".migrating"

// StorageVolumeSpec is the k3d volume mapping for the server node.
func StorageVolumeSpec() string {
	return STORAGE_VOLUME_NAME + ":" + storageVolumeMountPath
}

func volumeBinds() []string {
	return []string{STORAGE_VOLUME_NAME + ":" + storageVolumeScratchPath}
}

// ClusterHasVolume reports whether a server node of cluster mounts the named
// volume. k3d lists node volumes as docker binds ("source:target[:opts]").
func ClusterHasVolume(cluster *Cluster, name string) bool {
	for _, node := range cluster.Nodes {
		if node.Role != k3d.ServerRole {
			continue
		}
		for _, v := range node.Volumes {
			if strings.HasPrefix(v, name+":") {
				return true
			}
		}
	}
	return false
}

func serverNodeImage(cluster *Cluster) string {
	for _, node := range cluster.Nodes {
		if node.Role == k3d.ServerRole {
			return node.Image
		}
	}
	return ""
}

func RemoveStorageVolume(ctx context.Context) error {
	return docker.RemoveVolume(ctx, STORAGE_VOLUME_NAME, true)
}

// RemoveStorageVolumeSubDirs deletes the given stores (e.g. "mongodb") inside
// the storage volume. image is any local image with a shell, e.g. the k3s one.
func RemoveStorageVolumeSubDirs(ctx context.Context, image string, subDirs []string) error {
	paths := make([]string, 0, len(subDirs))
	for _, d := range subDirs {
		paths = append(paths, path.Join(storageVolumeScratchPath, d))
	}
	if err := docker.RunOnce(ctx, image, volumeBinds(), "rm -rf "+strings.Join(paths, " ")); err != nil {
		return fmt.Errorf("remove %v from volume %s: %w", subDirs, STORAGE_VOLUME_NAME, err)
	}
	return nil
}

// storageSetupScript prepares the volume the way initStandaloneSubDirs
// prepares the host dir: world-writable dirs for the pods that run as
// non-root (elasticsearch, keycloak). With migrate it first moves the staged
// app data into place.
func storageSetupScript(migrate bool) string {
	s := storageVolumeScratchPath
	es := path.Join(s, path.Base(local.ELASTIC_STORAGE_DIR_NAME))
	kc := path.Join(s, path.Base(local.KEYCLOAK_DB_STORAGE_DIR_NAME))
	staging := path.Join(s, storageStagingDir)
	script := ""
	if migrate {
		script = fmt.Sprintf("chmod -R a+rwX %[1]s\nmv %[1]s/* %[2]s/ && rmdir %[1]s\n", staging, s)
	}
	return script + fmt.Sprintf("mkdir -p %s %s && chmod a+rwx %s %s %s", es, kc, s, es, kc)
}

// volumeHasData reports whether the volume already holds a mongodb store:
// app data a rerun must never overwrite.
func volumeHasData(ctx context.Context, image string) (bool, error) {
	mongo := path.Join(storageVolumeScratchPath, path.Base(local.MONGODB_STORAGE_DIR_NAME))
	out, err := docker.RunOnceOutput(ctx, image, volumeBinds(), fmt.Sprintf("[ -e %s ] && echo yes || echo no", mongo))
	return strings.TrimSpace(out) == "yes", err
}

// volumeFreeKB is the free space of the disk holding the volume, measured
// where the daemon runs: Docker Desktop's VM disk, a Linux engine's data root.
func volumeFreeKB(ctx context.Context, image string) (int64, error) {
	out, err := docker.RunOnceOutput(ctx, image, volumeBinds(), "df -Pk "+storageVolumeScratchPath+" | tail -1 | tr -s ' ' | cut -d' ' -f4")
	if err != nil {
		return 0, err
	}
	free, err := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse free space of volume %s %q: %w", STORAGE_VOLUME_NAME, out, err)
	}
	return free, nil
}

// storageMigration is app data still in the host data dir, from an install
// that predates the volume.
type storageMigration struct {
	hostStorage string
	// hostMountpoint is the volume's directory when this machine can reach it
	// (a native Linux engine), "" when only the daemon can (Docker Desktop's
	// VM, a remote engine).
	hostMountpoint string
}

// planStorageMigration returns nil when the host dir holds no app data or the
// volume already holds some.
func planStorageMigration(ctx context.Context, image string) (*storageMigration, error) {
	hasHost, err := local.StorageHasData()
	if err != nil || !hasHost {
		return nil, err
	}
	hasVolume, err := volumeHasData(ctx, image)
	if err != nil || hasVolume {
		return nil, err
	}
	m := &storageMigration{hostStorage: path.Join(local.GetServerDataDir(), local.STORAGE_DIR_NAME)}
	m.hostMountpoint, err = hostVolumeMountpoint(ctx, image)
	return m, err
}

// hostVolumeMountpoint returns the volume's directory on this machine, or ""
// when the path the daemon reports is not the volume here: a probe dir made
// through the daemon has to show up at that path. It does not on Docker
// Desktop, whose daemon runs in a VM, even when the path exists on the host.
func hostVolumeMountpoint(ctx context.Context, image string) (string, error) {
	mountpoint, err := docker.VolumeMountpoint(ctx, STORAGE_VOLUME_NAME)
	if err != nil {
		return "", err
	}
	probe := path.Join(storageVolumeScratchPath, ".probe")
	if err := docker.RunOnce(ctx, image, volumeBinds(), "mkdir -p "+probe); err != nil {
		return "", err
	}
	defer func() { _ = docker.RunOnce(ctx, image, volumeBinds(), "rmdir "+probe) }()
	status, err := local.CheckDirectoryStatus(filepath.Join(mountpoint, ".probe"))
	if err != nil || !status.Exists {
		return "", nil
	}
	return mountpoint, nil
}

// checkStorageMigrationSpace fails when the streamed copy would not fit: it
// needs the data's size free on the daemon's disk and, since Docker Desktop's
// VM disk is a sparse file growing on the host disk, on the host disk holding
// the data dir. A move through the host mountpoint needs no room (a rename),
// and its cross-filesystem fallback copy fails with the source intact.
func checkStorageMigrationSpace(ctx context.Context, image string, m *storageMigration) error {
	if m.hostMountpoint != "" {
		return nil
	}
	size, err := local.DirSize(m.hostStorage)
	if err != nil {
		return fmt.Errorf("measure %s: %w", m.hostStorage, err)
	}
	needKB := size/1024 + 1
	free, err := volumeFreeKB(ctx, image)
	if err != nil {
		return err
	}
	if hostFree, err := local.FreeDiskKB(m.hostStorage); err != nil {
		log.Warnf("Could not read free space of the disk holding %s: %v", m.hostStorage, err)
	} else if hostFree < free {
		free = hostFree
	}
	if free < needKB {
		return fmt.Errorf("the app data in %s (%d GB) needs that much free space on Docker's disk and on the host disk holding it, only %d GB is free: raise Docker Desktop's disk limit or free disk space", m.hostStorage, needKB>>20, free>>20)
	}
	return nil
}

// CheckStorageMigrationSpace fails, before an upgrade removes the cluster,
// when app data in the host dir would not fit in Docker's disk: the transfer
// in setupStorageVolume would otherwise fail with the old cluster already
// gone. It runs in the current cluster's image, so it is a no-op without one.
func CheckStorageMigrationSpace(ctx context.Context) error {
	cluster, err := GetCluster(ctx)
	if err != nil || cluster == nil {
		return err
	}
	image := serverNodeImage(cluster)
	if image == "" {
		return nil
	}
	if _, err := docker.CreateVolumeIfNotExists(ctx, STORAGE_VOLUME_NAME, nil); err != nil {
		return err
	}
	m, err := planStorageMigration(ctx, image)
	if err != nil || m == nil {
		return err
	}
	return checkStorageMigrationSpace(ctx, image, m)
}

// setupStorageVolume runs once per cluster creation, before any pod exists:
// it prepares the volume and, when the host data dir still holds app data
// (an install from before the volume existed), moves it in.
func setupStorageVolume(ctx context.Context, image string) error {
	m, err := planStorageMigration(ctx, image)
	if err != nil {
		return err
	}
	if m == nil {
		return docker.RunOnce(ctx, image, volumeBinds(), storageSetupScript(false))
	}
	if err := checkStorageMigrationSpace(ctx, image, m); err != nil {
		return err
	}
	// leftovers of an earlier failed transfer
	if err := docker.RunOnce(ctx, image, volumeBinds(), "rm -rf "+path.Join(storageVolumeScratchPath, storageStagingDir)); err != nil {
		return err
	}
	if m.hostMountpoint != "" {
		err = moveStorageIntoVolume(m)
	} else {
		err = streamStorageIntoVolume(ctx, image, m)
	}
	if err != nil {
		return err
	}
	if err := docker.RunOnce(ctx, image, volumeBinds(), storageSetupScript(true)); err != nil {
		return fmt.Errorf("finish moving app data into volume %s: %w", STORAGE_VOLUME_NAME, err)
	}
	if m.hostMountpoint != "" {
		log.Infof("App data is in Docker volume %s now", STORAGE_VOLUME_NAME)
		return nil
	}
	keepMigratedCopy(m.hostStorage)
	return nil
}

// moveStorageIntoVolume renames the host dir into the volume's staging dir: no
// copy and no extra space when both sit on one filesystem (sudo when Docker's
// data root is root-only). The host dir is gone afterwards; the data stays in
// the volume across uninstalls that keep data.
func moveStorageIntoVolume(m *storageMigration) error {
	log.Infof("Moving app data from %s into Docker volume %s at %s", m.hostStorage, STORAGE_VOLUME_NAME, m.hostMountpoint)
	src, err := local.CheckDirectoryStatus(m.hostStorage)
	if err != nil {
		return err
	}
	dst, err := local.CheckDirectoryStatus(filepath.Join(m.hostMountpoint, storageStagingDir))
	if err != nil {
		return err
	}
	if err := local.MoveOrCopyDirectory(src, dst); err != nil {
		return fmt.Errorf("move app data into volume %s: %w", STORAGE_VOLUME_NAME, err)
	}
	return nil
}

// streamStorageIntoVolume copies the host dir into the volume's staging dir as
// a tar stream through the Docker API (what docker cp does): this side reads
// its own disk, the daemon writes its own, and Docker Desktop's file-sharing
// bridge is never in the path.
func streamStorageIntoVolume(ctx context.Context, image string, m *storageMigration) error {
	log.Infof("Copying app data from %s into Docker volume %s through the Docker API (one time, may take a while)", m.hostStorage, STORAGE_VOLUME_NAME)
	pr, pw := io.Pipe()
	readErr := make(chan error, 1)
	go func() {
		var logged int64
		err := local.TarDir(m.hostStorage, storageStagingDir, pw, func(written int64) {
			if written-logged >= 10<<30 {
				logged = written
				log.Infof("Copied %d GB so far", written>>30)
			}
		})
		pw.CloseWithError(err)
		readErr <- err
	}()
	err := docker.ExtractArchive(ctx, image, volumeBinds(), storageVolumeScratchPath, pr)
	_ = pr.CloseWithError(err) // frees the reader goroutine if the daemon stopped early
	if rerr := <-readErr; rerr != nil && err == nil {
		err = fmt.Errorf("read %s: %w", m.hostStorage, rerr)
	}
	if err != nil {
		return fmt.Errorf("copy app data into volume %s: %w", STORAGE_VOLUME_NAME, err)
	}
	return nil
}

// keepMigratedCopy moves the host dir aside as storage.migrated: nothing is
// lost if the new cluster disappoints, and uninstall --purge removes it.
func keepMigratedCopy(hostStorage string) {
	backup := local.MigratedStoragePath()
	if _, err := os.Stat(backup); err == nil {
		backup += "-" + time.Now().Format("20060102-150405")
	}
	if err := os.Rename(hostStorage, backup); err != nil {
		log.Warnf("App data is in Docker volume %s now, but the old copy could not be moved aside (%v); delete %s by hand", STORAGE_VOLUME_NAME, err, hostStorage)
		return
	}
	log.Infof("App data is in Docker volume %s now; the old copy is kept at %s, delete it once the upgrade checks out", STORAGE_VOLUME_NAME, backup)
}
