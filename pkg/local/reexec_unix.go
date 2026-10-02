//go:build !windows
// +build !windows

package local

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// sharedGroupReexecEnv marks a process already re-executed under sg, so a
// membership that still is not effective cannot loop forever.
const sharedGroupReexecEnv = "TL_SHARED_GROUP_REEXEC"

// reexecWithSharedGroup replaces the current process with the same command run
// under `sg <group> -c`, which starts it with the shared group as its effective
// group. sg is setuid root and consults the account database, so it works the
// moment usermod has added the user, without a new login. It only returns on
// failure.
func reexecWithSharedGroup() error {
	if os.Getenv(sharedGroupReexecEnv) != "" {
		return errors.New("group membership is still not active after re-executing under sg; log out and back in, then retry")
	}
	sgPath, err := exec.LookPath("sg")
	if err != nil {
		return fmt.Errorf("cannot activate group %s for this run (sg not found); log out and back in, then retry: %w", SHARED_GROUP_NAME, err)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot re-execute under sg: %w", err)
	}
	cmdline := shellJoin(append([]string{exe}, os.Args[1:]...))
	env := append(os.Environ(), sharedGroupReexecEnv+"=1")
	if err := syscall.Exec(sgPath, []string{"sg", SHARED_GROUP_NAME, "-c", cmdline}, env); err != nil {
		return fmt.Errorf("re-executing under sg %s failed: %w", SHARED_GROUP_NAME, err)
	}
	return nil
}
