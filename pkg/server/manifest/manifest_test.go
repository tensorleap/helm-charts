package manifest

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tensorleap/helm-charts/pkg/github"
	"gopkg.in/yaml.v3"
)

func TestBuildManifest(t *testing.T) {

	t.Skip("skip test") // For debugging only

	mnf, err := GenerateManifest("")
	if err != nil {
		t.Fatal(err)
	}
	err = ValidateManifestVersion(mnf)
	assert.NoError(t, err)
}

func TestGetHelmVersionFromTag(t *testing.T) {
	tag := "tensorleap-1.0.357"
	assert.Equal(t, "1.0.357", GetHelmVersionFromTag(tag))
}

func TestFindLatestTag(t *testing.T) {
	expectedServerHelmTag := "tensorleap-1.0.357"
	expectedManifestTag := "manifest-1.0.357"

	tags := []github.Release{
		{TagName: "v0.0.1"},
		{TagName: "tensorleap-infra-1.0.357"},
		{TagName: expectedManifestTag},
		{TagName: expectedServerHelmTag},
	}
	latestServerHelmTag, _ := findLatestTensorleapTag(tags, serverHelmTagReg)
	latestManifestTag, _ := findLatestTensorleapTag(tags, manifestTagReg)

	assert.Equal(t, expectedServerHelmTag, latestServerHelmTag)
	assert.Equal(t, expectedManifestTag, latestManifestTag)

}

type InstallationImagesV1 struct {
	K3s                    string `yaml:"k3s"`
	K3sGpu                 string `yaml:"k3sGpu"`
	K3dTools               string `yaml:"k3dTools"`
	Register               string `yaml:"register"`
	CheckDockerRequirement string `yaml:"checkDockerRequirement"`
}

type ManifestImagesV1 struct {
	InstallationImagesV1 `yaml:",inline"`
	K3sImages            []string `yaml:"k3sImages"`
	K3sGpuImages         []string `yaml:"k3sGpuImages"`
	ServerImages         []string `yaml:"serverImages"`
}

type HelmChartMetaV1 struct {
	Version     string `yaml:"version"`
	RepoUrl     string `yaml:"repoUrl"`
	ChartName   string `yaml:"chartName"`
	ReleaseName string `yaml:"releaseName"`
}

type InstallationManifestV1 struct {
	Version         string           `yaml:"version"`
	Images          ManifestImagesV1 `yaml:"images"`
	ServerHelmChart HelmChartMetaV1  `yaml:"serverHelmChart"`
}

func TestManifestV1(t *testing.T) {
	b, err := os.ReadFile("installation-manifest-v1.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var mnfV1 InstallationManifestV1
	var mnf InstallationManifest
	err = yaml.Unmarshal(b, &mnfV1)
	if err != nil {
		t.Fatal(err)
	}
	err = yaml.Unmarshal(b, &mnf)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, toMap(mnfV1), toMap(mnf))
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
