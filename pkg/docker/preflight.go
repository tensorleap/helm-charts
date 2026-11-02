package docker

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/tensorleap/helm-charts/pkg/log"
)

const PREFLIGHT_PING_TIMEOUT = 10 * time.Second

// NotReadyError is what CheckReady returns when Docker looks unusable. Its message is the whole line
// the user sees: what seems wrong, the next step, and where to read more. The raw Docker error only goes
// to the verbose log.
type NotReadyError struct{ message string }

func (e *NotReadyError) Error() string { return e.message }

var (
	ErrDockerNotInstalled = &NotReadyError{"Docker seems to be not installed. Install it: https://docs.docker.com/engine/install/"}
	ErrDockerNotRunning   = &NotReadyError{notRunningMessage()}
	ErrDockerNoPermission = &NotReadyError{"Docker seems to be not usable by this user. Add yourself to the docker group (sudo usermod -aG docker $USER), then sign out and back in: https://docs.docker.com/engine/install/linux-postinstall/"}
)

func notRunningMessage() string {
	if runtime.GOOS == "linux" {
		return "Docker seems to be not running. Start it (sudo systemctl start docker): https://docs.docker.com/engine/daemon/start/"
	}
	return "Docker seems to be not running. Start Docker Desktop: https://docs.docker.com/desktop/"
}

// tests replace these to exercise every outcome without a Docker on the machine
var (
	lookPath   = exec.LookPath
	pingDaemon = func(ctx context.Context) error {
		cli, err := NewClient()
		if err != nil {
			return err
		}
		defer func() { _ = cli.Close() }()
		_, err = cli.Ping(ctx)
		return err
	}
)

// CheckReady tells whether this user seems able to run Docker: the docker
// command is installed, the daemon answers, and the user may talk to it (on
// Linux, is in the docker group). DISABLE_DOCKER_CHECKS=true skips it, like the
// other Docker checks.
func CheckReady(ctx context.Context) error {
	if os.Getenv("DISABLE_DOCKER_CHECKS") == "true" {
		return nil
	}
	if _, err := lookPath("docker"); err != nil {
		log.VerboseLogger.Debugf("docker preflight: docker command not found: %v", err)
		return ErrDockerNotInstalled
	}

	ctx, cancel := context.WithTimeout(ctx, PREFLIGHT_PING_TIMEOUT)
	defer cancel()
	err := pingDaemon(ctx)
	if err == nil {
		return nil
	}
	log.VerboseLogger.Debugf("docker preflight: daemon ping failed: %v", err)
	// the daemon's socket is there but this user may not open it
	if errors.Is(err, fs.ErrPermission) {
		return ErrDockerNoPermission
	}
	return ErrDockerNotRunning
}

// warn is log.Warn; tests replace it to see what the user is told
var warn = func(line string) { log.Warn(line) }

// WarnIfNotReady runs CheckReady and, when Docker looks unusable, prints its one line. It never stops the
// command: a check that misreads a working Docker must not block an install, so the command goes on and
// the later Docker steps fail (or work) as before. It returns whether it warned.
func WarnIfNotReady(ctx context.Context) bool {
	err := CheckReady(ctx)
	if err == nil {
		return false
	}
	warn(err.Error())
	log.SendCloudReport("warning", "Docker check: "+err.Error(), "Running", nil)
	return true
}
