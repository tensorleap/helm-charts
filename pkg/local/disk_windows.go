//go:build windows
// +build windows

package local

import "errors"

func DiskTotalBytes(path string) (int64, error) {
	return 0, errors.New("disk size detection is not supported on windows")
}
