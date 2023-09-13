package local

import (
	"os"
)

func CleanupTempFile(file *os.File) {
	file.Close()
	os.Remove(file.Name())
}
