package docker

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// RunOnce runs script with `sh -ec` in a throwaway container of image and
// removes the container afterwards. binds use docker's "source:target[:ro]"
// form; a named volume as source is attached as-is. A non-zero exit returns an
// error carrying the container's last output lines.
func RunOnce(ctx context.Context, image string, binds []string, script string) error {
	cli, err := NewClient()
	if err != nil {
		return fmt.Errorf("docker client: %w", err)
	}

	created, err := cli.ContainerCreate(ctx,
		&container.Config{Image: image, Entrypoint: []string{"/bin/sh", "-ec"}, Cmd: []string{script}},
		&container.HostConfig{Binds: binds}, nil, nil, "")
	if err != nil {
		return fmt.Errorf("create container from %s: %w", image, err)
	}
	defer func() {
		_ = cli.ContainerRemove(context.WithoutCancel(ctx), created.ID, container.RemoveOptions{Force: true})
	}()

	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("start container %s: %w", created.ID, err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, created.ID, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return fmt.Errorf("wait for container %s: %w", created.ID, err)
	case status := <-statusCh:
		if status.StatusCode == 0 {
			return nil
		}
		return fmt.Errorf("container from %s exited with %d: %s", image, status.StatusCode, containerOutput(ctx, cli, created.ID))
	}
}

func containerOutput(ctx context.Context, cli Client, id string) string {
	logs, err := cli.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: "20"})
	if err != nil {
		return err.Error()
	}
	defer logs.Close()
	var out bytes.Buffer
	_, _ = stdcopy.StdCopy(&out, &out, logs)
	return strings.TrimSpace(out.String())
}
