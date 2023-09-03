package local

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractTarGzFile(t *testing.T) {
	file, _ := os.Open("./test_assets/project.tar.zip")
	defer file.Close()
	outputDir := "./exported"
	defer os.RemoveAll(outputDir)
	files, err := ExtractTarGzFile(file, outputDir)
	assert.NoError(t, err)
	assert.FileExists(t, filepath.Join(outputDir, "project.json"))
	assert.Len(t, files, 13)
}

func TestCalculateSHA256(t *testing.T) {
	file, _ := os.Open("./test_assets/project.tar.zip")
	defer file.Close()

	hash1, _ := GetFileChecksum(file)
	_, _ = file.Seek(0, 0)
	hash2, _ := GetFileChecksum(file)

	assert.Equal(t, hash1, hash2)
}
