package local

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckDirectoryStatus(t *testing.T) {
	t.Run("Sudo dir", func(t *testing.T) {
		t.Skip("Skip this test as it requires sudo permissions")
		status, err := CheckDirectoryStatus("../../tempSudoDir") // create tempSudoDir directory with sudo on the repo root
		assert.Nil(t, err)
		assert.True(t, status.Exists)
		assert.False(t, status.CanWrite)
		assert.True(t, status.CanCreateOnParentDirectory)
	})

	t.Run("Sub Sudo dir", func(t *testing.T) {
		t.Skip("Skip this test as it requires sudo permissions")
		status, err := CheckDirectoryStatus("../../tempSudoDir/dir") // create tempSudoDir directory with sudo on the repo root
		assert.Nil(t, err)
		assert.False(t, status.Exists)
		assert.False(t, status.CanWrite)
		assert.True(t, status.CanRead)
		assert.False(t, status.CanCreateOnParentDirectory)
	})

	t.Run("Normal dir", func(t *testing.T) {
		status, err := CheckDirectoryStatus("../../pkg")
		assert.Nil(t, err)
		assert.True(t, status.Exists)
		assert.True(t, status.CanWrite)
		assert.True(t, status.CanCreateOnParentDirectory)
	})

	t.Run("Non-existent dir", func(t *testing.T) {
		status, err := CheckDirectoryStatus("./non-existent-dir")
		assert.Nil(t, err)
		assert.False(t, status.Exists)
		assert.True(t, status.CanWrite)
		assert.True(t, status.CanCreateOnParentDirectory)
	})

	t.Run("Non-existent sub dir", func(t *testing.T) {
		status, err := CheckDirectoryStatus("./non-existent-dir/non-existent-dir/non-existent-dir")
		assert.Nil(t, err)
		assert.False(t, status.Exists)
		assert.True(t, status.CanWrite)
		assert.True(t, status.CanCreateOnParentDirectory)
	})
}

func TestRemoveDirectory(t *testing.T) {
	t.Run("Non-existent dir", func(t *testing.T) {
		status := FileSystemStatus{
			Path: "./non-existent-dir",
		}
		err := RemoveDirectory(status)
		assert.Nil(t, err)
	})

	t.Run("Normal dir", func(t *testing.T) {
		err := os.Mkdir("test-dir", 0777)
		assert.Nil(t, err)
		status, err := CheckDirectoryStatus("test-dir")
		assert.Nil(t, err)
		err = RemoveDirectory(status)
		assert.Nil(t, err)
	})
}

func TestMoveOrCopyDirectory(t *testing.T) {
	t.Run("Non-existent src dir", func(t *testing.T) {
		tmpSrcFile := "tmp-src-file"
		tmpDstFile := "tmp-dst-file"
		file, err := os.Create(tmpSrcFile)
		assert.Nil(t, err)
		file.Close()
		srcStatus, err := CheckDirectoryStatus(file.Name())
		assert.Nil(t, err)
		dstStatus, err := CheckDirectoryStatus(tmpDstFile)
		assert.Nil(t, err)
		assert.Nil(t, err)
		err = MoveOrCopyDirectory(srcStatus, dstStatus)
		assert.Nil(t, err)
		dstStatus, err = CheckDirectoryStatus(tmpDstFile)
		assert.Nil(t, err)
		assert.True(t, dstStatus.Exists)
		srcStatus, err = CheckDirectoryStatus(file.Name())
		assert.Nil(t, err)
		assert.False(t, srcStatus.Exists)
		os.Remove(tmpDstFile)
	})

}
