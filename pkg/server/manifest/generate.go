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
	IsGetLatestVersions := len(serverChartVersion) == 0 || len(infraChartVersion) == 0
	if IsGetLatestVersions {
		var err error
		serverChartTag, err = GetLatestServerHelmChartTag()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest version: %v", err)
		}
		serverChartVersion = GetHelmVersionFromTag(serverChartTag)
		infraChartVersion, err = GetLatestInfraHelmChartVersion()
		if err != nil {
			return nil, fmt.Errorf("failed to get latest version: %v", err)
		}
	} else {
		serverChartTag = fmt.Sprintf("tensorleap-%s", serverChartVersion)
	}

	tensorleapRepoRef := serverChartTag
	fileGetter := buildRemoteFileGetter(tensorleapRepoRef)

	serverImages, err := getTensorleapImages(fileGetter)
	if err != nil {
		return nil, err
	}

	return NewManifest(helmRepoUrl, serverChartVersion, infraChartVersion, serverImages)
}
