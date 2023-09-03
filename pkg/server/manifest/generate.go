package manifest

import (
	"fmt"

	"github.com/tensorleap/helm-charts/pkg/helm/chart"
)

// GenerateManifest builds an installation manifest for the given branch and tag. If tag is empty, the latest tag is used.
func GenerateManifest(branch, tag string) (*InstallationManifest, error) {

	if len(branch) == 0 && len(tag) == 0 {
		var err error
		tag, err = GetLatestTag()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest verison: %v", err)
		}
	}

	var serverChartVersion string
	if len(tag) > 0 {
		serverChartVersion = GetHelmVersionFromTag(tag)
	}

	tensorleapRepoRef := getTensorleapRepoRef(branch, tag)
	mnf, err := getManifestWithBasicInfo(tensorleapRepoRef)
	if err != nil {
		return nil, err
	}

	serverImages, err := getTensorleapImages(tensorleapRepoRef)
	if err != nil {
		return nil, err
	}

	version, err := chart.GetVersion(mnf.ServerHelmChart.RepoUrl, mnf.ServerHelmChart.ChartName, serverChartVersion)
	if err != nil {
		return nil, err
	}
	helmVersion := version.Version

	mnf.Images.ServerImages = serverImages
	mnf.ServerHelmChart.Version = helmVersion

	return mnf, nil
}
