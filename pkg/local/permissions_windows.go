//go:build windows
// +build windows

package local

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// not tested - not supposed to be used just for completion
func SetPermissionFromFileInfo(perms *FileSystemStatus, info fs.FileInfo) error {
	// Get the file path from the FileInfo
	filename := info.Name()

	// Check read permission by attempting to open the file for reading
	file, err := os.OpenFile(filename, os.O_RDONLY, 0)
	if err == nil {
		perms.CanRead = true
		file.Close()
	}

	// Check write permission by attempting to open the file for writing
	file, err = os.OpenFile(filename, os.O_WRONLY, 0)
	if err == nil {
		perms.CanWrite = true
		file.Close()
	}

	// Check execute permission based on file extension
	ext := strings.ToLower(filepath.Ext(filename))
	executableExtensions := map[string]bool{
		".exe": true,
		".bat": true,
		".com": true,
	}

	if executableExtensions[ext] {
		perms.CanExecute = true
	}

	return nil
}
