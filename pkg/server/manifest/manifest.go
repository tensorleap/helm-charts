package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tensorleap/helm-charts/pkg/log"
	"gopkg.in/yaml.v3"
)

type InstallationImages struct {
	K3s                    string `yaml:"k3s"`
	K3sGpu                 string `yaml:"k3sGpu"`
	K3dTools               string `yaml:"k3dTools"`
	Register               string `yaml:"register"`
	CheckDockerRequirement string `yaml:"checkDockerRequirement"`
}

type ManifestImages struct {
	InstallationImages `yaml:",inline"`
	K3sImages          []string `yaml:"k3sImages"`
	K3sGpuImages       []string `yaml:"k3sGpuImages"`
	ServerImages       []string `yaml:"serverImages"`
}

type HelmChartMeta struct {
	Version     string `yaml:"version"`
	RepoUrl     string `yaml:"repoUrl"`
	ChartName   string `yaml:"chartName"`
	ReleaseName string `yaml:"releaseName"`
}

type InstallationManifest struct {
	Version         string         `yaml:"version"`
	Images          ManifestImages `yaml:"images"`
	ServerHelmChart HelmChartMeta  `yaml:"serverHelmChart"`
}

func Load(installationManifestPath string) (*InstallationManifest, error) {
	fileBytes, err := os.ReadFile(installationManifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open installation manifest: %v", err)
	}
	mnf := &InstallationManifest{}
	err = yaml.Unmarshal(fileBytes, mnf)
	if err != nil {
		return nil, fmt.Errorf("failed to parse installation manifest: %v", err)
	}
	return mnf, nil
}

func (mnf *InstallationManifest) GetManifestVersion() string {
	return mnf.Version
}

func (mnf *InstallationManifest) Save(path string) error {
	log.Infof("Saving installation manifest to %s", path)
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory for installation manifest: %w", err)
	}
	b, err := yaml.Marshal(*mnf)
	if err != nil {
		return fmt.Errorf("failed to marshal installation manifest: %w", err)
	}
	err = os.WriteFile(path, b, 0644)
	if err != nil {
		return fmt.Errorf("failed to write installation manifest: %w", err)
	}

	log.Infof("Installation manifest saved")
	return nil
}

func (mnf *InstallationManifest) GetAllImages() []string {
	images := []string{}
	images = append(images, mnf.Images.ServerImages...)
	images = append(images, mnf.Images.K3sImages...)
	images = append(images, mnf.Images.K3sGpuImages...)
	images = append(images, mnf.Images.K3s)
	images = append(images, mnf.Images.K3sGpu)
	images = append(images, mnf.Images.Register)
	images = append(images, mnf.Images.K3dTools)
	images = append(images, mnf.Images.CheckDockerRequirement)
	return images
}

func (mnf *InstallationManifest) GetRegisterImages() []string {
	images := []string{}
	images = append(images, mnf.Images.ServerImages...)
	images = append(images, mnf.Images.K3sGpuImages...)
	images = append(images, mnf.Images.K3sImages...)
	return images
}
