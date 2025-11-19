package docker

import (
	"fmt"
	"strings"

	ggcr "github.com/google/go-containerregistry/pkg/name"
	"github.com/tensorleap/helm-charts/pkg/utils"
)

func GetDockerRegistry(image string) (string, error) {
	ref, err := ggcr.ParseReference(image)
	if err != nil {
		return "", err
	}

	registry := ref.Context().Registry.Name()

	// Normalize Docker Hub registry name
	if registry == "index.docker.io" {
		return "docker.io", nil
	}

	return registry, nil
}

type ImagePullerLimiter struct {
	limitPerRegistry map[string]*utils.DynamicLimiter
}

func (limiter *ImagePullerLimiter) GetLimiter(image string) (*utils.DynamicLimiter, error) {
	registry, err := GetDockerRegistry(image)
	if err != nil {
		return nil, err
	}
	registryLimiter, ok := limiter.limitPerRegistry[registry]
	if !ok {
		return nil, fmt.Errorf("registry %s not found", registry)
	}
	return registryLimiter, nil
}

func FindTensorleapRegistry(images []string) (string, error) {
	for _, image := range images {
		if strings.Contains(image, "node-server:") {
			registry, err := GetDockerRegistry(image)
			if err != nil {
				return "", err
			}
			return registry, nil
		}
	}
	return "", nil
}

const LIMIT_PER_REGISTRY = 10
const TENSORLEAP_LIMIT_PER_REGISTRY = 2

func NewPullerLimiter(images []string) (*ImagePullerLimiter, error) {
	limiter := &ImagePullerLimiter{
		limitPerRegistry: make(map[string]*utils.DynamicLimiter),
	}

	tensorleapRegistry, err := FindTensorleapRegistry(images)
	if err != nil {
		return nil, err
	}

	for _, image := range images {
		registry, err := GetDockerRegistry(image)
		if err != nil {
			return nil, err
		}
		if _, ok := limiter.limitPerRegistry[registry]; !ok {
			limit := LIMIT_PER_REGISTRY
			if registry == tensorleapRegistry {
				limit = TENSORLEAP_LIMIT_PER_REGISTRY
			}
			limiter.limitPerRegistry[registry] = utils.NewDynamicLimiter(limit)
		}
	}
	return limiter, nil
}
