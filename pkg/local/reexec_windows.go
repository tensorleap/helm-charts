//go:build windows
// +build windows

package local

import "errors"

func reexecWithSharedGroup() error {
	return errors.New("shared group activation is not supported on windows")
}
