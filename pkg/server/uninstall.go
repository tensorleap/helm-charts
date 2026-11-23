package server

import (
	"context"
	"fmt"
	"path"

	"github.com/tensorleap/helm-charts/pkg/docker"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
)

const legacySidecarRegistryName = "k3d-tensorleap-registry"

// CustomTarget identifies a single piece of data the custom uninstall can
// delete. A normal uninstall (cluster removal) always runs regardless; these
// are the extra opt-in deletions.
type CustomTarget string

const (
	TargetMongo      CustomTarget = "mongodb"
	TargetMinio      CustomTarget = "minio"
	TargetElastic    CustomTarget = "elasticsearch"
	TargetKeycloak   CustomTarget = "keycloak"
	TargetAllAppData CustomTarget = "all-app-data"
	TargetManifests  CustomTarget = "manifests"
	TargetHostname   CustomTarget = "hostname"
	TargetImageCache CustomTarget = "image-cache"
	TargetRegistry   CustomTarget = "registry"
	TargetHelmCache  CustomTarget = "helm-cache"
)

// removeClusterAndLegacySidecar performs the baseline uninstall shared by every
// mode: delete the k3d cluster, then best-effort remove the legacy pre-Zot
// sidecar registry container.
func removeClusterAndLegacySidecar(ctx context.Context) error {
	if err := k3d.UninstallCluster(ctx); err != nil {
		return err
	}
	if rmErr := docker.TryRemoveContainer(ctx, legacySidecarRegistryName); rmErr != nil {
		log.Warnf("Failed to remove legacy registry container: %v", rmErr)
	}
	return nil
}

// UninstallCustom runs a normal uninstall (removing the cluster) and then
// deletes only the extra data identified by targets. Deletions are best-effort:
// a failure is logged and the rest still run; the first error is returned so the
// command exits non-zero.
func UninstallCustom(ctx context.Context, targets []CustomTarget) error {
	if err := removeClusterAndLegacySidecar(ctx); err != nil {
		return err
	}

	selected := make(map[CustomTarget]bool, len(targets))
	for _, t := range targets {
		selected[t] = true
	}

	var firstErr error
	remove := func(subDir string) {
		if err := local.RemoveDataSubDir(subDir); err != nil {
			log.Warnf("Failed to remove %s: %v", subDir, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	// Application data. The "all" rollup supersedes the per-store selections.
	// The stores live in the storage volume; the host dir is still cleared for
	// installs from before the volume existed.
	if selected[TargetAllAppData] {
		remove(local.STORAGE_DIR_NAME)
		if err := k3d.RemoveStorageVolume(ctx); err != nil {
			log.Warnf("Failed to remove storage volume: %v", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	} else {
		var stores []string
		for _, store := range []struct {
			target CustomTarget
			subDir string
		}{
			{TargetMongo, local.MONGODB_STORAGE_DIR_NAME},
			{TargetMinio, local.MINIO_STORAGE_DIR_NAME},
			{TargetElastic, local.ELASTIC_STORAGE_DIR_NAME},
			{TargetKeycloak, local.KEYCLOAK_DB_STORAGE_DIR_NAME},
		} {
			if selected[store.target] {
				remove(store.subDir)
				stores = append(stores, path.Base(store.subDir))
			}
		}
		if len(stores) > 0 {
			if err := removeStoresFromVolume(ctx, stores); err != nil {
				log.Warnf("Failed to remove %v from storage volume: %v", stores, err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}

	// Installation config.
	if selected[TargetManifests] {
		remove(local.MANIFEST_DIR_NAME)
	}
	if selected[TargetHostname] {
		remove(local.HOSTNAME_FILE)
	}

	// Cache. The container image cache lives as a local dir or a Docker volume
	// depending on the platform, so clear both forms.
	if selected[TargetImageCache] {
		remove(local.CONTAINERD_DIR_NAME)
		if err := k3d.RemoveImageCachingVolume(ctx); err != nil {
			log.Warnf("Failed to remove image caching volume: %v", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if selected[TargetRegistry] {
		remove(local.REGISTRY_DIR_NAME)
	}
	if selected[TargetHelmCache] {
		remove(local.HELM_CACHE_DIR_NAME)
	}

	return firstErr
}

func Uninstall(ctx context.Context, purge bool, cleanup bool, clearData bool) (err error) {
	if err = removeClusterAndLegacySidecar(ctx); err != nil {
		return err
	}

	if cleanup || purge {
		err = k3d.RemoveImageCachingVolume(ctx)
		if err != nil {
			return err
		}
	}

	if purge {
		// Remove everything: data + cache
		err = local.PurgeData()
		if err != nil {
			return err
		}
	} else if clearData {
		// Remove only application data, keep cache
		err = local.ClearAppData()
		if err != nil {
			return err
		}
	}
	if purge || clearData {
		if err = k3d.RemoveStorageVolume(ctx); err != nil {
			return err
		}
	}
	if cleanup && !purge && !clearData {
		// Remove only cache, keep data
		err = local.CleanupCacheData()
		if err != nil {
			return err
		}
	}
	return nil
}

// removeStoresFromVolume deletes single stores inside the storage volume. The
// cluster is gone by now, so a throwaway container of the installed k3s image
// does the work; the install manifest names that image.
func removeStoresFromVolume(ctx context.Context, stores []string) error {
	mnf, err := LoadPreviousManifest()
	if err != nil {
		return fmt.Errorf("load install manifest to pick an image for cleaning volume %s: %w", k3d.STORAGE_VOLUME_NAME, err)
	}
	image := mnf.Images.K3s
	if prvParams, _ := LoadInstallationParamsFromPrevious(); prvParams != nil && prvParams.IsUseGpu() {
		image = mnf.Images.K3sGpu
	}
	return k3d.RemoveStorageVolumeSubDirs(ctx, image, stores)
}
