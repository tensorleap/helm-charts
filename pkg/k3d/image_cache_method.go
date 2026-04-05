package k3d

import "runtime"

// ImageCachingMethod represents the method used for containerd volume caching.
// With the Zot in-cluster registry, containerd volumes are always mounted for
// image persistence. The method determines whether the volume is a Docker volume
// or a local host directory.
type ImageCachingMethod string

const (
	// ImageCachingDockerVolume uses a Docker named volume for containerd storage
	ImageCachingDockerVolume ImageCachingMethod = "docker-volume"
	// ImageCachingLocalVolume uses a local host directory for containerd storage
	ImageCachingLocalVolume ImageCachingMethod = "local-volume"
)

// GetDefaultImageCachingMethod returns the default containerd volume method based on OS.
func GetDefaultImageCachingMethod(isAirgap bool) ImageCachingMethod {
	switch runtime.GOOS {
	case "linux":
		return ImageCachingLocalVolume
	default:
		return ImageCachingDockerVolume
	}
}

// IsImageCachingMethodAvailable checks if the specified caching method is available for the current environment
func IsImageCachingMethodAvailable(method ImageCachingMethod, isAirgap bool) bool {
	switch method {
	case ImageCachingDockerVolume:
		return true
	case ImageCachingLocalVolume:
		return runtime.GOOS == "linux"
	default:
		return false
	}
}
