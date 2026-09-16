package local

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The copy fallback replicates the data dir, containerd snapshots included, so
// it has to reproduce modes exactly rather than let the umask reshape them. The
// modes here (0777, 0666) are ones a default 0022 umask would change, which is
// what `cp -r` used to do (BF-1092).
func TestCopyDirPreservingAttrs(t *testing.T) {
	tests := []struct {
		name string
		path string
		mode os.FileMode
	}{
		{"world-writable dir", "sub", 0o777},
		{"world-writable file", "sub/world.txt", 0o666},
		{"private file", "sub/private.txt", 0o600},
		{"executable file", "exec.sh", 0o755},
	}

	root := t.TempDir()
	src := filepath.Join(root, "src")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o777))
	for _, tt := range tests {
		full := filepath.Join(src, tt.path)
		if filepath.Ext(tt.path) != "" {
			require.NoError(t, os.WriteFile(full, []byte("x"), tt.mode))
		}
		// WriteFile and MkdirAll both apply the umask; force the mode we want.
		require.NoError(t, os.Chmod(full, tt.mode))
	}
	require.NoError(t, os.Symlink("sub/world.txt", filepath.Join(src, "link")))

	dst := filepath.Join(root, "dst")
	require.NoError(t, copyDirPreservingAttrs(src, dst, false))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := os.Lstat(filepath.Join(dst, tt.path))
			require.NoError(t, err)
			assert.Equal(t, tt.mode, info.Mode().Perm(), "mode not preserved for %s", tt.path)
		})
	}

	t.Run("symlink stays a symlink", func(t *testing.T) {
		info, err := os.Lstat(filepath.Join(dst, "link"))
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink)
	})
}

// A second local user has to be able to maintain an install the first one
// created, so every run heals the dirs we manage. The heal used to be gated on
// the data dir root not already being 0777 — which it always is after an
// install — so drift below it was never repaired.
func TestInitStandaloneDirHealsDriftedSubDirs(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "standalone")
	require.NoError(t, os.MkdirAll(dataDir, 0o777))
	require.NoError(t, os.Chmod(dataDir, 0o777))
	t.Setenv(DATA_DIR_ENV_NAME, dataDir)

	drifted := filepath.Join(dataDir, CONTAINERD_DIR_NAME)
	require.NoError(t, os.MkdirAll(drifted, 0o700))
	require.NoError(t, os.Chmod(drifted, 0o700))

	require.NoError(t, InitStandaloneDir())

	t.Run("existing subdir is healed", func(t *testing.T) {
		info, err := os.Stat(drifted)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o777), info.Mode().Perm())
	})

	t.Run("missing subdirs are created world-writable", func(t *testing.T) {
		for _, dir := range []string{STORAGE_DIR_NAME, REGISTRY_DIR_NAME, LOGS_DIR_NAME, MANIFEST_DIR_NAME, ELASTIC_STORAGE_DIR_NAME, KEYCLOAK_DB_STORAGE_DIR_NAME, HELM_CACHE_DIR_NAME} {
			info, err := os.Stat(filepath.Join(dataDir, dir))
			require.NoError(t, err, "subdir %s not created", dir)
			assert.Equal(t, os.FileMode(0o777), info.Mode().Perm(), "subdir %s not world-writable", dir)
		}
	})
}
