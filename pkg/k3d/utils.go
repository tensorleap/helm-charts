package k3d

import (
	"fmt"
	"strings"

	"github.com/tensorleap/helm-charts/pkg/helm"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
	"gopkg.in/yaml.v3"
)

// CreateMirrorFromManifest generates a k3s registries.yaml for the in-cluster
// Zot registry. The internalPort is the Zot pod's hostPort inside the k3s node
// (typically 5000), NOT the Docker-host-mapped port (e.g. 5699).
//
// In airgap mode, ALL upstream registries are mirrored through Zot so that
// containerd pulls every image from the local registry (no internet needed).
//
// In online mode, only the in-cluster service name (tensorleap-registry:5000)
// is mirrored so that DnD-pushed images can be pulled by pods. Upstream images
// are fetched directly by containerd, avoiding storage duplication in Zot.
func CreateMirrorFromManifest(mfs *manifest.InstallationManifest, internalPort uint, isAirgap bool) (string, error) {
	zotEndpoint := fmt.Sprintf("http://127.0.0.1:%d", internalPort)
	mirrors := make(map[string]interface{})

	if isAirgap {
		mirrors["docker.io"] = map[string]interface{}{
			"endpoint": []string{zotEndpoint},
		}
		for _, image := range mfs.GetRegisterImages() {
			imageHost := strings.Split(image, "/")[0]
			if mirrors[imageHost] == nil {
				mirrors[imageHost] = map[string]interface{}{
					"endpoint": []string{zotEndpoint},
				}
			}
		}
	}

	// Always mirror the in-cluster service name so DnD/BuildKit pushes
	// and subsequent pod pulls resolve through the Zot hostPort.
	mirrors["tensorleap-registry:5000"] = map[string]interface{}{
		"endpoint": []string{zotEndpoint},
	}

	// Always mirror public.ecr.aws so that built engine-generic images
	// (pushed to Zot by pippin) are found when pods reference the ECR path.
	// Containerd falls back to the real ECR for images not in Zot.
	mirrors["public.ecr.aws"] = map[string]interface{}{
		"endpoint": []string{zotEndpoint},
	}

	registryConfig := map[string]interface{}{
		"mirrors": mirrors,
		"configs": map[string]interface{}{
			fmt.Sprintf("127.0.0.1:%d", internalPort): map[string]interface{}{
				"tls": map[string]interface{}{
					"insecure_skip_verify": true,
				},
			},
		},
	}

	yamlBytes, err := yaml.Marshal(registryConfig)
	if err != nil {
		return "", err
	}

	return string(yamlBytes), nil
}

// registryURLMap maps upstream registry hosts to their full HTTPS URLs for Zot sync.
// Docker Hub uses registry-1.docker.io as the actual API endpoint.
var registryURLMap = map[string]string{
	"docker.io":         "https://registry-1.docker.io",
	"public.ecr.aws":    "https://public.ecr.aws",
	"registry.k8s.io":   "https://registry.k8s.io",
	"docker.elastic.co": "https://docker.elastic.co",
	"quay.io":           "https://quay.io",
	"ghcr.io":           "https://ghcr.io",
	"nvcr.io":           "https://nvcr.io",
}

// BuildZotSyncRegistries generates the Zot sync registries configuration from the manifest's
// image list. Each upstream registry host gets one entry with prefix filters derived from the
// image paths that belong to that host.
func BuildZotSyncRegistries(mfs *manifest.InstallationManifest) []helm.ZotSyncRegistry {
	allImages := mfs.GetRegisterImages()

	// Group image path prefixes by registry host
	hostPrefixes := make(map[string]map[string]struct{})
	for _, image := range allImages {
		parts := strings.SplitN(image, "/", 2)
		if len(parts) < 2 {
			continue
		}
		host := parts[0]
		// Extract the org/project prefix (everything before the image name)
		pathParts := strings.Split(parts[1], "/")
		var prefix string
		if len(pathParts) > 1 {
			prefix = strings.Join(pathParts[:len(pathParts)-1], "/") + "/**"
		} else {
			prefix = "**"
		}

		if hostPrefixes[host] == nil {
			hostPrefixes[host] = make(map[string]struct{})
		}
		hostPrefixes[host][prefix] = struct{}{}
	}

	var registries []helm.ZotSyncRegistry
	for host, prefixes := range hostPrefixes {
		upstreamURL, ok := registryURLMap[host]
		if !ok {
			upstreamURL = fmt.Sprintf("https://%s", host)
		}

		var content []helm.ZotSyncContent
		for prefix := range prefixes {
			content = append(content, helm.ZotSyncContent{Prefix: prefix})
		}

		registries = append(registries, helm.ZotSyncRegistry{
			URLs:      []string{upstreamURL},
			Content:   content,
			OnDemand:  true,
			TLSVerify: true,
		})
	}

	return registries
}
