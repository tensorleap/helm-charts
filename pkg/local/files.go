package local

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
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

// The data dir holds three kinds of directories, each with its own policy:
//
//   - shared (EnsureSharedDir): the ones humans write to through the CLI — the
//     data dir root, manifests/, logs/, helm-cache/. Owned by the tensorleap
//     group, mode 2775: every member creates and replaces files, new entries
//     inherit the group (setgid), everyone else only reads.
//   - storage (EnsureDirExists): hostPath volumes the pods write to with their
//     own uids (elasticsearch as 1000, ...). kubelet applies no fsGroup to
//     hostPath and pods are not members of host groups, so these stay
//     world-writable.
//   - cache (EnsurePrivateDir): containerd/, written only by root inside the
//     k3s node. Plain 0755 with no setgid, so nothing created inside inherits
//     the shared group; never copied on a data-dir transfer.
//
// All three heal an existing directory only when it is a real directory we
// own; otherwise they escalate with sudo only for paths that resolve to inside
// the data dir. Arbitrary paths reach these functions (user-supplied dataset
// volumes), and a group-writable tree lets another local user plant a symlink,
// so an unconditional sudo chmod would let either trick a sudo-capable operator
// into changing any directory on the host. When an existing dir can't be
// healed they warn and continue: it may still be usable as-is, and failing
// would break installs that work today.

// EnsureDirExists makes sure path exists as a world-writable directory. Use it
// for directories the pods write to (see the policy note above); directories
// humans write to belong to EnsureSharedDir.
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
		// Five octal digits: GNU chmod keeps a directory's setgid bit for
		// shorter numeric modes, and the parent may be a setgid shared dir.
		if err := runMaybeSudo(createdWithSudo, "chmod", "00777", path); err != nil {
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
		if err := runMaybeSudo(true, "chmod", "777", resolved); err == nil {
			return nil
		}
	}
	log.Warnf("Could not make %s world-writable. Other local users may be unable to use it; have its owner or an admin run: sudo chmod 777 %s", path, path)
	return nil
}

// sharedDirPerm is the permission part of a shared directory's mode; the full
// mode is 2775 (setgid on top).
const sharedDirPerm = os.FileMode(0o775)

// EnsureSharedDir makes sure path exists as a directory owned by the shared
// group with mode 2775 (see the policy note above). Without a shared group —
// non-Linux hosts, or the group could not be created — it falls back to the
// world-writable directory EnsureDirExists provides.
func EnsureSharedDir(path string) error {
	gid, ok := SharedGroupGID()
	if !ok {
		return EnsureDirExists(path)
	}
	return ensureSharedDir(path, gid)
}

func ensureSharedDir(path string, gid int) error {
	status, err := CheckDirectoryStatus(path)
	if err != nil {
		return err
	}

	if !status.Exists {
		createdWithSudo := !status.CanCreateOnParentDirectory
		if err := runMaybeSudo(createdWithSudo, "mkdir", "-p", path); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", path, err)
		}
		if err := setSharedDirPerms(path, gid, createdWithSudo); err != nil {
			return fmt.Errorf("failed to set shared permissions on %s: %w", path, err)
		}
		return nil
	}

	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return nil
	}
	if hasSharedDirPerms(info, gid) {
		return nil
	}
	if err := setSharedDirPerms(path, gid, false); err == nil {
		return nil
	}
	if resolved, ok := resolveInsideDataDir(path); ok {
		if err := setSharedDirPerms(resolved, gid, true); err == nil {
			return nil
		}
	}
	log.Warnf("Could not hand %s to group %s. Other users may be unable to use it; have its owner or an admin run: sudo chgrp %s %s && sudo chmod 2775 %s",
		path, SHARED_GROUP_NAME, SHARED_GROUP_NAME, path, path)
	return nil
}

func hasSharedDirPerms(info os.FileInfo, gid int) bool {
	_, fileGID, ok := ownerOf(info)
	if !ok || fileGID != gid {
		return false
	}
	return info.Mode().Perm() == sharedDirPerm && info.Mode()&os.ModeSetgid != 0
}

func setSharedDirPerms(path string, gid int, useSudo bool) error {
	if useSudo {
		if err := runMaybeSudo(true, "chgrp", strconv.Itoa(gid), path); err != nil {
			return err
		}
		return runMaybeSudo(true, "chmod", "2775", path)
	}
	if err := os.Chown(path, -1, gid); err != nil {
		return err
	}
	return os.Chmod(path, sharedDirPerm|os.ModeSetgid)
}

// EnsurePrivateDir makes sure path exists as a plain 0755 directory with no
// setgid bit (see the policy note above). An existing directory keeps its
// permission bits; only an inherited setgid bit is cleared.
func EnsurePrivateDir(path string) error {
	status, err := CheckDirectoryStatus(path)
	if err != nil {
		return err
	}

	if !status.Exists {
		createdWithSudo := !status.CanCreateOnParentDirectory
		if err := runMaybeSudo(createdWithSudo, "mkdir", "-p", path); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", path, err)
		}
		if err := runMaybeSudo(createdWithSudo, "chmod", "00755", path); err != nil {
			return fmt.Errorf("failed to set directory permissions on %s: %w", path, err)
		}
		return nil
	}

	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSetgid == 0 {
		return nil
	}
	if err := os.Chmod(path, info.Mode().Perm()); err == nil {
		return nil
	}
	if resolved, ok := resolveInsideDataDir(path); ok {
		if err := runMaybeSudo(true, "chmod", "g-s", resolved); err == nil {
			return nil
		}
	}
	log.Warnf("Could not clear the setgid bit on %s; files created inside will inherit group %s", path, SHARED_GROUP_NAME)
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

// runMaybeSudo runs a command, through sudo when asked to and we are not root
// already.
func runMaybeSudo(useSudo bool, args ...string) error {
	if useSudo && os.Geteuid() != 0 {
		args = append([]string{"sudo"}, args...)
	}
	return RunCommand(args...)
}

// runAsRoot runs a command that needs root: directly when we already are root,
// through sudo otherwise.
func runAsRoot(args ...string) error {
	return runMaybeSudo(true, args...)
}

// MoveOrCopyDirectory tries to rename the directory, on failure tries to copy
func MoveOrCopyDirectory(srcStatus, dstStatus FileSystemStatus) error {
	// Ensure the parent directory of the destination exists. Only create it
	// when missing — an existing parent may be a system dir (/opt, /var/lib,
	// a home dir) whose permissions are not ours to change.
	dstParent := filepath.Dir(dstStatus.Path)
	parentStatus, err := CheckDirectoryStatus(dstParent)
	if err != nil {
		return err
	}
	if !parentStatus.Exists {
		if err := EnsurePrivateDir(dstParent); err != nil {
			return err
		}
	}

	if !srcStatus.Exists {
		return fmt.Errorf("source directory does not exist: %s", srcStatus.Path)
	}

	isStorageMovePermissionNeeded := !srcStatus.CanWrite || !srcStatus.CanCreateOnParentDirectory || !dstStatus.CanCreateOnParentDirectory

	if isStorageMovePermissionNeeded {
		log.Warn("Move operation requires sudo permissions")
	}
	if err := runMaybeSudo(isStorageMovePermissionNeeded, "mv", srcStatus.Path, dstStatus.Path); err != nil {
		log.Warnf("Failed to move directory, attempting to copy: %v", err)
		if err := copyDirPreservingAttrs(srcStatus.Path, dstStatus.Path, isStorageMovePermissionNeeded); err != nil {
			return fmt.Errorf("copy operation failed: %w", err)
		}

		if err := RemoveDirectory(srcStatus); err != nil {
			log.Warnf("failed to remove src directory: %v", err)
		}
	}

	return nil
}

// copyDirPreservingAttrs copies the contents of src into dst as an exact
// replica: ownership, modes, timestamps, symlinks, hard links and xattrs. All
// of them matter for the data dir — app storage keeps the uids the pods run
// as, and a plain `cp -r` would reown everything to the copying user with
// umask-masked modes. Copying src/. (the contents) rather than src means a
// destination that a failed mv left half-populated is merged into, not nested
// under.
func copyDirPreservingAttrs(src, dst string, useSudo bool) error {
	if useSudo {
		log.Warn("Copy operation requires sudo permissions")
	}
	contents := filepath.Clean(src) + string(filepath.Separator) + "."
	return runMaybeSudo(useSudo, "cp", "-a", contents, dst)
}

// RemovePath removes path and everything under it, retrying with sudo when a
// permission error blocks direct removal (root-owned content in a directory
// we can write to, or a directory we cannot).
func RemovePath(path string) error {
	err := os.RemoveAll(path)
	if err == nil {
		return nil
	}
	if sudoErr := runAsRoot("rm", "-rf", path); sudoErr != nil {
		return fmt.Errorf("failed to remove %s: %v (and with sudo: %w)", path, err, sudoErr)
	}
	return nil
}

// SetSharedFilePerms sets perm on a file the CLI wrote and hands it to the
// shared group, so other members can read it (and, with a group-writable perm,
// replace it). A file owned by another member refuses the chmod, which is fine
// when its mode is already right.
func SetSharedFilePerms(path string, perm os.FileMode) error {
	if err := os.Chmod(path, perm); err != nil {
		info, statErr := os.Stat(path)
		if statErr != nil || info.Mode().Perm() != perm {
			return err
		}
	}
	if gid, ok := SharedGroupGID(); ok {
		_ = os.Chown(path, -1, gid) // best effort: refused when not the owner
	}
	return nil
}

// WriteFileAtomic writes data to path via a temp file in the same directory
// that is renamed over path. Rename only needs write access to the directory
// (the directories humans write to are group-writable) and replaces the target
// regardless of who owns it, so a second local user can update files created
// by the first — plain os.WriteFile fails there with EACCES (BF-1092).
//
// perm is applied with Chmod so it is exact, not umask-masked, and the file is
// handed to the shared group.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	// Remove the temp file on any failure below; after a successful rename it
	// no longer exists and Remove is a harmless no-op.
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write %s: %w", tmpPath, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to chmod %s: %w", tmpPath, err)
	}
	if gid, ok := SharedGroupGID(); ok {
		_ = tmp.Chown(-1, gid) // best effort; a setgid parent already gives the group
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to replace %s: %w", path, err)
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
