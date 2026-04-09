package k3d

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/tensorleap/helm-charts/pkg/log"
)

const (
	compressionMultiplier = 3
	storageBufferGB       = 5
	imageSizeConcurrency  = 2
	perImageTimeout       = 30 * time.Second
	manifestFetchRetries  = 3
	manifestRetryBackoff  = 2 * time.Second
)

// EstimateInstallSize queries remote registries for the compressed layer sizes
// of all images and returns an estimated on-disk (uncompressed) size in bytes.
// It uses compressed_size * compressionMultiplier as the estimate.
// Individual image failures are logged and skipped. If no image sizes could be
// resolved, an error is returned so the caller can fall back to a static default.
func EstimateInstallSize(images []string) (int64, error) {
	type result struct {
		image string
		size  int64
		err   error
	}

	results := make(chan result, len(images))
	sem := make(chan struct{}, imageSizeConcurrency)
	var wg sync.WaitGroup

	platform := v1.Platform{OS: "linux", Architecture: "amd64"}

	for _, img := range images {
		wg.Add(1)
		go func(img string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			size, err := getCompressedImageSize(img, &platform)
			results <- result{image: img, size: size, err: err}
		}(img)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var totalCompressed int64
	var failures int
	for r := range results {
		if r.err != nil {
			log.Warnf("Failed to query image size for %s: %v", r.image, r.err)
			failures++
			continue
		}
		totalCompressed += r.size
	}

	if totalCompressed == 0 {
		return 0, fmt.Errorf("failed to resolve compressed size for any of the %d images", len(images))
	}
	if failures > 0 {
		log.Warnf("Could not resolve %d/%d image sizes; estimate may be lower than actual", failures, len(images))
	}

	estimated := totalCompressed * compressionMultiplier
	return estimated, nil
}

func getCompressedImageSize(img string, platform *v1.Platform) (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), perImageTimeout)
	defer cancel()

	ref, err := name.ParseReference(img)
	if err != nil {
		return 0, fmt.Errorf("parse reference: %w", err)
	}

	for attempt := 1; attempt <= manifestFetchRetries; attempt++ {
		size, err := fetchLayerSizes(ctx, ref, platform)
		if err == nil {
			return size, nil
		}
		if !isRateLimitError(err) || attempt == manifestFetchRetries {
			return 0, err
		}
		select {
		case <-ctx.Done():
			return 0, fmt.Errorf("context expired: %w", ctx.Err())
		case <-time.After(manifestRetryBackoff * time.Duration(attempt)):
		}
	}

	return 0, fmt.Errorf("unreachable")
}

func fetchLayerSizes(ctx context.Context, ref name.Reference, platform *v1.Platform) (int64, error) {
	desc, err := remote.Get(ref, remote.WithContext(ctx), remote.WithPlatform(*platform))
	if err != nil {
		return 0, fmt.Errorf("fetch descriptor: %w", err)
	}

	image, err := desc.Image()
	if err != nil {
		return 0, fmt.Errorf("resolve image: %w", err)
	}

	layers, err := image.Layers()
	if err != nil {
		return 0, fmt.Errorf("get layers: %w", err)
	}

	var total int64
	for _, layer := range layers {
		size, err := layer.Size()
		if err != nil {
			return 0, fmt.Errorf("get layer size: %w", err)
		}
		total += size
	}

	return total, nil
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "TOOMANYREQUESTS") || strings.Contains(msg, "429")
}
