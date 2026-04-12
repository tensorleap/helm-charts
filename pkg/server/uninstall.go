package server

import (
	"context"

	"github.com/tensorleap/helm-charts/pkg/docker"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
)

const legacySidecarRegistryName = "k3d-tensorleap-registry"

func Uninstall(ctx context.Context, purge bool, cleanup bool, clearData bool) (err error) {
	err = k3d.UninstallCluster(ctx)
	if err != nil {
		return err
	}

	// Best-effort cleanup of legacy k3d sidecar registry container (pre-Zot installs)
	if rmErr := docker.TryRemoveContainer(ctx, legacySidecarRegistryName); rmErr != nil {
		log.Warnf("Failed to remove legacy registry container: %v", rmErr)
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
	} else if cleanup {
		// Remove only cache, keep data
		err = local.CleanupCacheData()
		if err != nil {
			return err
		}
	}
	return nil
}
