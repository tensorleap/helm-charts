package manifest

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tensorleap/helm-charts/pkg/github"
	"github.com/tensorleap/helm-charts/pkg/log"
)

const (
	tlOwner = "tensorleap"
	tlRepo  = "helm-charts"
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

func GetLatestTag() (string, error) {

	releases, err := github.GetReleasesPage(tlOwner, tlRepo, 1, 10)
	if err != nil {
		return "", err
	}

	latest, err := findLatestTensorleapTag(releases)
	if err != nil {
		return "", err
	}
	return latest, nil
}

func findLatestTensorleapTag(releases []github.Release) (string, error) {
	for _, release := range releases {
		tag := release.TagName
		isCorrectTag := !strings.Contains(tag, "web") && !strings.Contains(tag, "engine") && !strings.Contains(tag, "node")
		if isCorrectTag {
			latestTag := tag
			log.Infof("Using tag: %s", latestTag)
			return latestTag, nil
		}
	}

	return "", ErrNoTags
}

func GetHelmVersionFromTag(tag string) string {
	// Define a regular expression pattern to match the version number
	pattern := `(\d+\.\d+\.\d+)`

	regex := regexp.MustCompile(pattern)

	// Find the first match of the pattern in the tag string
	match := regex.FindStringSubmatch(tag)

	if len(match) > 1 {
		// The first capture group contains the version number
		return match[1]
	}

	return ""
}

func getManifestWithBasicInfo(ref string) (*InstallationManifest, error) {
	// todo: get the basic info from ref
	return createManifestWithBasicInfo()
}

func createManifestWithBasicInfo() (*InstallationManifest, error) {

	k3sVersion := "v1.26.4-k3s1"

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
		RepoUrl:     "https://helm.tensorleap.ai",
		ChartName:   "tensorleap",
		ReleaseName: "tensorleap",
	}

	info := &InstallationManifest{
		Version:         "v1",
		ServerHelmChart: serverHelm,
		Images: ManifestImages{
			InstallationImages: installationImages,
			K3sImages:          k3sImages,
			K3sGpuImages:       k3sImages,
		},
	}

	return info, nil
}

func getTensorleapRepoRef(branch, tag string) string {
	if len(branch) > 0 {
		return branch
	}
	return tag
}

func getTensorleapImages(ref string) ([]string, error) {
	fileUrl := "images.txt"

	b, err := github.GetFileContent(tlOwner, tlRepo, fileUrl, ref)
	if err != nil {
		return nil, err
	}

	images, err := getImagesFromBytes(b)
	if err != nil {
		return nil, err
	}
	return images, nil
}
