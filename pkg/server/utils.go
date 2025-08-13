package server

import (
	"context"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/tensorleap/helm-charts/pkg/docker"
	"github.com/tensorleap/helm-charts/pkg/helm"
	"github.com/tensorleap/helm-charts/pkg/helm/chart"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/airgap"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"k8s.io/kubectl/pkg/util/slice"
)

const (
	KUBE_CONTEXT   = "k3d-tensorleap"
	KUBE_NAMESPACE = "tensorleap"
)

func InitInstallationProcess(flags *InstallationSourceFlags, previousMnf *manifest.InstallationManifest) (mnf *manifest.InstallationManifest, isAirGap bool, infraHelmChart, serverHelmChart *chart.Chart, err error) {
	isAirGap = flags.AirGapInstallationFilePath != ""
	if isAirGap {
		log.DisableReporting()
		var file *os.File
		file, err = os.Open(flags.AirGapInstallationFilePath)
		if err != nil {
			return nil, false, nil, nil, err
		}
		mnf, infraHelmChart, serverHelmChart, err = airgap.Load(file)
		if err != nil {
			log.SendCloudReport("error", "Failed to load airgap installation file", "Failed",
				&map[string]interface{}{"error": err.Error()})
			return nil, false, nil, nil, err
		}
	} else {
		var err error
		if flags.Local {
			fileGetter := manifest.BuildLocalFileGetter("")
			mnf, err = manifest.GenerateManifestFromLocal(fileGetter)
		} else {
			tag := flags.Tag
			if previousMnf != nil && tag == "" && previousMnf.Tag != "" {
				latestMnfTag, err := manifest.GetLatestManifestTag()
				if err != nil {
					return nil, false, nil, nil, err
				}
				if previousMnf.Tag != latestMnfTag {
					usePreviousMnf, err := AskUserForVersionPreference(previousMnf.Tag, latestMnfTag)
					if err != nil {
						log.SendCloudReport("error", "Failed to ask for using current version", "Failed",
							&map[string]interface{}{"error": err.Error()})
						return nil, false, nil, nil, err
					}
					if usePreviousMnf {
						tag = previousMnf.Version
					}
				}
			}
			mnf, err = manifest.GetByTag(tag)
		}
		if err != nil {
			log.SendCloudReport("error", "Build manifest failed", "Failed",
				&map[string]interface{}{"error": err.Error()})
			return nil, false, nil, nil, err
		}
		serverHelmChart, err = chart.Load(mnf.ServerHelmChart.RepoUrl, mnf.ServerHelmChart.ChartName, mnf.ServerHelmChart.Version)
		if err != nil {
			log.SendCloudReport("error", "Failed loading tensorleap helm chart", "Failed",
				&map[string]interface{}{"error": err.Error()})
			return nil, false, nil, nil, err
		}
		infraHelmChart, err = chart.Load(mnf.InfraHelmChart.RepoUrl, mnf.InfraHelmChart.ChartName, mnf.InfraHelmChart.Version)
		if err != nil {
			log.SendCloudReport("error", "Failed loading tensorleap infra helm chart", "Failed",
				&map[string]interface{}{"error": err.Error()})
			return nil, false, nil, nil, err
		}
	}
	airgap.SetupEnvForK3dToolsImage(mnf.Images.K3dTools)
	return
}

func GetCurrentInsalledHelmChartVersion(ctx context.Context) (string, error) {
	cluster, err := k3d.GetCluster(ctx)
	if err != nil {
		return "", err
	}

	kubeConfigPath, clean, err := k3d.CreateTmpClusterKubeConfig(ctx, cluster)
	if err != nil {
		return "", err
	}
	defer clean()

	helmConfig, err := helm.CreateHelmConfig(kubeConfigPath, KUBE_CONTEXT, KUBE_NAMESPACE)
	if err != nil {
		return "", err
	}

	currentInfraVersion, err := helm.GetHelmReleaseVersion(helmConfig, KUBE_NAMESPACE)
	return currentInfraVersion, err
}

func SaveInstallation(mnf *manifest.InstallationManifest, installationParams *InstallationParams) error {
	err := mnf.Save(local.GetInstallationManifestPath())
	if err != nil {
		return err
	}
	return installationParams.Save()
}

func CalcWhichImagesToCache(manifest *manifest.InstallationManifest, useGpu, isAirgap bool) (necessaryImages []string) {

	allImages := []string{}

	allImages = append(allImages, manifest.Images.ServerImages...)
	if useGpu {
		allImages = append(allImages, manifest.Images.K3sGpuImages...)
	} else {
		allImages = append(allImages, manifest.Images.K3sImages...)
	}
	if isAirgap {
		return allImages
	}

	necessaryImages = []string{}
	for _, img := range allImages {
		if len(img) > 0 {
			necessaryImages = append(necessaryImages, img)
		}
	}

	return
}

func IsUseDefaultPropOption() bool {
	return os.Getenv("TL_USE_DEFAULT_OPTION") == "true"
}

func AskUserForVersionPreference(previousVersion, latestVersion string) (bool, error) {
	defaultValue := false
	if IsUseDefaultPropOption() {
		return defaultValue, nil
	}

	prompt := survey.Confirm{
		Message: fmt.Sprintf("A new version of Tensorleap is available (%s), do you want to use the current version (%s)?", latestVersion, previousVersion),
		Default: defaultValue,
	}
	confirm := false
	err := survey.AskOne(&prompt, &confirm)
	if err != nil {
		return false, err
	}
	if confirm {
		log.SendCloudReport("info", "User confirmed using current version", "Running",
			&map[string]interface{}{"version": previousVersion})
	} else {
		log.SendCloudReport("info", "User chose to upgrade to latest version", "Running",
			&map[string]interface{}{"version": latestVersion})
	}
	return confirm, nil
}

func AskForReinstall() (bool, error) {
	prompt := survey.Confirm{
		Message: "Reinstall is required to complete the upgrade, It will stop all running jobs, are you sure you want to continue?",
		Default: true,
	}
	confirm := false
	err := survey.AskOne(&prompt, &confirm)
	if err != nil {
		return false, err
	}
	if !confirm {
		log.SendCloudReport("info", "User aborted reinstall", "Failed", nil)
	} else {
		log.SendCloudReport("info", "User confirmed reinstall", "Running", nil)
	}
	return confirm, nil
}

func LoadPreviousManifest() (mnf *manifest.InstallationManifest, err error) {
	mnf, err = manifest.Load(local.GetInstallationManifestPath())
	if err == manifest.ErrManifestNotFound {
		return nil, err
	}
	if err != nil {
		log.SendCloudReport("error", "Failed loading manifest", "Failed",
			&map[string]interface{}{"error": err.Error()})
		return nil, err
	}
	return
}

func getHomePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Errorf("failed to get home directory: %w", err))
	}

	return homeDir
}

func cleanImages(currentMnf *manifest.InstallationManifest, previousMnf *manifest.InstallationManifest, isCleanCurrentImages bool) error {
	imagesToClean := []string{}
	currentImages := currentMnf.GetAllImages()
	if isCleanCurrentImages {
		imagesToClean = append(imagesToClean, currentMnf.Images.ServerImages...)
		imagesToClean = append(imagesToClean, currentMnf.Images.K3sImages...)
	}
	if previousMnf != nil {
		previousImages := previousMnf.GetAllImages()
		for _, img := range previousImages {
			if !slice.ContainsString(currentImages, img, nil) {
				imagesToClean = append(imagesToClean, img)
			}
		}
	}
	imagesToClean = uniqueStrings(imagesToClean)
	if len(imagesToClean) == 0 {
		return nil
	}
	if isCleanCurrentImages {
		log.Info("Cleaning images")
	} else {
		log.Info("Cleaning old images")
	}
	dockerClient, err := docker.NewClient()
	if err != nil {
		return err
	}
	err = docker.RemoveImages(dockerClient, imagesToClean)
	return err
}

func uniqueStrings(arr []string) []string {
	seen := make(map[string]struct{})
	result := arr[:0] // Reuse the input slice for efficiency

	for _, v := range arr {
		if _, exists := seen[v]; !exists {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}
