package manifest

import (
	"fmt"
	"path/filepath"
)

// GenerateManifestFromLocal generates a manifest from local helm charts.
// baseDir is the root directory containing the charts/ folder. If empty, uses current directory.
func GenerateManifestFromLocal(fileGetter FileGetter, baseDir string) (*InstallationManifest, error) {

	serverImages, err := getTensorleapImages(fileGetter)
	if err != nil {
		return nil, err
	}

	serverChartVersion, err := getLocalChartVersion(tensorleapChartName, fileGetter)
	if err != nil {
		return nil, err
	}
	infraChartVersion, err := getLocalChartVersion(tensorleapInfraChartName, fileGetter)
	if err != nil {
		return nil, err
	}

	// Construct full path to charts directory
	chartsPath := filepath.Join(baseDir, localHelmRepoUrl)

	return NewManifest(chartsPath, serverChartVersion, infraChartVersion, serverImages)
}

func GenerateManifestFromRemote(serverChartVersion, infraChartVersion string) (*InstallationManifest, error) {
	serverChartTag := ""
	if len(serverChartVersion) == 0 {
		var err error
		serverChartTag, err = GetLatestServerHelmChartTag()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest version: %v", err)
		}
		serverChartVersion = GetHelmVersionFromTag(serverChartTag)

	} else {
		serverChartTag = fmt.Sprintf("tensorleap-%s", serverChartVersion)
	}
	if len(infraChartVersion) == 0 {
		var err error
		infraChartVersion, err = GetLatestInfraHelmChartVersion()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest version: %v", err)
		}
	}

	tensorleapRepoRef := serverChartTag
	fileGetter := buildRemoteFileGetter(tensorleapRepoRef)

	serverImages, err := getTensorleapImages(fileGetter)
	if err != nil {
		return nil, err
	}

	return NewManifest(helmRepoUrl, serverChartVersion, infraChartVersion, serverImages)
}
