package manifest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tensorleap/helm-charts/pkg/github"
	"gopkg.in/yaml.v3"
)

func TestBuildManifest(t *testing.T) {

	t.Run("Build manifest from local", func(t *testing.T) {
		mnf, err := GenerateManifestFromLocal(BuildLocalFileGetter("../../../"))
		if err != nil {
			t.Fatal(err)
		}
		err = ValidateManifestVersion(mnf)
		assert.NoError(t, err)
	})

	t.Run("Build manifest from github", func(t *testing.T) {
		t.Skip("Skip github test") // for debugging
		mnf, err := GenerateManifestFromRemote("", "")
		if err != nil {
			t.Fatal(err)
		}
		err = ValidateManifestVersion(mnf)
		assert.NoError(t, err)
	})
}

func TestGetHelmVersionFromTag(t *testing.T) {
	expectedVersion := "1.0.357"
	serverTag := "tensorleap-1.0.357"
	infraTag := "tensorleap-infra-1.0.357"

	withExtra := "tensorleap-1.0.357-extra.0"
	expectedWithExtra := "1.0.357-extra.0"

	assert.Equal(t, expectedVersion, GetHelmVersionFromTag(serverTag))
	assert.Equal(t, expectedVersion, GetHelmVersionFromTag(infraTag))
	assert.Equal(t, expectedWithExtra, GetHelmVersionFromTag(withExtra))
}

func TestFindLatestTag(t *testing.T) {
	expectedServerHelmTag := "tensorleap-1.0.357"
	expectedManifestTag := "manifest-1.0.357"

	tags := []github.Release{
		{TagName: "v0.0.1"}, // go tag
		{TagName: "tensorleap-infra-1.0.357"},
		{TagName: expectedManifestTag},
		{TagName: expectedServerHelmTag},
	}
	latestServerHelmTag, _ := findLatestTensorleapTag(tags, serverHelmTagReg)
	latestManifestTag, _ := findLatestTensorleapTag(tags, manifestTagReg)

	assert.Equal(t, expectedServerHelmTag, latestServerHelmTag)
	assert.Equal(t, expectedManifestTag, latestManifestTag)
}

type InstallationImagesV2 struct {
	K3s                    string `yaml:"k3s"`
	K3sGpu                 string `yaml:"k3sGpu"`
	K3dTools               string `yaml:"k3dTools"`
	Register               string `yaml:"register"`
	CheckDockerRequirement string `yaml:"checkDockerRequirement"`
}

type ManifestImagesV2 struct {
	InstallationImagesV2 `yaml:",inline"`
	K3sImages            []string `yaml:"k3sImages"`
	K3sGpuImages         []string `yaml:"k3sGpuImages"`
	ServerImages         []string `yaml:"serverImages"`
}

type HelmChartMetaV2 struct {
	Version     string `yaml:"version"`
	RepoUrl     string `yaml:"repoUrl"`
	ChartName   string `yaml:"chartName"`
	ReleaseName string `yaml:"releaseName"`
}

type InstallationManifestV2 struct {
	Version          string           `yaml:"version"`
	InstallerVersion string           `yaml:"installerVersion"`
	AppVersion       string           `yaml:"appVersion"`
	Images           ManifestImagesV2 `yaml:"images"`
	ServerHelmChart  HelmChartMetaV2  `yaml:"serverHelmChart"`
	InfraHelmChart   HelmChartMetaV2  `yaml:"infraHelmChart"`
}

func TestManifestV2(t *testing.T) {
	b, err := os.ReadFile("installation-manifest-v2.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var mnfV2 InstallationManifestV2
	var mnf InstallationManifest
	err = yaml.Unmarshal(b, &mnfV2)
	if err != nil {
		t.Fatal(err)
	}
	err = yaml.Unmarshal(b, &mnf)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, toMap(mnfV2), toMap(mnf))
}

func TestGetLatestServerHelmChartTag(t *testing.T) {
	t.Skip("Skip github test") // for debugging
	mnf, err := GenerateManifestFromRemote("", "1.1.0-test.2")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, mnf.InfraHelmChart.Version, "1.1.0-test.2")
}

func toMap(v interface{}) map[string]interface{} {
	b, err := yaml.Marshal(v)
	if err != nil {
		panic(err)
	}
	var m map[string]interface{}
	err = yaml.Unmarshal(b, &m)
	if err != nil {
		panic(err)
	}
	return m
}
