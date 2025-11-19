package manifest

import (
	"fmt"
)

func GenerateManifestFromLocal(fileGetter FileGetter) (*InstallationManifest, error) {

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

	return NewManifest(localHelmRepoUrl, serverChartVersion, infraChartVersion, serverImages)
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
