package server

import (
	"context"
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/tensorleap/helm-charts/pkg/containerd"
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
	isAirGap = flags.IsAirGap()
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

				isInstallLatestVersion, err := AskUserForIsUseLatestVersion(previousMnf.Tag)
				if err != nil {
					log.SendCloudReport("error", "Failed to ask for using current version", "Failed",
						&map[string]interface{}{"error": err.Error()})
					return nil, false, nil, nil, err
				}
				if !isInstallLatestVersion {
					tag = previousMnf.Tag
					mnf = previousMnf
				}

			}
			if mnf == nil {
				mnf, err = manifest.GetByTag(tag)
				if err != nil {
					log.SendCloudReport("error", "Build manifest failed", "Failed",
						&map[string]interface{}{"error": err.Error()})
					return nil, false, nil, nil, err
				}
			}
			log.Info("Using tag: " + mnf.Tag)
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

// LoadManifestOnly loads only the manifest to enable lightweight decisions (e.g. reinstall prompt)
// before loading heavy assets (images/charts). For airgap installs, it reads the manifest from the
// tarball; for non-airgap it follows the same tag/local logic as a full install but skips chart loads.
func LoadManifestOnly(flags *InstallationSourceFlags, previousMnf *manifest.InstallationManifest) (mnf *manifest.InstallationManifest, isAirGap bool, err error) {
	isAirGap = flags.IsAirGap()
	if isAirGap {
		file, openErr := os.Open(flags.AirGapInstallationFilePath)
		if openErr != nil {
			return nil, false, openErr
		}
		defer file.Close()
		mnf, err = airgap.LoadManifestOnly(file)
		if err != nil {
			return nil, false, err
		}
		return mnf, true, nil
	}

	if flags.Local {
		fileGetter := manifest.BuildLocalFileGetter("")
		mnf, err = manifest.GenerateManifestFromLocal(fileGetter)
	} else {
		tag := flags.Tag
		if previousMnf != nil && tag == "" && previousMnf.Tag != "" {

			isInstallLatestVersion, err := AskUserForIsUseLatestVersion(previousMnf.Tag)
			if err != nil {
				log.SendCloudReport("error", "Failed to ask for using current version", "Failed",
					&map[string]interface{}{"error": err.Error()})
				return nil, false, err
			}
			if !isInstallLatestVersion {
				tag = previousMnf.Tag
				mnf = previousMnf
			}

		}
		if mnf == nil {
			mnf, err = manifest.GetByTag(tag)
			if err != nil {
				log.SendCloudReport("error", "Build manifest failed", "Failed",
					&map[string]interface{}{"error": err.Error()})
				return nil, false, err
			}
		}
		log.Info("Using tag: " + mnf.Tag)
	}
	if err != nil {
		log.SendCloudReport("error", "Build manifest failed", "Failed",
			&map[string]interface{}{"error": err.Error()})
		return nil, false, err
	}
	return mnf, false, nil
}

// EnsureReinstallConsent checks whether reinstall is needed and, if so, prompts once and marks params
// to skip subsequent prompts in the flow. Returns true if reinstall is required (and confirmed).
func EnsureReinstallConsent(ctx context.Context, mnfPreview, previousMnf *manifest.InstallationManifest, installationParams, previousParams *InstallationParams) (bool, error) {
	shouldReinstall, err := IsNeedsToReinstall(ctx, mnfPreview, previousMnf, installationParams, previousParams)
	if err != nil {
		return false, err
	}
	if !shouldReinstall {
		return false, nil
	}

	isContinue, err := AskForReinstall()
	if err != nil {
		return false, err
	}
	if !isContinue {
		return false, fmt.Errorf("reinstall aborted")
	}

	return true, nil
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

func CalcWhichImagesToCache(manifest *manifest.InstallationManifest, useGpu bool, imageCachingMethod k3d.ImageCachingMethod) (necessaryImages []string) {

	if imageCachingMethod != k3d.ImageCachingRegistry {
		return []string{}
	}

	allImages := []string{}

	allImages = append(allImages, manifest.Images.ServerImages...)
	if useGpu {
		allImages = append(allImages, manifest.Images.K3sGpuImages...)
	} else {
		allImages = append(allImages, manifest.Images.K3sImages...)
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

func AskUserForIsUseLatestVersion(previousTag string) (bool, error) {
	isInstallLatestVersion := false

	if IsUseDefaultPropOption() {
		return isInstallLatestVersion, nil
	}

	latestTag, err := manifest.GetLatestManifestTag()
	if err != nil {
		log.Warnf("Failed to get latest manifest tag: %v", err)
		return isInstallLatestVersion, nil
	}
	if latestTag == previousTag {
		return isInstallLatestVersion, nil
	}

	prompt := survey.Confirm{
		Message: fmt.Sprintf("Do you want to use latest version (latest: %s, current: %s)?", latestTag, previousTag),
		Default: isInstallLatestVersion,
	}
	err = survey.AskOne(&prompt, &isInstallLatestVersion)

	if err != nil {
		return false, err
	}
	if isInstallLatestVersion {
		log.SendCloudReport("info", "User chose to upgrade to latest version", "Running",
			&map[string]interface{}{"version": latestTag})
	} else {
		log.SendCloudReport("info", "User confirmed using current version", "Running",
			&map[string]interface{}{"version": previousTag})
	}
	return isInstallLatestVersion, nil
}

func AskForReinstall() (bool, error) {
	if IsUseDefaultPropOption() {
		// In non-interactive mode, proceed with reinstall
		log.SendCloudReport("info", "Auto-confirmed reinstall (non-interactive mode)", "Running", nil)
		return true, nil
	}
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

func cleanImagesFromContainerd(ctx context.Context, currentMnf *manifest.InstallationManifest, dockerName string) error {

	err := containerd.PruneContainerdExceptImageList(ctx, dockerName, "k8s.io", currentMnf.GetAllImages(), false)
	if err != nil {
		return err
	}
	return nil
}

func cleanImages(currentMnf *manifest.InstallationManifest, previousMnf *manifest.InstallationManifest, isCleanCurrentImages bool) error {
	imagesToClean := []string{}
	imageToKeep := []string{}
	if isCleanCurrentImages {
		imagesToClean = append(imagesToClean, currentMnf.GetRegisterImages()...)
		imageToKeep = append(imageToKeep, currentMnf.GetRunningOnMachineImages()...)
	} else {
		imageToKeep = append(imageToKeep, currentMnf.GetAllImages()...)
	}
	if previousMnf != nil {
		previousImages := previousMnf.GetAllImages()
		for _, img := range previousImages {
			if !slice.ContainsString(imageToKeep, img, nil) {
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
