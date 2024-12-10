package server

import (
	"context"

	"github.com/tensorleap/helm-charts/pkg/helm"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"helm.sh/helm/v3/pkg/chart"
)

func Install(ctx context.Context, mnf *manifest.InstallationManifest, isAirgap bool, installationParams *InstallationParams, infraChart, serverChart *chart.Chart) error {

	prvInstallationParams, _ := LoadInstallationParamsFromPrevious()
	prvMnf, err := LoadPreviousManifest()
	if err != nil && err != manifest.ErrManifestNotFound {
		return err
	}

	shouldReinstall, err := IsNeedsToReinstall(ctx, mnf, prvMnf, installationParams, prvInstallationParams)
	if err != nil {
		return err
	}

	if shouldReinstall {
		return SafetyReinstall(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	}

	if installationParams.FixK3dDns {
		k3d.FixDockerDns()
	}

	registry, err := k3d.CreateLocalRegistry(ctx, mnf.Images.Register, installationParams.GetCreateRegistryParams())
	if err != nil {
		return err
	}

	registryPortStr, err := k3d.GetRegistryPort(ctx, registry)
	if err != nil {
		return err
	}

	imagesToCache := CalcWhichImagesToCache(mnf, installationParams.IsUseGpu(), isAirgap)

	err = k3d.CacheImagesInParallel(ctx, imagesToCache, registryPortStr, isAirgap)
	if err != nil {
		return err
	}

	_, _, err = InitCluster(
		ctx,
		mnf,
		prvMnf,
		installationParams,
		prvInstallationParams,
	)
	if err != nil {
		return err
	}

	if err := InstallCharts(ctx, mnf, installationParams, infraChart, serverChart); err != nil {
		return err
	}

	_ = SaveInstallation(mnf, installationParams)

	err = cleanImages(mnf, prvMnf, installationParams.ClearInstallationImages)
	if err != nil {
		log.SendCloudReport("error", "Failed cleaning images", "Failed", &map[string]interface{}{"error": err.Error()})
		log.Warnf("Failed cleaning images: %v", err)
	}

	return nil
}

func InitCluster(ctx context.Context, mnf, previousMnf *manifest.InstallationManifest, installationParams, previousInstallationParams *InstallationParams) (cluster *k3d.Cluster, createNew bool, err error) {
	cluster, err = k3d.GetCluster(ctx)
	if err != nil {
		return
	}

	clusterNotExists := cluster == nil
	if clusterNotExists {
		cluster, err = k3d.CreateCluster(ctx, mnf, installationParams.GetCreateK3sClusterParams())
		if err != nil {
			createNew = true
		}
		return
	}

	log.Info("Cluster already exists, skipping creation")
	return
}

func InstallCharts(ctx context.Context, mnf *manifest.InstallationManifest, installationParams *InstallationParams, infraChart, serverChart *chart.Chart) error {
	log.SendCloudReport("info", "Installing helm", "Running", nil)

	cluster, err := k3d.GetCluster(ctx)
	if err != nil {
		return err
	}

	kubeConfigPath, clean, err := k3d.CreateTmpClusterKubeConfig(ctx, cluster)
	if err != nil {
		return err
	}
	defer clean()

	helmConfig, err := helm.CreateHelmConfig(kubeConfigPath, KUBE_CONTEXT, KUBE_NAMESPACE)
	if err != nil {
		log.SendCloudReport("error", "Failed creating helm config", "Failed",
			&map[string]interface{}{"kubeContext": KUBE_CONTEXT, "kubeNamespace": KUBE_NAMESPACE, "error": err.Error()})
		return err
	}
	infraChartMeta := mnf.InfraHelmChart
	isInfraReleaseExisted, err := helm.IsHelmReleaseExists(helmConfig, infraChartMeta.ReleaseName)
	if err != nil {
		log.SendCloudReport("error", "Failed checking if helm release exists", "Failed",
			&map[string]interface{}{"helmConfig": helmConfig, "error": err.Error()})
		return err
	}
	if !isInfraReleaseExisted {
		log.SendCloudReport("info", "Setting up infra helm repo", "Running", &map[string]interface{}{"version": infraChartMeta.Version})
		infraValues := helm.CreateInfraChartValues(installationParams.GetInfraHelmValuesParams())
		if err := helm.InstallChart(
			helmConfig,
			infraChartMeta.ReleaseName,
			infraChart,
			infraValues,
		); err != nil {
			log.SendCloudReport("error", "Failed installing latest chart versions", "Failed",
				&map[string]interface{}{"version": infraChartMeta.Version, "error": err.Error()})
			return err
		}
	}

	serverChartMeta := mnf.ServerHelmChart
	isServerReleaseExisted, err := helm.IsHelmReleaseExists(helmConfig, serverChartMeta.ReleaseName)
	if err != nil {
		log.SendCloudReport("error", "Failed checking if helm release exists", "Failed",
			&map[string]interface{}{"version": serverChartMeta.Version, "error": err.Error()})
		return err
	}
	serverValues, err := helm.CreateTensorleapChartValues(installationParams.GetServerHelmValuesParams())
	if err != nil {
		log.SendCloudReport("error", "Failed to create chart values", "Failed",
			&map[string]interface{}{"version": serverChartMeta.Version, "error": err.Error()})
		return err
	}
	if isServerReleaseExisted {
		log.SendCloudReport("info", "Running helm upgrade", "Running", &map[string]interface{}{"version": serverChartMeta.Version})
		if err := helm.UpgradeChart(
			helmConfig,
			serverChartMeta.ReleaseName,
			serverChart,
			serverValues,
		); err != nil {
			log.SendCloudReport("error", "Failed upgrading helm latest charts versions", "Failed",
				&map[string]interface{}{"version": serverChartMeta.Version, "error": err.Error()})
			return err
		}
	} else {
		log.SendCloudReport("info", "Setting up server helm repo", "Running", &map[string]interface{}{"version": serverChartMeta.Version})
		if err := helm.InstallChart(
			helmConfig,
			serverChartMeta.ReleaseName,
			serverChart,
			serverValues,
		); err != nil {
			log.SendCloudReport("error", "Failed installing latest server chart versions", "Failed",
				&map[string]interface{}{"version": serverChartMeta.Version, "error": err.Error()})
			return err
		}
	}

	log.SendCloudReport("info", "Successfully installed helm charts", "Running", nil)
	log.Info("Tensorleap installed on local k3d cluster")
	return nil
}
