//go:build !windows
// +build !windows

package log

import (
	"strings"

	"golang.org/x/sys/unix"
)

func getUnameData() map[string]interface{} {
	var unameInfo unix.Utsname

	err := unix.Uname(&unameInfo)
	if err != nil {
		return map[string]interface{}{}
	}

	sysname := strings.ReplaceAll(string(unameInfo.Sysname[:]), "\u0000", "")
	nodename := strings.ReplaceAll(string(unameInfo.Nodename[:]), "\u0000", "")
	release := strings.ReplaceAll(string(unameInfo.Release[:]), "\u0000", "")
	version := strings.ReplaceAll(string(unameInfo.Version[:]), "\u0000", "")
	machine := strings.ReplaceAll(string(unameInfo.Machine[:]), "\u0000", "")

	return map[string]interface{}{"sysname": sysname, "nodename": nodename, "release": release, "version": version, "machine": machine}
}
