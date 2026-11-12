package docker

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

// RunOnce runs script with `sh -ec` in a throwaway container of image and
// removes the container afterwards. binds use docker's "source:target[:ro]"
// form; a named volume as source is attached as-is. A non-zero exit returns an
// error carrying the container's last output lines.
func RunOnce(ctx context.Context, image string, binds []string, script string) error {
	_, err := runOnce(ctx, image, binds, script, false)
	return err
}

// RunOnceOutput is RunOnce returning what the script wrote to stdout.
func RunOnceOutput(ctx context.Context, image string, binds []string, script string) (string, error) {
	return runOnce(ctx, image, binds, script, true)
}

// ExtractArchive extracts the tar stream archive at dir inside a throwaway
// container of image, without running it: the docker cp mechanism, which the
// daemon writes straight to the container's disk, so a bind-mounted source
// and its file-sharing bridge stay out of the path.
func ExtractArchive(ctx context.Context, image string, binds []string, dir string, archive io.Reader) error {
	cli, id, remove, err := createContainer(ctx, image, binds, "true")
	if err != nil {
		return err
	}
	defer remove()
	if err := cli.CopyToContainer(ctx, id, dir, archive, container.CopyToContainerOptions{}); err != nil {
		return fmt.Errorf("extract archive at %s in container from %s: %w", dir, image, err)
	}
	return nil
}

func runOnce(ctx context.Context, image string, binds []string, script string, wantOutput bool) (string, error) {
	cli, id, remove, err := createContainer(ctx, image, binds, script)
	if err != nil {
		return "", err
	}
	defer remove()

	if err := cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("start container %s: %w", id, err)
	}

	statusCh, errCh := cli.ContainerWait(ctx, id, container.WaitConditionNotRunning)
	select {
	case err := <-errCh:
		return "", fmt.Errorf("wait for container %s: %w", id, err)
	case status := <-statusCh:
		if status.StatusCode != 0 {
			return "", fmt.Errorf("container from %s exited with %d: %s", image, status.StatusCode, containerOutput(ctx, cli, id, "20", true))
		}
	}
	if !wantOutput {
		return "", nil
	}
	return containerOutput(ctx, cli, id, "all", false), nil
}

// createContainer creates (does not start) a container running script with
// `sh -ec`, and returns the client, the container id and a remover.
func createContainer(ctx context.Context, image string, binds []string, script string) (Client, string, func(), error) {
	cli, err := NewClient()
	if err != nil {
		return nil, "", nil, fmt.Errorf("docker client: %w", err)
	}
	if _, _, err := cli.ImageInspectWithRaw(ctx, image); client.IsErrNotFound(err) {
		if err := PullDockerImages(cli, []string{image}); err != nil {
			return nil, "", nil, fmt.Errorf("pull %s: %w", image, err)
		}
	}
	created, err := cli.ContainerCreate(ctx,
		&container.Config{Image: image, Entrypoint: []string{"/bin/sh", "-ec"}, Cmd: []string{script}},
		&container.HostConfig{Binds: binds}, nil, nil, "")
	if err != nil {
		return nil, "", nil, fmt.Errorf("create container from %s: %w", image, err)
	}
	remove := func() {
		_ = cli.ContainerRemove(context.WithoutCancel(ctx), created.ID, container.RemoveOptions{Force: true})
	}
	return cli, created.ID, remove, nil
}

func containerOutput(ctx context.Context, cli Client, id, tail string, withStderr bool) string {
	logs, err := cli.ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: withStderr, Tail: tail})
	if err != nil {
		return err.Error()
	}
	defer logs.Close()
	var out bytes.Buffer
	_, _ = stdcopy.StdCopy(&out, &out, logs)
	return strings.TrimSpace(out.String())
}
