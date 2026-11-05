package k3d

import (
	"context"
	"fmt"
	"os"
	"path"
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

// StorageVolumeSpec is the k3d volume mapping for the server node.
func StorageVolumeSpec() string {
	return STORAGE_VOLUME_NAME + ":" + storageVolumeMountPath
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
	binds := []string{STORAGE_VOLUME_NAME + ":" + storageVolumeScratchPath}
	if err := docker.RunOnce(ctx, image, binds, "rm -rf "+strings.Join(paths, " ")); err != nil {
		return fmt.Errorf("remove %v from volume %s: %w", subDirs, STORAGE_VOLUME_NAME, err)
	}
	return nil
}

// storageSpaceGuard exits 1 when the host data bound at /src would not fit
// in the free space of the volume's disk (du, df, tail, tr and cut are
// busybox applets the k3s image ships).
const storageSpaceGuard = "need=$(du -sk /src | cut -f1)\n" +
	"free=$(df -Pk " + storageVolumeScratchPath + " | tail -1 | tr -s ' ' | cut -d' ' -f4)\n" +
	"if [ \"$free\" -lt \"$need\" ]; then\n" +
	"  echo \"the app data (${need}K) does not fit in the free space of Docker's disk (${free}K): raise Docker Desktop's disk limit or free space under Docker's data root\"\n" +
	"  exit 1\n" +
	"fi\n"

func volumeMongoPath() string {
	return path.Join(storageVolumeScratchPath, path.Base(local.MONGODB_STORAGE_DIR_NAME))
}

// storageSpaceCheckScript is the guard alone, a no-op once the volume holds
// the data (a rerun after a finished migration).
func storageSpaceCheckScript() string {
	return fmt.Sprintf("if [ -e %s ]; then exit 0; fi\n", volumeMongoPath()) + storageSpaceGuard
}

// storageSetupScript prepares the volume the way initStandaloneSubDirs
// prepares the host dir: world-writable dirs for the pods that run as
// non-root (elasticsearch, keycloak). With migrate it first copies the host
// data bound at /src, after checking it fits, staged so a failed copy leaves
// no half store behind, and only when the volume holds no mongodb yet: a
// rerun must never overwrite live data.
func storageSetupScript(migrate bool) string {
	s := storageVolumeScratchPath
	es := path.Join(s, path.Base(local.ELASTIC_STORAGE_DIR_NAME))
	kc := path.Join(s, path.Base(local.KEYCLOAK_DB_STORAGE_DIR_NAME))
	staging := path.Join(s, ".migrating")
	script := ""
	if migrate {
		script = fmt.Sprintf("if [ ! -e %s ]; then\n", volumeMongoPath()) +
			fmt.Sprintf("rm -rf %s\n", staging) + // a previous failed copy, dropped before measuring
			storageSpaceGuard +
			fmt.Sprintf("mkdir %[1]s\n"+
				"cp -a /src/. %[1]s/\n"+
				"chmod -R a+rwX %[1]s\n"+
				"mv %[1]s/* %[2]s/ && rmdir %[1]s\n"+
				"fi\n", staging, s)
	}
	return script + fmt.Sprintf("mkdir -p %s %s && chmod a+rwx %s %s %s", es, kc, s, es, kc)
}

// CheckStorageMigrationSpace fails, before an upgrade removes the cluster,
// when the host app data would not fit in Docker's disk: the copy in
// setupStorageVolume would otherwise fail with the old cluster already gone.
// It runs in the current cluster's image, so it is a no-op without a cluster
// or without host data to move.
func CheckStorageMigrationSpace(ctx context.Context) error {
	migrate, err := local.StorageHasData()
	if err != nil || !migrate {
		return err
	}
	cluster, err := GetCluster(ctx)
	if err != nil || cluster == nil {
		return err
	}
	image := ""
	for _, node := range cluster.Nodes {
		if node.Role == k3d.ServerRole {
			image = node.Image
		}
	}
	if image == "" {
		return nil
	}
	hostStorage := path.Join(local.GetServerDataDir(), local.STORAGE_DIR_NAME)
	binds := []string{STORAGE_VOLUME_NAME + ":" + storageVolumeScratchPath, hostStorage + ":/src:ro"}
	if err := docker.RunOnce(ctx, image, binds, storageSpaceCheckScript()); err != nil {
		return fmt.Errorf("app data in %s cannot move into Docker volume %s: %w", hostStorage, STORAGE_VOLUME_NAME, err)
	}
	return nil
}

// setupStorageVolume runs once per cluster creation, before any pod exists:
// it prepares the volume and, when the host data dir still holds app data
// (an install from before the volume existed), moves it in and keeps the host
// copy as storage.migrated.
func setupStorageVolume(ctx context.Context, image string) error {
	hostStorage := path.Join(local.GetServerDataDir(), local.STORAGE_DIR_NAME)
	migrate, err := local.StorageHasData()
	if err != nil {
		return err
	}
	binds := []string{STORAGE_VOLUME_NAME + ":" + storageVolumeScratchPath}
	if migrate {
		log.Infof("Moving app data from %s into Docker volume %s (one time, may take a while; needs that much free space on Docker's disk)", hostStorage, STORAGE_VOLUME_NAME)
		binds = append(binds, hostStorage+":/src:ro")
	}
	if err := docker.RunOnce(ctx, image, binds, storageSetupScript(migrate)); err != nil {
		return fmt.Errorf("prepare storage volume %s: %w", STORAGE_VOLUME_NAME, err)
	}
	if !migrate {
		return nil
	}

	backup := local.MigratedStoragePath()
	if _, err := os.Stat(backup); err == nil {
		backup += "-" + time.Now().Format("20060102-150405")
	}
	if err := os.Rename(hostStorage, backup); err != nil {
		log.Warnf("App data is in Docker volume %s now, but the old copy could not be moved aside (%v); delete %s by hand", STORAGE_VOLUME_NAME, err, hostStorage)
		return nil
	}
	log.Infof("App data is in Docker volume %s now; the old copy is kept at %s, delete it once the upgrade checks out", STORAGE_VOLUME_NAME, backup)
	return nil
}
