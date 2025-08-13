package docker

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/tensorleap/helm-charts/pkg/utils"
)

func GetDockerRegistry(image string) string {
	// Try parsing as a URL first
	if u, err := url.Parse(image); err == nil && u.Host != "" {
		return u.Host
	}

	// Otherwise treat as Docker image name
	parts := strings.Split(image, "/")
	if len(parts) > 1 && strings.Contains(parts[0], ".") {
		// First part looks like a registry (contains a dot)
		return parts[0]
	}

	// Default Docker registry if none specified
	return "docker.io"
}

type ImagePullerLimiter struct {
	limitPerRegistry map[string]*utils.DynamicLimiter
}

func (limiter *ImagePullerLimiter) GetLimiter(image string) (*utils.DynamicLimiter, error) {
	registry := GetDockerRegistry(image)
	registryLimiter, ok := limiter.limitPerRegistry[registry]
	if !ok {
		return nil, fmt.Errorf("registry %s not found", registry)
	}
	return registryLimiter, nil
}

func NewPullerLimiter(images []string) *ImagePullerLimiter {
	limiter := &ImagePullerLimiter{
		limitPerRegistry: make(map[string]*utils.DynamicLimiter),
	}
	for _, image := range images {
		registry := GetDockerRegistry(image)
		if _, ok := limiter.limitPerRegistry[registry]; !ok {
			limiter.limitPerRegistry[registry] = utils.NewDynamicLimiter(10)
		}
	}
	return limiter
}
