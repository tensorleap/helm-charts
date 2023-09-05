package manifest

import (
	"fmt"

	"github.com/tensorleap/helm-charts/pkg/helm/chart"
)

// GenerateManifest builds an installation manifest for the given branch and tag. If tag is empty, the latest tag is used.
func GenerateManifest(serverChartVersion string) (*InstallationManifest, error) {

	serverChartTag := ""
	if len(serverChartVersion) == 0 {
		var err error
		serverChartTag, err = GetLatestHelmChartTag()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest verison: %v", err)
		}
		serverChartVersion = GetHelmVersionFromTag(serverChartTag)
	} else {
		serverChartTag = fmt.Sprintf("tensorleap-%s", serverChartVersion)
	}

	tensorleapRepoRef := serverChartTag
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
