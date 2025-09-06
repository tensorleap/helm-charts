package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/client"
	"github.com/tensorleap/helm-charts/pkg/log"
)

const OWNER = "tensorleap"

// CreateVolume ensures a named local volume exists (idempotent).
// Use labels so you can find/cleanup it later.
func CreateVolumeIfNotExists(ctx context.Context, name string, labels map[string]string) (volume.Volume, error) {
	// safety timeout
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if labels == nil {
		labels = make(map[string]string)
	}
	labels["owner"] = OWNER

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return volume.Volume{}, fmt.Errorf("docker client: %w", err)
	}

	// If it already exists, return it
	flt := filters.NewArgs()
	flt.Add("name", name)
	list, err := cli.VolumeList(cctx, volume.ListOptions{Filters: flt})
	if err != nil {
		return volume.Volume{}, fmt.Errorf("list volumes: %w", err)
	}
	for _, v := range list.Volumes {
		if v.Name == name {
			log.Infof("Volume %q already exists", name)
			return *v, nil
		}
	}

	// Create new local volume (lives inside Docker VM on macOS)
	req := volume.CreateOptions{
		Name: name,
		// Default "local" puts the data on the Linux VM's ext4, which is overlayfs-friendly.
		Driver: "local",
		Labels: labels,
	}

	log.Infof("Creating volume %q", name)
	v, err := cli.VolumeCreate(cctx, req)
	if err != nil {
		return volume.Volume{}, fmt.Errorf("create volume: %w", err)
	}
	return v, nil
}

// RemoveVolume removes a named volume. If `force` is true, it removes even if in use.
func RemoveVolume(ctx context.Context, name string, force bool) error {
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	log.Infof("Remove volume %q", name)

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}
	if err := cli.VolumeRemove(cctx, name, force); err != nil {
		return fmt.Errorf("remove volume %q: %w", name, err)
	}
	return nil
}
