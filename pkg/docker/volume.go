package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/moby/moby/client"
	"github.com/moby/moby/api/types/volume"
)

const OWNER = "tensorleap"

// CreateVolume ensures a named local volume exists (idempotent).
// Use labels so you can find/cleanup it later.
func CreateVolumeIfNotExists(ctx context.Context, name string, labels map[string]string) (*volume.Volume, error) {
	// safety timeout
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if labels == nil {
		labels = make(map[string]string)
	}
	labels["owner"] = OWNER

	cli, err := NewClient()
	if err != nil {
		return nil, fmt.Errorf("docker client: %w", err)
	}

	// If it already exists, return it
	filters := client.Filters{}
	filters.Add("name", name)
	list, err := cli.VolumeList(cctx, client.VolumeListOptions{Filters: filters})
	if err != nil {
		return nil, fmt.Errorf("list volumes: %w", err)
	}
	for _, v := range list.Items {
		if v.Name == name {
			log.Infof("Volume %q already exists", name)
			return &v, nil
		}
	}

	// Create new local volume (lives inside Docker VM on macOS)
	req := client.VolumeCreateOptions{
		Name: name,
		// Default "local" puts the data on the Linux VM's ext4, which is overlayfs-friendly.
		Driver: "local",
		Labels: labels,
	}

	log.Infof("Creating volume %q", name)
	v, err := cli.VolumeCreate(cctx, req)
	if err != nil {
		return nil, fmt.Errorf("create volume: %w", err)
	}
	return &v.Volume, nil
}

// RemoveVolume removes a named volume. If `force` is true, it removes even if in use.
func RemoveVolume(ctx context.Context, name string, force bool) error {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	log.Infof("Remove volume %q", name)

	cli, err := NewClient()
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}
	_, err = cli.VolumeRemove(cctx, name, client.VolumeRemoveOptions{Force: force})
	if err != nil {
		return fmt.Errorf("remove volume %q: %w", name, err)
	}
	return nil
}
