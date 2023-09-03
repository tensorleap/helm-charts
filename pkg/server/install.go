package server

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/tensorleap/helm-charts/pkg/helm"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"helm.sh/helm/v3/pkg/chart"
)

func InitUseGPU(useGpu *bool, useCpu bool) error {
	if *useGpu || useCpu {
		return nil
	}

	prompt := survey.Confirm{
		Message: "Do you want to use GPU?",
		Default: false,
	}

	err := survey.AskOne(&prompt, useGpu)
	return err
}

func InitDatasetDirectory(datasetDirectory *string) error {
	defaultDatasetDirectory := GetDefaultDataVolume()

	if *datasetDirectory == "" {
		fromPath := ""
		prompt := survey.Input{
			Message: "Enter dataset directory:",
			Default: defaultDatasetDirectory,
		}
		err := survey.AskOne(&prompt, &fromPath)
		if err != nil {
			return err
		}
		*datasetDirectory = fromPath
	}
	if !strings.Contains(*datasetDirectory, ":") {
		toPath := ""
		prompt := survey.Input{
			Message: "Enter container dataset directory:",
			Default: *datasetDirectory,
		}
		err := survey.AskOne(&prompt, &toPath)
		if err != nil {
			return err
		}
		*datasetDirectory = fmt.Sprintf("%s:%s", *datasetDirectory, toPath)
	}
	log.SendCloudReport("info", "Init data volume", "Starting",
		&map[string]interface{}{"params": map[string]interface{}{"datasetDirectory": datasetDirectory}},
	)

	dataPath := strings.Split(*datasetDirectory, ":")[0]
	return os.MkdirAll(dataPath, 0777)
}

func GetDefaultDataVolume() string {
	defaultDataPath := fmt.Sprintf("%s/tensorleap/data", getHomePath())
	return defaultDataPath
}

func InstallHelm(ctx context.Context, chartMeta manifest.HelmChartMeta, chart *chart.Chart, useGpu bool, dataContainerPath string, disableMetrics bool) error {
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

	isHelmReleaseExisted, err := helm.IsHelmReleaseExists(helmConfig, chartMeta)
	if err != nil {
		log.SendCloudReport("error", "Failed checking if helm release exists", "Failed",
			&map[string]interface{}{"helmConfig": helmConfig, "error": err.Error()})
		return err
	}
	values := helm.CreateTensorleapChartValues(useGpu, dataContainerPath, disableMetrics)
	if isHelmReleaseExisted {
		log.SendCloudReport("info", "Running helm upgrade", "Running", &map[string]interface{}{"isHelmReleaseExisted": isHelmReleaseExisted})
		if err := helm.UpgradeTensorleapChartVersion(
			helmConfig,
			chartMeta,
			chart,
			values,
		); err != nil {
			log.SendCloudReport("error", "Failed upgrading helm charts versions", "Failed",
				&map[string]interface{}{"isHelmReleaseExisted": isHelmReleaseExisted, "helmConfig": helmConfig, "error": err.Error()})
			return err
		}
	} else {
		log.SendCloudReport("info", "Setting up helm repo", "Running", &map[string]interface{}{"isHelmReleaseExisted": isHelmReleaseExisted})
		if err := helm.InstallChart(
			helmConfig,
			chartMeta,
			chart,
			values,
		); err != nil {
			log.SendCloudReport("error", "Failed installing latest chart versions", "Failed",
				&map[string]interface{}{"isHelmReleaseExisted": isHelmReleaseExisted, "helmConfig": helmConfig, "values": values, "error": err.Error()})
			return err
		}
	}

	log.SendCloudReport("info", "Successfully installed helm charts", "Running", nil)
	return nil
}

func getHomePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Errorf("failed to get home directory: %w", err))
	}

	return homeDir
}
