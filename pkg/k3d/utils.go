package k3d

import (
	"strings"

	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"gopkg.in/yaml.v3"
)

// CreateMirrorFromManifest creates a mirror config from a manifest
// Example:
// mirrors:
//
//	registry.k8s.io:
//	  endpoint:
//		  - http://k3d-tensorleap-registry:5000
//	docker.io:
//	  endpoint:
//	    - http://k3d-tensorleap-registry:5000
//	k8s.gcr.io:
//	  endpoint:
//	    - http://k3d-tensorleap-registry:5000
//	gcr.io:
//	  endpoint:
//	    - http://k3d-tensorleap-registry:5000
//	docker.elastic.co:
//	  endpoint:
//	    - http://k3d-tensorleap-registry:5000
//	quay.io:
//	  endpoint:
//	    - http://k3d-tensorleap-registry:5000
//	us-central1-docker.pkg.dev:
//	  endpoint:
//	    - http://k3d-tensorleap-registry:5000
//	        `
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
