//go:build !windows
// +build !windows

package local

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

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
