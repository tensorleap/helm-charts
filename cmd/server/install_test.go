package server

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/docker"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
)

// G89: install, reinstall and upgrade check Docker first, before they ask or touch anything, and go on
// whatever the check says: it is advice, never a stop
func TestDockerCheckRunsFirstAndNeverStopsTheCommand(t *testing.T) {
	log.DisableReporting() // no cloud report from a test
	commands := map[string]func() error{
		"install": func() error {
			_, err := RunInstallCmd(NewInstallCmd(), &InstallFlags{})
			return err
		},
		"reinstall": func() error {
			_, err := RunReinstallCmd(NewReinstallCmd(), &ReinstallFlags{}, false)
			return err
		},
		"upgrade": func() error {
			_, err := RunUpgradeCmd(&cobra.Command{}, &UpgradeFlags{})
			return err
		},
	}
	for name, run := range commands {
		t.Run(name, func(t *testing.T) {
			dataDir := t.TempDir()
			t.Setenv(local.DATA_DIR_ENV_NAME, dataDir)
			// no docker (and no mkdir): the check warns, and the next step, the data dir setup, fails
			t.Setenv("PATH", t.TempDir())
			t.Setenv("DISABLE_DOCKER_CHECKS", "")

			checked := false
			prev := warnIfDockerNotReady
			warnIfDockerNotReady = func(ctx context.Context) {
				entries, _ := os.ReadDir(dataDir)
				if len(entries) != 0 {
					t.Errorf("the data dir was set up before the Docker check: %v", entries)
				}
				checked = docker.WarnIfNotReady(ctx)
			}
			t.Cleanup(func() { warnIfDockerNotReady = prev })

			err := run()
			if !checked {
				t.Fatal("the Docker check did not run, or did not warn without docker on PATH")
			}
			var notReady *docker.NotReadyError
			if err == nil || errors.As(err, &notReady) {
				t.Fatalf("the command must go on past the check (and fail later here, without mkdir); got %v", err)
			}
		})
	}
}
