package local

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSnapshotTree lays out a minimal containerd overlay snapshot tree with two
// files, as an unpacked image layer would have.
func newSnapshotTree(t *testing.T, containerdDir string) {
	t.Helper()
	etc := filepath.Join(containerdDir, overlaySnapshotsSubdir, "1", "fs", "etc")
	require.NoError(t, os.MkdirAll(etc, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(etc, "passwd"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etc, "shadow"), []byte("x"), 0o640))
}

func fakeOwners(t *testing.T, owner func(fs.FileInfo) (int, int, bool)) {
	t.Helper()
	prev := ownerOf
	ownerOf = owner
	t.Cleanup(func() { ownerOf = prev })
}

func allRoot(fs.FileInfo) (int, int, bool) { return 0, 0, true }

func TestImageCacheReowned(t *testing.T) {
	t.Run("no snapshot tree is not inspected", func(t *testing.T) {
		reowned, inspected := ImageCacheReowned(t.TempDir())
		assert.False(t, inspected)
		assert.False(t, reowned)
	})

	t.Run("empty snapshot tree is not inspected", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, overlaySnapshotsSubdir), 0o755))
		_, inspected := ImageCacheReowned(dir)
		assert.False(t, inspected)
	})

	t.Run("one non-root entry anywhere means healthy", func(t *testing.T) {
		dir := t.TempDir()
		newSnapshotTree(t, dir)
		fakeOwners(t, func(info fs.FileInfo) (int, int, bool) {
			if info.Name() == "shadow" {
				return 0, 42, true // root:shadow, as every image ships it
			}
			return 0, 0, true
		})
		reowned, inspected := ImageCacheReowned(dir)
		assert.True(t, inspected)
		assert.False(t, reowned)
	})

	t.Run("everything root-owned means reowned", func(t *testing.T) {
		dir := t.TempDir()
		newSnapshotTree(t, dir)
		fakeOwners(t, allRoot)
		reowned, inspected := ImageCacheReowned(dir)
		assert.True(t, inspected)
		assert.True(t, reowned)
	})

	t.Run("unknown ownership is not inspected", func(t *testing.T) {
		dir := t.TempDir()
		newSnapshotTree(t, dir)
		fakeOwners(t, func(fs.FileInfo) (int, int, bool) { return 0, 0, false })
		_, inspected := ImageCacheReowned(dir)
		assert.False(t, inspected)
	})
}

func TestRepairReownedImageCache(t *testing.T) {
	t.Run("rebuilds a reowned cache", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv(DATA_DIR_ENV_NAME, dataDir)
		containerdDir := GetContainerdDataDir()
		newSnapshotTree(t, containerdDir)
		fakeOwners(t, allRoot)

		require.NoError(t, RepairReownedImageCache())

		assert.NoDirExists(t, filepath.Join(containerdDir, overlaySnapshotsSubdir))
		info, err := os.Stat(containerdDir)
		require.NoError(t, err, "an empty cache dir is left for the cluster to fill")
		assert.True(t, info.IsDir())
	})

	t.Run("leaves a healthy cache alone", func(t *testing.T) {
		dataDir := t.TempDir()
		t.Setenv(DATA_DIR_ENV_NAME, dataDir)
		containerdDir := GetContainerdDataDir()
		newSnapshotTree(t, containerdDir)
		fakeOwners(t, func(fs.FileInfo) (int, int, bool) { return 101, 82, true })

		require.NoError(t, RepairReownedImageCache())

		assert.FileExists(t, filepath.Join(containerdDir, overlaySnapshotsSubdir, "1", "fs", "etc", "passwd"))
	})
}
