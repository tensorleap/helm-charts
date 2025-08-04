package airgap

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/k3d-io/k3d/v5/pkg/types"
	"github.com/tensorleap/helm-charts/pkg/docker"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

func Load(file io.Reader) (
	installationManifest *manifest.InstallationManifest,
	infraChart, serverChart *chart.Chart,
	err error,
) {
	tarReader := tar.NewReader(file)
	var imageLoaded bool
	var infraChartLoaded bool
	var serverChartLoaded bool

	dockerClient, err := docker.NewClient()
	if err != nil {
		return nil, nil, nil, err
	}

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, nil, nil, err
		}

		fileName := filepath.Clean(header.Name)

		switch fileName {
		case MANIFEST_FILE_NAME:
			installationManifest = &manifest.InstallationManifest{}
			content, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, nil, nil, err
			}

			err = yaml.Unmarshal(content, installationManifest)
			if err != nil {
				return nil, nil, nil, err
			}
		case IMAGES_FILE_NAME:
			imageLoaded = true
			if installationManifest != nil {
				_, notFound, err := docker.GetExistedAndNotExistedImages(dockerClient, installationManifest.GetAllImages())
				if err != nil {
					return nil, nil, nil, err
				}
				if len(notFound) == 0 {
					log.Info("All images already exist, skipping loading images from tar file")
					_, err = io.Copy(io.Discard, tarReader) // Skip the rest of the tarReader
					if err != nil {
						log.Warnf("Failed to skip loading images from tar file: %v", err)
					}
					break
				}
			}
			err = docker.LoadingImages(dockerClient, tarReader)
			if err != nil {
				return nil, nil, nil, err
			}
		case INFRA_HELM_CHART_FILE_NAME:
			infraChartLoaded = true
			infraChart, err = loadChart(tarReader)
			if err != nil {
				return nil, nil, nil, err
			}
		case SERVER_HELM_CHART_FILE_NAME:
			serverChartLoaded = true
			serverChart, err = loadChart(tarReader)
			if err != nil {
				return nil, nil, nil, err
			}
		}
	}

	if installationManifest == nil {
		return nil, nil, nil, fmt.Errorf("not found %s", MANIFEST_FILE_NAME)
	}
	if !imageLoaded {
		return nil, nil, nil, fmt.Errorf("not found %s", IMAGES_FILE_NAME)
	}
	if !infraChartLoaded {
		return nil, nil, nil, fmt.Errorf("not found %s", INFRA_HELM_CHART_FILE_NAME)
	}
	if !serverChartLoaded {
		return nil, nil, nil, fmt.Errorf("not found %s", SERVER_HELM_CHART_FILE_NAME)
	}

	SetupEnvForK3dToolsImage(installationManifest.Images.K3dTools)

	return installationManifest, infraChart, serverChart, nil
}

func loadChart(tarReader io.Reader) (*chart.Chart, error) {
	tempHelmFile, err := os.CreateTemp("", "helm-*.tgz")
	if err != nil {
		return nil, err
	}
	defer local.CleanupTempFile(tempHelmFile)
	_, err = io.Copy(tempHelmFile, tarReader)
	if err != nil {
		return nil, err
	}
	chart, err := loader.Load(tempHelmFile.Name())
	if err != nil {
		return nil, err
	}
	return chart, nil
}

func SetupEnvForK3dToolsImage(image string) {
	// k3d take the image from this env variable
	os.Setenv(types.K3dEnvImageTools, image)
}
