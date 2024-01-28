package server

import (
	"context"
	"fmt"
	"reflect"

	"github.com/tensorleap/helm-charts/pkg/helm"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"github.com/tensorleap/helm-charts/pkg/version"
)

var (
	ErrCliUpgradeRequired = fmt.Errorf("CLI upgrade required")
	ErrOldManifest        = fmt.Errorf("old manifest")
	ErrReinstallRequired  = fmt.Errorf("reinstall required")
)

func ValidateInstallerVersion(installerVersion string) error {
	currentVersion := version.Version
	if version.IsMinorVersionSmaller(currentVersion, installerVersion) {
		return ErrOldManifest
	}
	if version.IsMinorVersionChange(currentVersion, installerVersion) {
		return ErrCliUpgradeRequired
	}
	return nil
}

func IsNeedsToReinstall(ctx context.Context, mnf, previousMnf *manifest.InstallationManifest, installationParams, previousInstallationParams *InstallationParams) (bool, error) {
	cluster, err := k3d.GetCluster(ctx)
	if err != nil {
		return false, err
	}

	clusterNotExists := cluster == nil
	if clusterNotExists {
		return false, nil
	}

	if previousInstallationParams == nil || previousMnf == nil {
		return true, nil
	}

	// make sure cluster is running before checking if it needs to be reinstalled (for helm checking)
	err = k3d.RunCluster(ctx)
	if err != nil {
		log.SendCloudReport("warning", "Failed to run cluster, try to recreate", "Running",
			&map[string]interface{}{"error": err.Error()})

		return true, nil
	}

	newK3sImage := ""
	currentK3sImage := ""

	if installationParams.IsUseGpu() {
		newK3sImage = mnf.Images.K3sGpu
	} else {
		newK3sImage = mnf.Images.K3sGpu
	}

	if previousInstallationParams.IsUseGpu() {
		currentK3sImage = previousMnf.Images.K3sGpu
	} else {
		currentK3sImage = previousMnf.Images.K3sGpu
	}

	isChartsRequiredReinstall, err := IsHelmRequiredReinstall(ctx, mnf, cluster)
	if err != nil {
		return false, err
	}
	IsK3sImageChange := currentK3sImage != newK3sImage
	isAppVersionChanged := mnf.AppVersion != previousMnf.AppVersion
	isInfraHelmChartParamsChanged := !reflect.DeepEqual(previousInstallationParams.GetInfraHelmValuesParams(), installationParams.GetInfraHelmValuesParams())
	isCreateClusterParamsChanged := !reflect.DeepEqual(previousInstallationParams.GetCreateK3sClusterParams(), installationParams.GetCreateK3sClusterParams())

	shouldReinstall := isChartsRequiredReinstall || IsK3sImageChange || isAppVersionChanged || isInfraHelmChartParamsChanged || isCreateClusterParamsChanged
	if shouldReinstall {
		return true, nil
	}

	return false, nil
}

func IsHelmRequiredReinstall(ctx context.Context, mnf *manifest.InstallationManifest, cluster *k3d.Cluster) (bool, error) {
	kubeConfigPath, clean, err := k3d.CreateTmpClusterKubeConfig(ctx, cluster)
	if err != nil {
		return false, err
	}
	defer clean()

	helmConfig, err := helm.CreateHelmConfig(kubeConfigPath, KUBE_CONTEXT, KUBE_NAMESPACE)
	if err != nil {
		return false, err
	}

	currentInfraVersion, err := helm.GetHelmReleaseVersion(helmConfig, mnf.InfraHelmChart.ReleaseName)
	if err == helm.ErrNoRelease {
		iServerReleaseExists, err := helm.IsHelmReleaseExists(helmConfig, mnf.ServerHelmChart.ReleaseName)
		if err != nil {
			return false, err
		}
		if iServerReleaseExists {
			return true, nil
		}
		return false, nil

	} else if err != nil {
		return false, err
	}
	currentServerVersion, err := helm.GetHelmReleaseVersion(helmConfig, mnf.ServerHelmChart.ReleaseName)
	if err == helm.ErrNoRelease {
		return false, nil
	} else if err != nil {
		return false, err
	}

	isMinorVersionSmaller := version.IsMinorVersionChange(currentServerVersion, mnf.ServerHelmChart.Version) || version.IsMinorVersionSmaller(currentInfraVersion, mnf.InfraHelmChart.Version)
	if isMinorVersionSmaller {
		return false, ErrOldManifest
	}

	isVersionChange := currentInfraVersion != mnf.InfraHelmChart.Version != version.IsMinorVersionChange(currentServerVersion, mnf.ServerHelmChart.Version)
	if isVersionChange {
		return true, nil
	}

	return false, nil
}
