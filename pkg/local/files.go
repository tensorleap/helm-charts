package local

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/tensorleap/helm-charts/pkg/log"
)

func CleanupTempFile(file *os.File) {
	file.Close()
	os.Remove(file.Name())
}

func RunCommand(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run command: %v", err)
	}
	return nil
}

// EnsureDirExists makes sure path exists as a world-writable directory so any
// local user can use the shared single-node install tree (e.g. a second
// operator reinstalling: ubuntu installs, ssm-user maintains). A freshly
// created dir is chmod'ed 0777 (with sudo if the parent needed it). An
// already-existing dir is healed via os.Chmod when we own it; when we don't,
// sudo is used only for real directories that resolve to inside the Tensorleap
// data dir. Arbitrary paths reach this function (user-supplied dataset
// volumes), and the data dir tree is world-writable so another local user can
// plant a symlink in it — an unconditional sudo chmod would let either trick a
// sudo-capable operator into making any directory on the host world-writable.
// When an existing dir can't be healed, warn and continue — it may still be
// usable as-is, and failing would break installs that work today.
func EnsureDirExists(path string) error {
	status, err := CheckDirectoryStatus(path)
	if err != nil {
		return err
	}

	if !status.Exists {
		createdWithSudo := !status.CanCreateOnParentDirectory
		if err := runMaybeSudo(createdWithSudo, "mkdir", "-p", path); err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
		if err := runMaybeSudo(createdWithSudo, "chmod", "777", path); err != nil {
			return fmt.Errorf("failed to set directory permissions on %s: %v", path, err)
		}
		return nil
	}

	// Only heal a real directory, and never follow a symlink: os.Chmod would
	// chmod the link target, which could be anywhere.
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}
	if info.Mode().Perm()&0o002 != 0 {
		return nil // already world-writable
	}
	if err := os.Chmod(path, 0o777); err == nil {
		return nil
	}
	// Not ours. Escalate only inside the data dir, on the symlink-resolved
	// path so a planted link can't redirect the chmod outside it.
	if resolved, ok := resolveInsideDataDir(path); ok {
		if err := RunCommand("sudo", "chmod", "777", resolved); err == nil {
			return nil
		}
	}
	log.Warnf("Could not make %s world-writable. Other local users may be unable to use it; have its owner or an admin run: sudo chmod 777 %s", path, path)
	return nil
}

// resolveInsideDataDir resolves symlinks in path and reports whether it is an
// existing directory within the Tensorleap data dir (inclusive).
func resolveInsideDataDir(path string) (string, bool) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false
	}
	dataDir, err := filepath.EvalSymlinks(GetServerDataDir())
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(dataDir, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	info, err := os.Lstat(resolved)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return resolved, true
}

func runMaybeSudo(useSudo bool, args ...string) error {
	if useSudo {
		args = append([]string{"sudo"}, args...)
	}
	return RunCommand(args...)
}

// MoveOrCopyDirectory tries to rename the directory, on failure tries to copy
func MoveOrCopyDirectory(srcStatus, dstStatus FileSystemStatus) error {
	// Ensure the parent directory of the destination exists. Only create it
	// when missing — an existing parent may be a system dir (/opt, /var/lib,
	// a home dir) that must not be made world-writable.
	dstParent := filepath.Dir(dstStatus.Path)
	parentStatus, err := CheckDirectoryStatus(dstParent)
	if err != nil {
		return err
	}
	if !parentStatus.Exists {
		if err := EnsureDirExists(dstParent); err != nil {
			return err
		}
	}

	if !srcStatus.Exists {
		return fmt.Errorf("source directory does not exist: %s", srcStatus.Path)
	}

	isStorageMovePermissionNeeded := !srcStatus.CanWrite || !srcStatus.CanCreateOnParentDirectory || !dstStatus.CanCreateOnParentDirectory

	mvArgs := []string{"mv", srcStatus.Path, dstStatus.Path}
	if isStorageMovePermissionNeeded {
		log.Warn("Move operation requires sudo permissions")
		mvArgs = append([]string{"sudo"}, mvArgs...)
	}
	if err := RunCommand(mvArgs...); err != nil {
		log.Warnf("Failed to move directory, attempting to copy: %v", err)
		cpArgs := []string{"cp", "-r", srcStatus.Path, dstStatus.Path}
		if isStorageMovePermissionNeeded {
			log.Warn("Copy operation requires sudo permissions")
			cpArgs = append([]string{"sudo"}, cpArgs...)
		}
		if err := RunCommand(cpArgs...); err != nil {
			return fmt.Errorf("copy operation failed: %v", err)
		}

		if err := RemoveDirectory(srcStatus); err != nil {
			log.Warnf("failed to remove src directory: %v", err)
		}
	}

	return nil
}

// FileSystemStatus holds information about the existence of a directory and the permissions related to it.
type FileSystemStatus struct {
	Path                       string
	Exists                     bool
	CanCreateOnParentDirectory bool
	CanRead                    bool
	CanWrite                   bool
	CanExecute                 bool
}

// CheckDirectoryStatus checks if a directory exists and determines the permissions for creating, reading, and writing.
func CheckDirectoryStatus(path string) (FileSystemStatus, error) {
	var status FileSystemStatus = FileSystemStatus{Path: path}
	info, err := os.Stat(path)

	if canCreate, err := canCreateDirectory(path); err == nil {
		status.CanCreateOnParentDirectory = canCreate
	}
	if err != nil {
		if os.IsNotExist(err) {
			if status.CanCreateOnParentDirectory {
				status.CanRead = true
				status.CanWrite = true
			}
			return status, nil
		}
		if os.IsPermission(err) {
			// Check if we can stat the directory using sudo.
			status.Exists, err = sudoStat(path)
			if err != nil {
				return status, fmt.Errorf("failed to stat directory: %v", err)
			}
			return status, nil
		}
		return status, err
	}

	// The directory exists, set Exists and check read and write permissions.
	status.Exists = true

	err = SetPermissionFromFileInfo(&status, info)
	if err != nil {
		return status, fmt.Errorf("failed to check permissions: %v", err)
	}

	return status, nil
}

// sudoStat checks if a directory exists using sudo and interprets if the directory does not exist.
func sudoStat(path string) (bool, error) {
	cmd := exec.Command("sudo", "stat", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if the error output indicates that the directory does not exist
		if strings.Contains(string(output), "cannot stat") {
			return false, nil // Indicates directory does not exist
		}
		return false, fmt.Errorf("failed to stat directory with sudo: %s, error: %v", output, err)
	}
	return true, nil // Directory exists
}

func RemoveDirectory(status FileSystemStatus) error {
	if !status.Exists {
		return nil
	}

	rmArgs := []string{"rm", "-rf", status.Path}
	if !status.CanWrite || !status.CanCreateOnParentDirectory {
		rmArgs = append([]string{"sudo"}, rmArgs...)
	}
	if err := RunCommand(rmArgs...); err != nil {
		return fmt.Errorf("failed to remove directory: %v", err)
	}

	return nil
}

// canCreateDirectory checks if the process can create a directory at the specified path.
func canCreateDirectory(dirPath string) (bool, error) {
	checkPath := path.Dir(dirPath)

	for {
		dirInfo, err := os.Stat(checkPath)
		if err == nil {
			// If the directory exists, check if we can write to it.
			var p FileSystemStatus
			err := SetPermissionFromFileInfo(&p, dirInfo)
			return p.CanWrite, err
		} else if os.IsNotExist(err) {
			// do nothing
		} else if os.IsPermission(err) {
			return false, nil
		} else {
			return false, err
		}
		if checkPath == "/" || checkPath == "." {
			// Reached the root or current directory without finding an existing directory.
			return false, fmt.Errorf("reached the root or a non-existent segment without finding an existing directory")
		}

		checkPath = path.Dir(checkPath)
	}
}

func FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to stat file: %v", err)
	}
	return true, nil
}

func DownloadIntoFile(url string, file *os.File) error {
	res, err := http.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("failed downloading (%s): %v", url, res.StatusCode)
	}
	_, err = io.Copy(file, res.Body)
	if err != nil {
		return err
	}
	_, err = file.Seek(0, 0)
	if err != nil {
		return err
	}

	return nil
}

// RealPath returns the true, case-preserved path as it exists on disk.
// It walks through each directory level and reads the actual names.
func RealPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	vol := filepath.VolumeName(abs)
	path := strings.TrimPrefix(abs, vol)
	if path == "" {
		return vol, nil
	}

	parts := strings.Split(path, string(filepath.Separator))
	current := vol + string(filepath.Separator)

	for _, part := range parts {
		if part == "" {
			continue
		}
		entries, err := os.ReadDir(current)
		if err != nil {
			return "", fmt.Errorf("reading %q: %w", current, err)
		}

		found := false
		for _, e := range entries {
			if strings.EqualFold(e.Name(), part) {
				current = filepath.Join(current, e.Name())
				found = true
				break
			}
		}

		if !found {
			// Part not found — maybe path doesn’t exist yet
			current = filepath.Join(current, part)
		}
	}
	return current, nil
}
