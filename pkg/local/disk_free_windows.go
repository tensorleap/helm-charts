//go:build windows

package local

import "golang.org/x/sys/windows"

// FreeDiskKB returns the free space, in 1K blocks, of the volume holding
// path, as seen by the current user.
func FreeDiskKB(path string) (int64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return int64(free / 1024), nil
}
