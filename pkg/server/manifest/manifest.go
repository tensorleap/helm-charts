package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tensorleap/helm-charts/pkg/helm/chart"
	"github.com/tensorleap/helm-charts/pkg/log"
	"gopkg.in/yaml.v3"
)

const CurrentManifestVersion = "1.0.0"
const CurrentAppVersion = "0.1.0"

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

func (chartMeta *HelmChartMeta) IsLocal() bool {
	return chart.IsLocalChart(chartMeta.RepoUrl)
}

type VersionRecord struct {
	Version string `yaml:"version"`
}

func (mnf *VersionRecord) GetVersion() string {
	return mnf.Version
}

type InstallationManifest struct {
	Version          string         `yaml:"version"`
	InstallerVersion string         `yaml:"installerVersion"` // InstallerVersion determines the version of the installer, if the version is different from the current version we will require the user to update the installer
	AppVersion       string         `yaml:"appVersion"`       // AppVersion determines the version of the tensorleap application, if the version is different from the current version we will require the user to reinstall the application
	Images           ManifestImages `yaml:"images"`
	Tag              string         `yaml:"tag,omitempty"` // Tag is used to identify the manifest version
	ServerHelmChart  HelmChartMeta  `yaml:"serverHelmChart"`
	InfraHelmChart   HelmChartMeta  `yaml:"infraHelmChart"`
}

type VersionGetter interface {
	GetVersion() string
}

var ErrManifestNotFound = fmt.Errorf("installation manifest not found")

func Load(installationManifestPath string) (*InstallationManifest, error) {
	fileBytes, err := os.ReadFile(installationManifestPath)
	if os.IsNotExist(err) {
		return nil, ErrManifestNotFound
	} else if err != nil {
		return nil, fmt.Errorf("failed to open installation manifest: %v", err)
	}
	mnf := &InstallationManifest{}
	err = yaml.Unmarshal(fileBytes, mnf)
	if err != nil {
		return nil, fmt.Errorf("failed to parse installation manifest: %v", err)
	}
	return mnf, nil
}

func LoadFromBytes(data []byte) (*InstallationManifest, error) {
	mnfVersion := &VersionRecord{}
	err := yaml.Unmarshal(data, mnfVersion)
	if err != nil {
		return nil, err
	}

	if err := ValidateManifestVersion(mnfVersion); err != nil {
		return nil, err
	}

	mnf := &InstallationManifest{}
	err = yaml.Unmarshal(data, mnf)
	if err != nil {
		return nil, err
	}
	return mnf, nil
}

var ErrUnsupportedManifestVersion = fmt.Errorf("unsupported installation manifest version, supported manifest version %s", CurrentManifestVersion)

func ValidateManifestVersion(mnf VersionGetter) error {
	if mnf.GetVersion() != CurrentManifestVersion {
		return ErrUnsupportedManifestVersion
	}
	return nil
}

func (mnf *InstallationManifest) GetVersion() string {
	return mnf.Version
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
	images := mnf.GetRunningOnMachineImages()
	images = append(images, mnf.GetRegisterImages()...)
	return images
}

func (mnf *InstallationManifest) GetRunningOnMachineImages() []string {
	images := []string{}
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
