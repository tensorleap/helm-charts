package manifest

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
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

	// Third-party images come from our mirror (mirror.go); copy a new or bumped
	// one there before changing it here.
	k3dUtilsImage          = MirrorRegistry + "/ghcr.io/k3d-io/k3d-tools:5.5.2"
	registerImage          = MirrorRegistry + "/docker.io/library/registry:2"
	zotImage               = MirrorRegistry + "/ghcr.io/project-zot/zot:v2.1.15"
	checkDockerRequirement = MirrorRegistry + "/docker.io/library/alpine:3.18.3"
)

var (
	k3sImage    = fmt.Sprintf("%s/docker.io/rancher/k3s:%s", MirrorRegistry, k3sVersion)
	k3sGpuImage = fmt.Sprintf("public.ecr.aws/tensorleap/k3s:%s-cuda-11.8.0-ubuntu-22.04-v1", k3sVersion)
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

// manifestTagReg matches released manifests only; rc and feature manifests such
// as manifest-1.4.36-rc.5 or manifest-1.1.602-fast-pca.2 are never "latest".
var manifestTagReg = regexp.MustCompile(`^manifest-(\d+)\.(\d+)\.(\d+)$`)

// maxManifestCandidates is how many of the highest tags are checked for a
// published manifest.yaml before giving up.
const maxManifestCandidates = 5

// GetLatestManifestTag returns the highest manifest-X.Y.Z tag whose release has
// a manifest.yaml. Tags are read over git first, because the REST API's
// anonymous limit of 60 requests/hour per IP blocked installs (BF-1416). The API
// is only the backup for when git's tag listing fails.
func GetLatestManifestTag() (string, error) {
	tags, err := github.ListTags(tlOwner, tlRepo)
	if err != nil {
		log.Warnf("Listing helm-charts tags over git failed, falling back to the GitHub API: %v", err)
		var apiErr error
		tags, apiErr = releaseTagsFromApi()
		if apiErr != nil {
			return "", errors.Join(err, apiErr)
		}
	}
	return latestPublishedManifestTag(tags)
}

func releaseTagsFromApi() ([]string, error) {
	releases, err := github.GetReleasesPage(tlOwner, tlRepo, 1, 100)
	if err != nil {
		return nil, err
	}
	tags := make([]string, len(releases))
	for i, release := range releases {
		tags[i] = release.TagName
	}
	return tags, nil
}

func latestPublishedManifestTag(tags []string) (string, error) {
	candidates := sortManifestTagsDesc(tags)
	if len(candidates) > maxManifestCandidates {
		candidates = candidates[:maxManifestCandidates]
	}
	for _, tag := range candidates {
		published, err := github.ReleaseAssetExists(tlOwner, tlRepo, tag, "manifest.yaml")
		if err != nil {
			return "", err
		}
		if published {
			return tag, nil
		}
	}
	return "", ErrNoTags
}

// sortManifestTagsDesc keeps the tags manifestTagReg matches, highest version
// first, comparing numbers rather than strings (1.6.10 > 1.6.9).
func sortManifestTagsDesc(tags []string) []string {
	type versionedTag struct {
		tag     string
		version [3]int
	}
	var versioned []versionedTag
	for _, tag := range tags {
		match := manifestTagReg.FindStringSubmatch(tag)
		if match == nil {
			continue
		}
		vt := versionedTag{tag: tag}
		valid := true
		for i := range vt.version {
			n, err := strconv.Atoi(match[i+1])
			if err != nil {
				valid = false
				break
			}
			vt.version[i] = n
		}
		if valid {
			versioned = append(versioned, vt)
		}
	}

	slices.SortFunc(versioned, func(a, b versionedTag) int {
		for i := range a.version {
			if c := cmp.Compare(b.version[i], a.version[i]); c != 0 {
				return c
			}
		}
		return 0
	})

	sorted := make([]string, len(versioned))
	for i, vt := range versioned {
		sorted[i] = vt.tag
	}
	return sorted
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
			return latestTag, nil
		}
	}

	return "", ErrNoTags
}

func getLocalChartVersion(chartName string, fileGetter FileGetter) (string, error) {
	path := fmt.Sprintf("charts/%s/Chart.yaml", chartName)
	b, err := fileGetter(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w (expected directory structure: <local-dir>/charts/%s/Chart.yaml)", path, err, chartName)
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
	K3sGpuImages := k3sImages

	if err != nil {
		return nil, err
	}

	// setup images
	installationImages := InstallationImages{
		K3dTools:               k3dUtilsImage,
		K3s:                    k3sImage,
		K3sGpu:                 k3sGpuImage,
		Register:               registerImage,
		Zot:                    zotImage,
		CheckDockerRequirement: checkDockerRequirement,
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
			K3sGpuImages:       K3sGpuImages,
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
