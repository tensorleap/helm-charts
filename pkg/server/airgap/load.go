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
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

func Load(file io.Reader) (
	installationManifest *manifest.InstallationManifest,
	chart *chart.Chart,
	clean func(),
	err error,
) {
	tarReader := tar.NewReader(file)
	var imageLoaded bool
	var helmLoaded bool

	cleanFuncs := make([]func(), 0)
	clean = func() {
		for _, cleanFunc := range cleanFuncs {
			cleanFunc()
		}
	}

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
			err = docker.LoadingImages(dockerClient, tarReader)
			if err != nil {
				return nil, nil, nil, err
			}
		case HELM_FILE_NAME:
			helmLoaded = true
			tempHelmFile, err := os.CreateTemp("", "helm-*.tgz")
			if err != nil {
				return nil, nil, nil, err
			}
			cleanFunc := func() {
				local.CleanupTempFile(tempHelmFile)
			}
			cleanFuncs = append(cleanFuncs, cleanFunc)
			_, err = io.Copy(tempHelmFile, tarReader)
			if err != nil {
				clean()
				return nil, nil, nil, err
			}
			chart, err = loader.Load(tempHelmFile.Name())
			if err != nil {
				clean()
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
	if !helmLoaded {
		return nil, nil, nil, fmt.Errorf("not found %s", HELM_FILE_NAME)
	}

	SetupEnvForK3dToolsImage(installationManifest.Images.K3dTools)

	return installationManifest, chart, clean, nil
}

func SetupEnvForK3dToolsImage(image string) {
	// k3d take the image from this env variable
	os.Setenv(types.K3dEnvImageTools, image)
}
