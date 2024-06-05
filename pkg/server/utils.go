package server

import (
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/tensorleap/helm-charts/pkg/helm/chart"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/airgap"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

const (
	KUBE_CONTEXT   = "k3d-tensorleap"
	KUBE_NAMESPACE = "tensorleap"
)

func InitInstallationProcess(flags *InstallationSourceFlags) (mnf *manifest.InstallationManifest, isAirGap bool, infraHelmChart, serverHelmChart *chart.Chart, err error) {
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
			mnf, err = manifest.GetByTag(flags.Tag)
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
