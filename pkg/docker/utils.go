package docker

import (
	"net/url"
	"strings"
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