//go:build !windows
// +build !windows

package local

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// SetPermissionFromFileInfo records whether the current process may read,
// write and execute the file described by info, following the kernel's rule:
// the owner bits apply to the owner, otherwise the group bits to a member of
// the file's group (primary or supplementary), otherwise the other bits. Root
// bypasses the check entirely.
func SetPermissionFromFileInfo(perms *FileSystemStatus, info fs.FileInfo) error {
	uid, gid, ok := fileOwner(info)
	if !ok {
		return fmt.Errorf("failed to get stat info")
	}
	if os.Geteuid() == 0 {
		perms.CanRead, perms.CanWrite, perms.CanExecute = true, true, true
		return nil
	}

	mode := info.Mode()
	isOwner := uid == os.Geteuid()
	inGroup := processHasGroup(gid)
	has := func(owner, group, other os.FileMode) bool {
		switch {
		case isOwner:
			return mode&owner != 0
		case inGroup:
			return mode&group != 0
		default:
			return mode&other != 0
		}
	}
	perms.CanRead = has(0400, 0040, 0004)
	perms.CanWrite = has(0200, 0020, 0002)
	perms.CanExecute = has(0100, 0010, 0001)
	return nil
}

// fileOwner returns the uid and gid owning the file described by info.
func fileOwner(info fs.FileInfo) (uid, gid int, ok bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return int(stat.Uid), int(stat.Gid), true
}
