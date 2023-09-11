package airgap

import (
	"archive/tar"
	"fmt"
	"io"
	"os"

	"github.com/tensorleap/helm-charts/pkg/docker"
	"github.com/tensorleap/helm-charts/pkg/helm/chart"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"gopkg.in/yaml.v3"
)

func Pack(mnf *manifest.InstallationManifest, outputFile io.Writer) error {

	tarWriter := tar.NewWriter(outputFile)
	defer tarWriter.Close()

	err := AddManifest(tarWriter, mnf)
	if err != nil {
		return err
	}

	err = AddImages(tarWriter, mnf)
	if err != nil {
		return err
	}

	err = AddHelm(tarWriter, &mnf.ServerHelmChart, SERVER_HELM_CHART_FILE_NAME)
	if err != nil {
		return err
	}

	err = AddHelm(tarWriter, &mnf.InfraHelmChart, INFRA_HELM_CHART_FILE_NAME)
	if err != nil {
		return err
	}

	return nil
}

func AddHelm(tarWriter *tar.Writer, chartMeta *manifest.HelmChartMeta, fileName string) error {

	tempHelmFile, clean, err := loadChartIntoTempFile(chartMeta)
	if err != nil {
		return err
	}
	defer clean()

	tempHelmFileStat, err := tempHelmFile.Stat()
	if err != nil {
		return err
	}
	helmHeader, err := tar.FileInfoHeader(tempHelmFileStat, tempHelmFileStat.Name())
	if err != nil {
		return err
	}
	helmHeader.Name = fileName
	err = tarWriter.WriteHeader(helmHeader)
	if err != nil {
		return err
	}
	_, err = tempHelmFile.Seek(0, 0)
	if err != nil {
		return err
	}
	_, err = io.Copy(tarWriter, tempHelmFile)
	if err != nil {
		return err
	}
	log.Infof("Packed helm chart: %s", chartMeta.ChartName)
	return nil
}

func AddImages(tarWriter *tar.Writer, mnf *manifest.InstallationManifest) error {
	dockerClient, err := docker.NewClient()
	if err != nil {
		return err
	}

	images := mnf.GetAllImages()

	// create temp file to store images
	// we can't stream the images directly to the tar writer, because we need to know the size of the images file before writing it to the tar
	tempImagesFile, err := os.CreateTemp("", "images.tgz")
	defer local.CleanupTempFile(tempImagesFile)
	if err != nil {
		return err
	}

	err = docker.DownloadDockerImages(dockerClient, images, tempImagesFile)

	if err != nil {
		return err
	}

	tempImagesFileStat, err := tempImagesFile.Stat()
	if err != nil {
		return err
	}
	imagesHeader, err := tar.FileInfoHeader(tempImagesFileStat, tempImagesFileStat.Name())
	if err != nil {
		return err
	}

	imagesHeader.Name = IMAGES_FILE_NAME
	err = tarWriter.WriteHeader(imagesHeader)
	if err != nil {
		return err
	}
	_, err = tempImagesFile.Seek(0, 0)
	if err != nil {
		return err
	}

	_, err = io.Copy(tarWriter, tempImagesFile)
	if err != nil {
		return err
	}
	log.Infof("Packed docker images")
	return nil
}

func AddManifest(tarWriter *tar.Writer, mnf *manifest.InstallationManifest) error {

	manifestBytes, err := yaml.Marshal(*mnf)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %v", err)
	}
	manifestHeader := &tar.Header{
		Name: MANIFEST_FILE_NAME,
		Mode: 0600,
		Size: int64(len(manifestBytes)),
	}

	err = tarWriter.WriteHeader(manifestHeader)
	if err != nil {
		return err
	}
	_, err = tarWriter.Write(manifestBytes)
	if err != nil {
		return err
	}
	log.Infof("Packed installation manifest")
	return nil
}

func loadChartIntoTempFile(chartMeta *manifest.HelmChartMeta) (tempHelmFile *os.File, clean func(), err error) {

	if chartMeta.IsLocal() {
		helmChart, err := chart.Load(chartMeta.RepoUrl, chartMeta.ChartName, chartMeta.Version)
		if err != nil {
			return nil, nil, err
		}
		tempDir := os.TempDir()
		filePath, err := chart.Save(helmChart, tempDir)
		if err != nil {
			return nil, nil, err
		}
		clean = func() { _ = os.Remove(filePath) }
		tempHelmFile, err = os.Open(filePath)
		if err != nil {
			clean()
			return nil, nil, err
		}
	} else {
		tempHelmFile, clean, err = chart.DownloadIntoTempFile(chartMeta.RepoUrl, chartMeta.ChartName, chartMeta.Version)
		if err != nil {
			return nil, nil, err
		}
	}

	return
}
