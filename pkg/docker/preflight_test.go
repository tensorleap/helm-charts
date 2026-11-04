package docker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"testing"

	"github.com/tensorleap/helm-charts/pkg/log"
)

func TestMain(m *testing.M) {
	log.DisableReporting() // no cloud reports from tests
	m.Run()
}

// G89: when Docker looks unusable the user gets one plain line, and the command goes on; a working
// Docker prints nothing
func TestWarnIfNotReady(t *testing.T) {
	installed := func(string) (string, error) { return "/usr/bin/docker", nil }
	missing := func(string) (string, error) { return "", exec.ErrNotFound }
	rawDaemonDown := errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?")
	// what the Docker client returns when the socket exists but this user may not open it
	rawNoPermission := fmt.Errorf("permission denied while trying to connect to the Docker daemon socket: %w", syscall.EACCES)

	cases := []struct {
		name     string
		lookPath func(string) (string, error)
		ping     error
		want     error // nil: nothing printed
	}{
		{name: "docker works", lookPath: installed},
		{name: "docker not installed", lookPath: missing, want: ErrDockerNotInstalled},
		{name: "daemon not running", lookPath: installed, ping: rawDaemonDown, want: ErrDockerNotRunning},
		{name: "user not in the docker group", lookPath: installed, ping: rawNoPermission, want: ErrDockerNoPermission},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("DISABLE_DOCKER_CHECKS", "")
			stubPreflight(t, c.lookPath, c.ping)
			var printed []string
			prevWarn := warn
			warn = func(line string) { printed = append(printed, line) }
			t.Cleanup(func() { warn = prevWarn })

			warned := WarnIfNotReady(context.Background())
			if c.want == nil {
				if warned || len(printed) != 0 {
					t.Fatalf("a working Docker printed %q", printed)
				}
				return
			}
			if !warned || len(printed) != 1 || printed[0] != c.want.Error() {
				t.Fatalf("printed %q, want the one line %q", printed, c.want.Error())
			}
			line := printed[0]
			if !strings.HasPrefix(line, "Docker seems to be ") || strings.Contains(line, "\n") || !strings.Contains(line, "https://") {
				t.Errorf("want one \"Docker seems to be …\" line with a link, got %q", line)
			}
			if c.ping != nil && strings.Contains(line, c.ping.Error()) {
				t.Errorf("the raw Docker error reached the user: %q", line)
			}
		})
	}
}

func TestCheckReadySkippedWhenDockerChecksAreDisabled(t *testing.T) {
	t.Setenv("DISABLE_DOCKER_CHECKS", "true")
	stubPreflight(t, func(string) (string, error) { return "", exec.ErrNotFound }, nil)
	if err := CheckReady(context.Background()); err != nil {
		t.Fatalf("CheckReady with DISABLE_DOCKER_CHECKS=true = %v, want nil", err)
	}
}

func stubPreflight(t *testing.T, look func(string) (string, error), ping error) {
	t.Helper()
	prevLook, prevPing := lookPath, pingDaemon
	lookPath = look
	pingDaemon = func(context.Context) error { return ping }
	t.Cleanup(func() { lookPath, pingDaemon = prevLook, prevPing })
}
