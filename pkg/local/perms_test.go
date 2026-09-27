package local

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useCurrentGIDAsSharedGroup makes the shared-group policy testable on any
// host: the process's effective gid stands in for the tensorleap group, which
// only exists on installed Linux hosts.
func useCurrentGIDAsSharedGroup(t *testing.T) int {
	t.Helper()
	gid := os.Getegid()
	prev := lookupSharedGroupGID
	lookupSharedGroupGID = func() (int, bool) { return gid, true }
	t.Cleanup(func() { lookupSharedGroupGID = prev })
	return gid
}

// withoutSharedGroupSetup keeps InitStandaloneDir from touching the host's groups.
func withoutSharedGroupSetup(t *testing.T) {
	t.Helper()
	prev := sharedGroupSetup
	sharedGroupSetup = func() error { return nil }
	t.Cleanup(func() { sharedGroupSetup = prev })
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
}

// mkdirExact creates path with exactly mode; MkdirAll alone applies the umask.
func mkdirExact(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(path, mode))
	require.NoError(t, os.Chmod(path, mode))
}

func statGID(t *testing.T, path string) int {
	t.Helper()
	info, err := os.Lstat(path)
	require.NoError(t, err)
	_, gid, ok := fileOwner(info)
	require.True(t, ok)
	return gid
}

func assertSharedDir(t *testing.T, dir string, gid int) {
	t.Helper()
	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o775), info.Mode().Perm(), "mode of %s", dir)
	assert.NotZero(t, info.Mode()&os.ModeSetgid, "%s needs setgid so new entries inherit the group", dir)
	assert.Equal(t, gid, statGID(t, dir), "group of %s", dir)
}

func TestEnsureSharedDir(t *testing.T) {
	skipOnWindows(t)
	gid := useCurrentGIDAsSharedGroup(t)

	t.Run("creates a missing dir", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "manifests")
		require.NoError(t, EnsureSharedDir(dir))
		assertSharedDir(t, dir, gid)
	})

	t.Run("creates missing parents", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "a", "b", "helm-cache")
		require.NoError(t, EnsureSharedDir(dir))
		assertSharedDir(t, dir, gid)
	})

	t.Run("heals a drifted dir we own", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "logs")
		mkdirExact(t, dir, 0o700)
		require.NoError(t, EnsureSharedDir(dir))
		assertSharedDir(t, dir, gid)
	})

	t.Run("tightens a legacy world-writable dir", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "standalone")
		mkdirExact(t, dir, 0o777)
		require.NoError(t, EnsureSharedDir(dir))
		assertSharedDir(t, dir, gid)
	})

	t.Run("leaves a regular file alone", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
		require.NoError(t, os.Chmod(file, 0o600))
		require.NoError(t, EnsureSharedDir(file))
		info, err := os.Stat(file)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("does not follow a symlink", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "target")
		mkdirExact(t, target, 0o700)
		link := filepath.Join(root, "link")
		require.NoError(t, os.Symlink(target, link))
		require.NoError(t, EnsureSharedDir(link))
		info, err := os.Stat(target)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "the link target must be untouched")
	})
}

func TestEnsureDirExists(t *testing.T) {
	skipOnWindows(t)

	t.Run("creates a missing dir world-writable", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "storage")
		require.NoError(t, EnsureDirExists(dir))
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o777), info.Mode().Perm())
	})

	t.Run("does not inherit setgid from a shared parent", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "standalone")
		mkdirExact(t, parent, 0o775|os.ModeSetgid)
		dir := filepath.Join(parent, "storage")
		require.NoError(t, EnsureDirExists(dir))
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o777), info.Mode().Perm())
		assert.Zero(t, info.Mode()&os.ModeSetgid, "pod storage must not propagate the shared group")
	})

	t.Run("heals a dir we own", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "registry")
		mkdirExact(t, dir, 0o755)
		require.NoError(t, EnsureDirExists(dir))
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o777), info.Mode().Perm())
	})

	t.Run("leaves a regular file alone", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "not-a-dir")
		require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
		require.NoError(t, os.Chmod(file, 0o600))
		require.NoError(t, EnsureDirExists(file))
		info, err := os.Stat(file)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("does not follow a symlink", func(t *testing.T) {
		root := t.TempDir()
		target := filepath.Join(root, "target")
		mkdirExact(t, target, 0o700)
		link := filepath.Join(root, "link")
		require.NoError(t, os.Symlink(target, link))
		require.NoError(t, EnsureDirExists(link))
		info, err := os.Stat(target)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	})
}

func TestEnsurePrivateDir(t *testing.T) {
	skipOnWindows(t)

	t.Run("creates a plain 0755 dir under a shared parent", func(t *testing.T) {
		parent := filepath.Join(t.TempDir(), "standalone")
		mkdirExact(t, parent, 0o775|os.ModeSetgid)
		dir := filepath.Join(parent, "containerd")
		require.NoError(t, EnsurePrivateDir(dir))
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
		assert.Zero(t, info.Mode()&os.ModeSetgid)
	})

	t.Run("clears an inherited setgid bit and keeps the other bits", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "containerd")
		mkdirExact(t, dir, 0o777|os.ModeSetgid)
		require.NoError(t, EnsurePrivateDir(dir))
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o777), info.Mode().Perm(), "existing bits are not tightened")
		assert.Zero(t, info.Mode()&os.ModeSetgid)
	})

	t.Run("leaves a plain dir alone", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "containerd")
		mkdirExact(t, dir, 0o755)
		require.NoError(t, EnsurePrivateDir(dir))
		info, err := os.Stat(dir)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	})
}

// Every run heals the data dir tree to its policy, so a second local user can
// maintain an install another user created and drift is repaired each time.
func TestInitStandaloneDirAppliesPolicy(t *testing.T) {
	skipOnWindows(t)
	gid := useCurrentGIDAsSharedGroup(t)
	withoutSharedGroupSetup(t)

	dataDir := filepath.Join(t.TempDir(), "standalone")
	t.Setenv(DATA_DIR_ENV_NAME, dataDir)
	drifted := filepath.Join(dataDir, MANIFEST_DIR_NAME)
	mkdirExact(t, drifted, 0o700)
	legacy := filepath.Join(dataDir, LOGS_DIR_NAME)
	mkdirExact(t, legacy, 0o777)

	require.NoError(t, InitStandaloneDir())

	t.Run("root and human-facing dirs are shared", func(t *testing.T) {
		assertSharedDir(t, dataDir, gid)
		for _, dir := range sharedDataSubDirs {
			assertSharedDir(t, filepath.Join(dataDir, dir), gid)
		}
	})

	t.Run("pod storage dirs are world-writable without setgid", func(t *testing.T) {
		for _, dir := range storageDataSubDirs {
			info, err := os.Stat(filepath.Join(dataDir, dir))
			require.NoError(t, err, "storage dir %s not created", dir)
			assert.Equal(t, os.FileMode(0o777), info.Mode().Perm(), "mode of %s", dir)
			assert.Zero(t, info.Mode()&os.ModeSetgid, "%s must not propagate the shared group to pod files", dir)
		}
	})

	t.Run("image cache is a plain dir", func(t *testing.T) {
		info, err := os.Stat(filepath.Join(dataDir, CONTAINERD_DIR_NAME))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
		assert.Zero(t, info.Mode()&os.ModeSetgid)
	})
}

// The copy fallback replicates the data dir, so it has to reproduce modes
// exactly rather than let the umask reshape them. The modes here (0777, 0666)
// are ones a default 0022 umask would change, which is what `cp -r` did.
func TestCopyDirPreservingAttrs(t *testing.T) {
	skipOnWindows(t)
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

	newSrc := func(t *testing.T, root string) string {
		src := filepath.Join(root, "src")
		require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0o777))
		for _, tt := range tests {
			full := filepath.Join(src, tt.path)
			if filepath.Ext(tt.path) != "" {
				require.NoError(t, os.WriteFile(full, []byte("x"), tt.mode))
			}
			require.NoError(t, os.Chmod(full, tt.mode))
		}
		require.NoError(t, os.Symlink("sub/world.txt", filepath.Join(src, "link")))
		return src
	}

	t.Run("into a new destination", func(t *testing.T) {
		root := t.TempDir()
		src := newSrc(t, root)
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
	})

	t.Run("merges into an existing destination instead of nesting", func(t *testing.T) {
		root := t.TempDir()
		src := newSrc(t, root)
		dst := filepath.Join(root, "dst")
		require.NoError(t, os.MkdirAll(dst, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dst, "leftover"), []byte("from a failed mv"), 0o644))

		require.NoError(t, copyDirPreservingAttrs(src, dst, false))

		assert.FileExists(t, filepath.Join(dst, "sub", "world.txt"))
		assert.NoDirExists(t, filepath.Join(dst, "src"), "contents are copied, not the directory itself")
		assert.FileExists(t, filepath.Join(dst, "leftover"))
	})
}

func TestRemovePath(t *testing.T) {
	t.Run("removes a tree", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "containerd")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "a", "b"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a", "b", "f"), []byte("x"), 0o644))
		require.NoError(t, RemovePath(dir))
		assert.NoDirExists(t, dir)
	})

	t.Run("missing path is fine", func(t *testing.T) {
		require.NoError(t, RemovePath(filepath.Join(t.TempDir(), "missing")))
	})
}

func TestWriteFileAtomic(t *testing.T) {
	tests := []struct {
		name     string
		existing *os.FileMode // nil = file does not exist yet
	}{
		{"creates a new file", nil},
		{"replaces a writable file", modePtr(0o644)},
		{"replaces a read-only file plain WriteFile cannot open", modePtr(0o444)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "manifest.yaml")
			if tt.existing != nil {
				require.NoError(t, os.WriteFile(path, []byte("old"), 0o600))
				require.NoError(t, os.Chmod(path, *tt.existing))
				if *tt.existing == 0o444 && os.Geteuid() != 0 {
					require.Error(t, os.WriteFile(path, []byte("new"), 0o666), "precondition: the old code path must fail here")
				}
			}

			require.NoError(t, WriteFileAtomic(path, []byte("new"), 0o664))

			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, "new", string(got))
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o664), info.Mode().Perm(), "perm is exact, not umask-masked")
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Len(t, entries, 1, "no temp file left behind")
		})
	}

	t.Run("hands the file to the shared group", func(t *testing.T) {
		skipOnWindows(t)
		gid := useCurrentGIDAsSharedGroup(t)
		path := filepath.Join(t.TempDir(), "params.yaml")
		require.NoError(t, WriteFileAtomic(path, []byte("x"), 0o664))
		assert.Equal(t, gid, statGID(t, path))
	})
}

func modePtr(m os.FileMode) *os.FileMode { return &m }
