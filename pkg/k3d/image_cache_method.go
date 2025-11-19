package k3d

import "runtime"

// ImageCachingMethod represents the method used for image caching
type ImageCachingMethod string

const (
	// ImageCachingDockerVolume uses Docker volume to containerd for image caching
	ImageCachingDockerVolume ImageCachingMethod = "docker-volume"
	// ImageCachingLocalVolume uses volume from local computer to containerd for image caching
	ImageCachingLocalVolume ImageCachingMethod = "local-volume"
	// ImageCachingRegistry uses registry cached images for image caching
	ImageCachingRegistry ImageCachingMethod = "registry"
)

// GetDefaultImageCachingMethod returns the default image caching method based on environment
func GetDefaultImageCachingMethod(isAirgap bool) ImageCachingMethod {
	if isAirgap {
		return ImageCachingRegistry
	}

	switch runtime.GOOS {
	case "darwin":
		return ImageCachingDockerVolume
	case "linux":
		return ImageCachingLocalVolume
	default:
		return ImageCachingDockerVolume
	}
}

// IsImageCachingMethodAvailable checks if the specified caching method is available for the current environment
func IsImageCachingMethodAvailable(method ImageCachingMethod, isAirgap bool) bool {
	switch method {
	case ImageCachingRegistry:
		return true
	case ImageCachingDockerVolume:
		return !isAirgap
	case ImageCachingLocalVolume:
		return !isAirgap && runtime.GOOS == "linux"
	default:
		return false
	}
}
