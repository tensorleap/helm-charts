package server

import (
	"context"

	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
)

func Uninstall(ctx context.Context, purge bool) (err error) {
	err = k3d.UninstallCluster(ctx)
	if err != nil {
		return err
	}

	err = k3d.UninstallRegister()
	if err != nil {
		return err
	}

	if purge {
		err = local.PurgeData()
		if err != nil {
			return err
		}
	}
	return nil
}
