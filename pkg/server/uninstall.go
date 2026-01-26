package server

import (
	"context"

	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
)

func Uninstall(ctx context.Context, purge bool, cleanup bool, clearData bool) (err error) {
	err = k3d.UninstallCluster(ctx)
	if err != nil {
		return err
	}

	err = k3d.UninstallRegister()
	if err != nil {
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
	} else if cleanup {
		// Remove only cache, keep data
		err = local.CleanupCacheData()
		if err != nil {
			return err
		}
	}
	return nil
}
