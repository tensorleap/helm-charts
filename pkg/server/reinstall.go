package server

import (
	"context"

	"github.com/tensorleap/helm-charts/pkg/helm/chart"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

func Reinstall(ctx context.Context, mnf *manifest.InstallationManifest, isAirgap bool, installationParams *InstallationParams, infraChart, serverChart *chart.Chart) (*InstallationResult, error) {
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
