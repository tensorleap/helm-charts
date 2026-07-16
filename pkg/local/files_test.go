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

func TestEnsureDirExists(t *testing.T) {
	t.Run("Creates missing dir world-writable", func(t *testing.T) {
		dir := t.TempDir() + "/new-dir"
		err := EnsureDirExists(dir)
		assert.Nil(t, err)
		info, err := os.Stat(dir)
		assert.Nil(t, err)
		assert.Equal(t, os.FileMode(0o777), info.Mode().Perm())
	})

	t.Run("Heals existing non-world-writable dir", func(t *testing.T) {
		dir := t.TempDir() + "/owned-dir"
		err := os.Mkdir(dir, 0o755)
		assert.Nil(t, err)
		err = EnsureDirExists(dir)
		assert.Nil(t, err)
		info, err := os.Stat(dir)
		assert.Nil(t, err)
		assert.Equal(t, os.FileMode(0o777), info.Mode().Perm())
	})

	t.Run("Leaves world-writable dir alone", func(t *testing.T) {
		dir := t.TempDir() + "/shared-dir"
		err := os.Mkdir(dir, 0o755)
		assert.Nil(t, err)
		err = os.Chmod(dir, 0o777)
		assert.Nil(t, err)
		err = EnsureDirExists(dir)
		assert.Nil(t, err)
		info, err := os.Stat(dir)
		assert.Nil(t, err)
		assert.Equal(t, os.FileMode(0o777), info.Mode().Perm())
	})

	t.Run("Does not chmod an existing regular file", func(t *testing.T) {
		file := t.TempDir() + "/afile"
		assert.Nil(t, os.WriteFile(file, []byte("x"), 0o644))
		err := EnsureDirExists(file)
		assert.Nil(t, err)
		info, err := os.Stat(file)
		assert.Nil(t, err)
		assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	})

	t.Run("Does not follow a symlink to chmod its target", func(t *testing.T) {
		base := t.TempDir()
		target := base + "/target"
		assert.Nil(t, os.Mkdir(target, 0o750))
		link := base + "/link"
		assert.Nil(t, os.Symlink(target, link))
		err := EnsureDirExists(link)
		assert.Nil(t, err)
		info, err := os.Stat(target)
		assert.Nil(t, err)
		assert.Equal(t, os.FileMode(0o750), info.Mode().Perm())
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
	t.Run("Does not chmod existing dst parent", func(t *testing.T) {
		srcDir := t.TempDir() + "/src"
		assert.Nil(t, os.Mkdir(srcDir, 0o755))
		parent := t.TempDir()
		assert.Nil(t, os.Chmod(parent, 0o755))

		srcStatus, err := CheckDirectoryStatus(srcDir)
		assert.Nil(t, err)
		dstStatus, err := CheckDirectoryStatus(parent + "/dst")
		assert.Nil(t, err)
		assert.Nil(t, MoveOrCopyDirectory(srcStatus, dstStatus))

		info, err := os.Stat(parent)
		assert.Nil(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	})

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
