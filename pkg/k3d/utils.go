package k3d

import (
	"strings"

	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"gopkg.in/yaml.v3"
)

func CreateMirrorFromManifest(mfs *manifest.InstallationManifest, registryUrl string) (string, error) {
	images := mfs.GetRegisterImages()
	mirrors := make(map[string]interface{})

	// Add default mirror
	mirrors["docker.io"] = map[string]interface{}{
		"endpoint": []string{registryUrl},
	}

	for _, image := range images {
		imageHost := strings.Split(image, "/")[0]

		if mirrors[imageHost] == nil {
			mirrors[imageHost] = map[string]interface{}{
				"endpoint": []string{registryUrl},
			}
		}
	}

	mirrorConfig := map[string]interface{}{
		"mirrors": mirrors,
	}

	yamlBytes, err := yaml.Marshal(mirrorConfig)
	if err != nil {
		return "", err
	}

	return string(yamlBytes), nil
}
