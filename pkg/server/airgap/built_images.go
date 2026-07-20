package airgap

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/tensorleap/helm-charts/pkg/log"
)

const builtImagesRepo = "tensorleap/engine-generic"

// Pippin tags built dependency images with a 40-hex sha1 of the dependency
// files; this filter excludes base images, branch tags and the buildcache
// manifest (which docker cannot pull).
var builtImageTagPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

func CollectBuiltImages(registry string) []string {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://%s/v2/%s/tags/list", registry, builtImagesRepo))
	if err != nil {
		log.Warnf("No local registry reachable at %s, packing without built dependency images: %v", registry, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Warnf("No %s repository at %s (status %d), packing without built dependency images", builtImagesRepo, registry, resp.StatusCode)
		return nil
	}

	var tagsList struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tagsList); err != nil {
		log.Warnf("Failed to parse tags from %s, packing without built dependency images: %v", registry, err)
		return nil
	}

	images := []string{}
	for _, tag := range tagsList.Tags {
		if builtImageTagPattern.MatchString(tag) {
			images = append(images, fmt.Sprintf("%s/%s:%s", registry, builtImagesRepo, tag))
		}
	}
	return images
}
