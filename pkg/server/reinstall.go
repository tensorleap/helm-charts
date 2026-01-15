package server

import (
	"context"

	"github.com/tensorleap/helm-charts/pkg/helm/chart"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

func Reinstall(ctx context.Context, mnf *manifest.InstallationManifest, isAirgap bool, installationParams *InstallationParams, infraChart, serverChart *chart.Chart) error {
	err := Uninstall(ctx, false, false)
	if err != nil {
		return err
	}

	err = Install(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	if err != nil {
		return err
	}
	log.SendCloudReport("info", "Successfully completed reinstall", "Success", nil)
	return nil
}
