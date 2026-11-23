//go:build !windows

package local

import "golang.org/x/sys/unix"

// FreeDiskKB returns the free space, in 1K blocks, of the filesystem holding
// path, as seen by an unprivileged user.
func FreeDiskKB(path string) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize) / 1024, nil
}
