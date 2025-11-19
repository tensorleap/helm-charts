//go:build windows
// +build windows

package log

import (
	"runtime"
)

func getUnameData() map[string]interface{} {
	return map[string]interface{}{
		"sysname":  "Windows",
		"nodename": "",
		"release":  runtime.GOOS,
		"version":  runtime.Version(),
		"machine":  runtime.GOARCH,
	}
}
