package manifest

import (
	"github.com/tensorleap/helm-charts/pkg/github"
)

// GetByTag manifest from manifest tag if empty use latest manifest release
func GetByTag(tag string) (*InstallationManifest, error) {
	if len(tag) == 0 {
		var err error
		tag, err = GetLatestManifestTag()
		if err != nil {
			return nil, err
		}
	}
	mnfBytes, err := github.GetTagArtifact(tlOwner, tlRepo, "manifest.yaml", tag)
	if err != nil {
		return nil, err
	}
	return LoadFromBytes(mnfBytes)
}
