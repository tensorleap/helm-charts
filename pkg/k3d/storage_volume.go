package k3d

import (
	"context"
	"fmt"
	"os"
	"path"
	"runtime"
	"strings"
	"time"

	k3d "github.com/k3d-io/k3d/v5/pkg/types"
	"github.com/tensorleap/helm-charts/pkg/docker"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
)

// STORAGE_VOLUME_NAME is the Docker named volume holding the app data
// (mongodb, elasticsearch, minio, keycloak, job-shared) on Docker Desktop.
const STORAGE_VOLUME_NAME = "tensorleap-storage-volume"

// storageVolumeMountPath is where the volume is mounted inside the k3s node:
// the dir the charts' hostPath PersistentVolumes point at, so the charts need
// no change. It shadows that dir of the host data dir bind.
const storageVolumeMountPath = local.DEFAULT_DATA_DIR + "/" + local.STORAGE_DIR_NAME

// storageVolumeScratchPath is where one-off containers see the volume.
const storageVolumeScratchPath = "/storage"

// UseStorageVolume reports whether app data lives in STORAGE_VOLUME_NAME
// instead of <data-dir>/storage on the host. On Docker Desktop (macOS,
// Windows) a bind-mounted host dir crosses the VM file-sharing bridge: slow
// for database I/O, and the bridge's change notifier stalls under load and
// takes Docker Desktop down ("service fs failed: injecting event blocked for
// 60s"). Linux has no bridge, so the host dir stays.
func UseStorageVolume() bool {
	return runtime.GOOS != "linux"
}

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

// storageSetupScript prepares the volume the way initStandaloneSubDirs
// prepares the host dir: world-writable dirs for the pods that run as
// non-root (elasticsearch, keycloak). With migrate it first copies the host
// data bound at /src, staged so a failed copy leaves no half store behind, and
// only when the volume holds no mongodb yet: a rerun must never overwrite
// live data.
func storageSetupScript(migrate bool) string {
	s := storageVolumeScratchPath
	es := path.Join(s, path.Base(local.ELASTIC_STORAGE_DIR_NAME))
	kc := path.Join(s, path.Base(local.KEYCLOAK_DB_STORAGE_DIR_NAME))
	mongo := path.Join(s, path.Base(local.MONGODB_STORAGE_DIR_NAME))
	staging := path.Join(s, ".migrating")
	script := ""
	if migrate {
		script = fmt.Sprintf("if [ ! -e %[1]s ]; then\n"+
			"  rm -rf %[2]s && mkdir %[2]s\n"+
			"  cp -a /src/. %[2]s/\n"+
			"  chmod -R a+rwX %[2]s\n"+
			"  mv %[2]s/* %[3]s/ && rmdir %[2]s\n"+
			"fi\n", mongo, staging, s)
	}
	return script + fmt.Sprintf("mkdir -p %s %s && chmod a+rwx %s %s %s", es, kc, s, es, kc)
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
		log.Infof("Moving app data from %s into Docker volume %s (one time, may take a while)", hostStorage, STORAGE_VOLUME_NAME)
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
