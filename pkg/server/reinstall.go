package server

import (
	"context"

	"github.com/tensorleap/helm-charts/pkg/helm/chart"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

func Reinstall(ctx context.Context, mnf *manifest.InstallationManifest, isAirgap bool, installationParams *InstallationParams, infraChart, serverChart *chart.Chart) (*InstallationResult, error) {
	// Fail while the old cluster still runs rather than after it is gone.
	if err := k3d.CheckStorageMigrationSpace(ctx); err != nil {
		return nil, err
	}
	err := Uninstall(ctx, false, false, false)
	if err != nil {
		return nil, err
	}

	result, err := Install(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	if err != nil {
		return nil, err
	}
	log.SendCloudReport("info", "Successfully completed reinstall", "Success", nil)
	return result, nil
}
