package local

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"syscall"

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

// EnsureDirExists checks if a directory exists, and if not, creates it with sudo if needed.
func EnsureDirExists(path string) error {
	status, err := CheckDirectoryStatus(path)
	if err != nil {
		return err
	}
	if status.Exists {
		return nil
	}

	mkDirArgs := []string{"mkdir", "-p", path}
	if !status.CanCreateOnParentDirectory {
		mkDirArgs = append([]string{"sudo"}, mkDirArgs...)
	}
	if err := RunCommand(mkDirArgs...); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	return nil
}

// MoveOrCopyDirectory tries to rename the directory, on failure tries to copy
func MoveOrCopyDirectory(srcStatus, dstStatus FileSystemStatus) error {
	// Ensure the parent directory of the destination exists
	dstParent := filepath.Dir(dstStatus.Path)
	if err := EnsureDirExists(dstParent); err != nil {
		return err
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

// SetPermissionFromFileInfo checks if the current process has read, write, and execute permissions on the given path.
func SetPermissionFromFileInfo(perms *FileSystemStatus, info fs.FileInfo) error {

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("failed to get stat info")
	}

	uid := os.Getuid()
	gid := os.Getgid()
	mode := info.Mode()

	// Check read permission
	if stat.Uid == uint32(uid) && mode&0400 != 0 {
		perms.CanRead = true
	} else if stat.Gid == uint32(gid) && mode&0040 != 0 {
		perms.CanRead = true
	} else if mode&0004 != 0 {
		perms.CanRead = true
	}

	// Check write permission
	if stat.Uid == uint32(uid) && mode&0200 != 0 {
		perms.CanWrite = true
	} else if stat.Gid == uint32(gid) && mode&0020 != 0 {
		perms.CanWrite = true
	} else if mode&0002 != 0 {
		perms.CanWrite = true
	}

	// Check execute permission
	if stat.Uid == uint32(uid) && mode&0100 != 0 {
		perms.CanExecute = true
	} else if stat.Gid == uint32(gid) && mode&0010 != 0 {
		perms.CanExecute = true
	} else if mode&0001 != 0 {
		perms.CanExecute = true
	}

	return nil
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
	if !status.CanWrite {
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
