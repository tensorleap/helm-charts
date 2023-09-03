package server

import (
	"os"
	"strings"

	"github.com/tensorleap/helm-charts/pkg/helm/chart"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/airgap"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

const (
	KUBE_CONTEXT   = "k3d-tensorleap"
	KUBE_NAMESPACE = "tensorleap"
)

func InitInstallationProcess(airgapInstallationFilePath, tag string) (mnf *manifest.InstallationManifest, isAirgap bool, helmChart *chart.Chart, clean func(), err error) {

	isAirgap = airgapInstallationFilePath != ""
	if isAirgap {
		log.DisableReporting()
		var file *os.File
		file, err = os.Open(airgapInstallationFilePath)
		if err != nil {
			return nil, false, nil, nil, err
		}
		mnf, helmChart, clean, err = airgap.Load(file)
		if err != nil {
			log.SendCloudReport("error", "Failed to load airgap installation file", "Failed",
				&map[string]interface{}{"error": err.Error()})
			return nil, false, nil, nil, err
		}
	} else {
		mnf, err = manifest.GenerateManifest("", tag)
		if err != nil {
			log.SendCloudReport("error", "Build manifest failed", "Failed",
				&map[string]interface{}{"error": err.Error()})
			return nil, false, nil, nil, err
		}
		helmChart, clean, err = chart.Load(mnf.ServerHelmChart.RepoUrl, mnf.ServerHelmChart.ChartName, mnf.ServerHelmChart.Version)
		if err != nil {
			log.SendCloudReport("error", "Failed loading helm chart", "Failed",
				&map[string]interface{}{"error": err.Error()})
			return nil, false, nil, nil, err
		}
	}
	airgap.SetupEnvForK3dToolsImage(mnf.Images.K3dTools)
	return
}

func CalcWhichImagesToCache(manifest *manifest.InstallationManifest, useGpu, isAirgap bool) (necessaryImages []string, backgroundImage string) {

	allImages := []string{}

	allImages = append(allImages, manifest.Images.ServerImages...)
	if useGpu {
		allImages = append(allImages, manifest.Images.K3sGpuImages...)
	} else {
		allImages = append(allImages, manifest.Images.K3sImages...)
	}
	if isAirgap {
		return allImages, ""
	}

	necessaryImages = []string{}
	for _, img := range allImages {
		if strings.Contains(img, "engine") {
			backgroundImage = img
		} else if len(img) > 0 {
			necessaryImages = append(necessaryImages, img)
		}
	}

	return
}
