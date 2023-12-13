package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tensorleap/helm-charts/pkg/github"
	"github.com/tensorleap/helm-charts/pkg/helm/chart"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/version"
	"gopkg.in/yaml.v3"
)

const (
	tlOwner                  = "tensorleap"
	tlRepo                   = "helm-charts"
	k3sVersion               = "v1.26.4-k3s1"
	tensorleapChartName      = "tensorleap"
	tensorleapInfraChartName = "tensorleap-infra"
	helmRepoUrl              = "https://helm.tensorleap.ai"
	localHelmRepoUrl         = "charts"
)

func getK3sImages(k3sVersion string) ([]string, error) {
	tag := strings.Replace(k3sVersion, "-", "+", 1)
	owner := "k3s-io"
	repo := "k3s"

	bytes, err := github.GetTagArtifact(owner, repo, "k3s-images.txt", tag)
	if err != nil {
		return nil, fmt.Errorf("failed fetching latest k3s images: %v", err)
	}
	images, err := getImagesFromBytes(bytes)
	if err != nil {
		return nil, fmt.Errorf("failed fetching latest k3s images: %v", err)
	}

	return images, nil
}

func getImagesFromBytes(imagesFile []byte) ([]string, error) {

	imagesFromFile := strings.Split(string(imagesFile), "\n")
	images := []string{}
	for _, image := range imagesFromFile {
		if len(image) > 0 {
			images = append(images, image)
		}
	}
	return images, nil
}

var ErrNoTags = fmt.Errorf("no tag found")

var manifestTagReg = regexp.MustCompile(`manifest-\d+\.\d+\.\d+`)

func GetLatestManifestTag() (string, error) {

	releases, err := github.GetReleasesPage(tlOwner, tlRepo, 1, 10)
	if err != nil {
		return "", err
	}

	latest, err := findLatestTensorleapTag(releases, manifestTagReg)
	if err != nil {
		return "", err
	}
	return latest, nil
}

var serverHelmTagReg = regexp.MustCompile(`tensorleap-\d+\.\d+\.\d+`)

func GetLatestServerHelmChartTag() (latestServerTag string, err error) {

	releases, err := github.GetReleasesPage(tlOwner, tlRepo, 1, 10)
	if err != nil {
		return
	}

	latestServerTag, err = findLatestTensorleapTag(releases, serverHelmTagReg)
	if err != nil {
		return
	}
	return
}

func GetLatestInfraHelmChartVersion() (latestInfraVersion string, err error) {
	version, err := chart.GetVersion(helmRepoUrl, tensorleapInfraChartName, "")
	if err != nil {
		return
	}
	return version.Version, nil
}

func findLatestTensorleapTag(releases []github.Release, pattern *regexp.Regexp) (string, error) {
	for _, release := range releases {
		tag := release.TagName
		// this code will change when we clean up the releases
		isCorrectTag := pattern.MatchString(tag)
		if isCorrectTag {
			latestTag := tag
			log.Infof("Using tag: %s", latestTag)
			return latestTag, nil
		}
	}

	return "", ErrNoTags
}

func getLocalChartVersion(chartName string, fileGetter FileGetter) (string, error) {
	path := fmt.Sprintf("charts/%s/Chart.yaml", chartName)
	b, err := fileGetter(path)
	if err != nil {
		return "", err
	}
	var chart VersionRecord
	err = yaml.Unmarshal(b, &chart)
	if err != nil {
		return "", err
	}
	return chart.Version, nil
}

func GetHelmVersionFromTag(tag string) string {
	// Define a regular expression pattern to match the version number
	regex := regexp.MustCompile(`(\d+\.\d+\.\d+.*$)`)

	// Find the first match of the pattern in the tag string
	match := regex.FindStringSubmatch(tag)

	if len(match) > 1 {
		// The first capture group contains the version number
		return match[1]
	}

	return ""
}

func NewManifest(helmRepoUrl, serverHelmVersion, infraHelmVersion string, serverImages []string) (*InstallationManifest, error) {

	k3sImages, err := getK3sImages(k3sVersion)

	if err != nil {
		return nil, err
	}

	// setup images
	installationImages := InstallationImages{
		K3dTools:               "ghcr.io/k3d-io/k3d-tools:5.5.2",
		K3s:                    fmt.Sprintf("docker.io/rancher/k3s:%s", k3sVersion),
		K3sGpu:                 fmt.Sprintf("us-central1-docker.pkg.dev/tensorleap/main/k3s:%s-cuda-11.8.0-ubuntu-22.04", k3sVersion),
		Register:               "docker.io/library/registry:2",
		CheckDockerRequirement: "alpine:3.18.3",
	}

	// setup server helm
	serverHelm := HelmChartMeta{
		RepoUrl:     helmRepoUrl,
		ChartName:   tensorleapChartName,
		ReleaseName: tensorleapChartName,
		Version:     serverHelmVersion,
	}

	// setup infra helm
	infraHelm := HelmChartMeta{
		RepoUrl:     helmRepoUrl,
		ChartName:   tensorleapInfraChartName,
		ReleaseName: tensorleapInfraChartName,
		Version:     infraHelmVersion,
	}

	info := &InstallationManifest{
		Version:          CurrentManifestVersion,
		InstallerVersion: version.Version,
		AppVersion:       CurrentAppVersion,
		ServerHelmChart:  serverHelm,
		InfraHelmChart:   infraHelm,
		Images: ManifestImages{
			InstallationImages: installationImages,
			K3sImages:          k3sImages,
			K3sGpuImages:       k3sImages,
			ServerImages:       serverImages,
		},
	}

	return info, nil
}

// getTensorleapImages returns the list of images from the images.txt file in the repo
func getTensorleapImages(fileGetter FileGetter) ([]string, error) {
	filePath := "images.txt"
	b, err := fileGetter(filePath)

	if err != nil {
		return nil, err
	}

	images, err := getImagesFromBytes(b)
	if err != nil {
		return nil, err
	}
	return images, nil
}

type FileGetter = func(url string) ([]byte, error)

func BuildLocalFileGetter(repoPath string) FileGetter {
	return func(filePath string) ([]byte, error) {
		relativeFilePath := filepath.Join(repoPath, filePath)
		return os.ReadFile(relativeFilePath)
	}
}

func buildRemoteFileGetter(ref string) FileGetter {
	return func(filePath string) ([]byte, error) {
		return github.GetFileContent(tlOwner, tlRepo, filePath, ref)
	}
}
