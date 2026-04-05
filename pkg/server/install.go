package server

import (
	"context"
	"fmt"
	"strconv"

	"github.com/tensorleap/helm-charts/pkg/helm"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"helm.sh/helm/v3/pkg/chart"
)

func Install(ctx context.Context, mnf *manifest.InstallationManifest, isAirgap bool, installationParams *InstallationParams, infraChart, serverChart *chart.Chart) (*InstallationResult, error) {

	prvInstallationParams, _ := LoadInstallationParamsFromPrevious()
	prvMnf, err := LoadPreviousManifest()
	if err != nil && err != manifest.ErrManifestNotFound {
		return nil, err
	}

	k3d.FixDockerDns()

	cluster, _, err := InitCluster(
		ctx,
		mnf,
		prvMnf,
		installationParams,
		prvInstallationParams,
	)
	if err != nil {
		return nil, err
	}

	regPortStr := strconv.FormatUint(uint64(installationParams.RegistryPort), 10)

	if isAirgap {
		if err := airgapBootstrap(ctx, mnf, installationParams, cluster, infraChart, regPortStr); err != nil {
			return nil, err
		}
	}

	if err := InstallCharts(ctx, mnf, installationParams, infraChart, serverChart); err != nil {
		return nil, err
	}

	_ = SaveInstallation(mnf, installationParams)
	err = cleanImagesFromContainerd(ctx, mnf, k3d.CONTAINER_NAME)
	if err != nil {
		log.SendCloudReport("error", "Failed cleaning images from containerd", "Failed", &map[string]interface{}{"error": err.Error()})
		log.Warnf("Failed cleaning images from containerd: %v", err)
	}

	if err := cleanImagesFromZot(installationParams.RegistryPort, mnf); err != nil {
		log.SendCloudReport("error", "Failed cleaning images from Zot", "Failed", &map[string]interface{}{"error": err.Error()})
		log.Warnf("Failed cleaning images from Zot registry: %v", err)
	}

	return installationParams.GetInstallationResult(), nil
}

// airgapBootstrap handles the two-phase airgap install:
// Phase 1: Import bootstrap images (k3s system + Zot) into containerd via k3d image import.
//
//	Install infra chart so Zot starts. Wait for Zot readiness.
//
// Phase 2: Push ALL application images from host Docker into Zot.
func airgapBootstrap(ctx context.Context, mnf *manifest.InstallationManifest, installationParams *InstallationParams, cluster *k3d.Cluster, infraChart *chart.Chart, regPortStr string) error {
	log.Info("Airgap mode: starting two-phase bootstrap...")

	// Phase 1: Import Zot + k3s system images into containerd
	bootstrapImages := getBootstrapImages(mnf, installationParams.IsUseGpu())
	if err := k3d.ImportImagesIntoCluster(ctx, cluster, bootstrapImages); err != nil {
		return fmt.Errorf("failed to import bootstrap images into containerd: %w", err)
	}

	// Install infra chart so Zot starts
	if err := installInfraChart(ctx, mnf, installationParams, infraChart); err != nil {
		return fmt.Errorf("failed to install infra chart during airgap bootstrap: %w", err)
	}

	// Wait for Zot to become ready
	log.Info("Waiting for in-cluster Zot registry to be ready...")
	if err := k3d.WaitForRegistry(ctx, regPortStr); err != nil {
		return fmt.Errorf("zot registry did not become ready: %w", err)
	}
	log.Info("Zot registry is ready")

	// Phase 2: Push all application images into Zot
	imagesToCache := CalcWhichImagesToCache(mnf, installationParams.IsUseGpu(), true)
	if len(imagesToCache) > 0 {
		log.Infof("Pushing %d images into Zot registry...", len(imagesToCache))
		if err := k3d.CacheImagesInParallel(ctx, imagesToCache, regPortStr, true, ""); err != nil {
			return fmt.Errorf("failed to push images into Zot: %w", err)
		}
	}

	log.Info("Airgap bootstrap complete")
	return nil
}

// getBootstrapImages returns the minimal set of images that must be imported
// into containerd via k3d image-import before any in-cluster registry exists.
// k3s system images (coredns, klipper-lb, etc.) are embedded in the k3s binary
// and auto-loaded on startup, so only the Zot registry image is needed here.
func getBootstrapImages(mnf *manifest.InstallationManifest, useGpu bool) []string {
	images := []string{}
	if mnf.Images.Zot != "" {
		images = append(images, mnf.Images.Zot)
	}
	return images
}

// installInfraChart installs the infrastructure Helm chart (which deploys Zot).
func installInfraChart(ctx context.Context, mnf *manifest.InstallationManifest, installationParams *InstallationParams, infraChart *chart.Chart) error {
	clusterObj, err := k3d.GetCluster(ctx)
	if err != nil {
		return err
	}

	kubeConfigPath, clean, err := k3d.CreateTmpClusterKubeConfig(ctx, clusterObj)
	if err != nil {
		return err
	}
	defer clean()

	helmConfig, err := helm.CreateHelmConfig(kubeConfigPath, KUBE_CONTEXT, KUBE_NAMESPACE)
	if err != nil {
		return err
	}

	infraChartMeta := mnf.InfraHelmChart
	isInfraReleaseExisted, err := helm.IsHelmReleaseExists(helmConfig, infraChartMeta.ReleaseName)
	if err != nil {
		return err
	}
	if !isInfraReleaseExisted {
		var syncRegistries []helm.ZotSyncRegistry
		if installationParams.IsAirgap {
			syncRegistries = k3d.BuildZotSyncRegistries(mnf)
		}
		infraValues := helm.CreateInfraChartValues(installationParams.GetInfraHelmValuesParams(syncRegistries, mnf.Images.Zot))
		if err := helm.InstallChart(helmConfig, infraChartMeta.ReleaseName, infraChart, infraValues); err != nil {
			return err
		}
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
		cluster, err = k3d.CreateCluster(ctx, mnf, installationParams.GetCreateK3sClusterParams(), local.GetContainerdDataDir())
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
		var syncRegistries []helm.ZotSyncRegistry
		if installationParams.IsAirgap {
			syncRegistries = k3d.BuildZotSyncRegistries(mnf)
		}
		infraValues := helm.CreateInfraChartValues(installationParams.GetInfraHelmValuesParams(syncRegistries, mnf.Images.Zot))
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
	serverValues, err := helm.CreateTensorleapChartValues(installationParams.GetServerHelmValuesParams(mnf.Tag))
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
