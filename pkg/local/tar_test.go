package local

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTarDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "mongodb", "db"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "mongodb", "db", "WiredTiger"), []byte("db"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "obj"), []byte("object"), 0o644))
	if err := os.Symlink("obj", filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	size, err := DirSize(root)
	require.NoError(t, err)
	assert.Equal(t, int64(8), size)

	var buf bytes.Buffer
	var reported int64
	require.NoError(t, TarDir(root, ".migrating", &buf, func(n int64) { reported = n }))
	assert.Equal(t, int64(8), reported)

	type entry struct {
		typeflag byte
		content  string
		link     string
		mode     int64
	}
	got := map[string]entry{}
	tr := tar.NewReader(&buf)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		body, _ := io.ReadAll(tr)
		mode := hdr.Mode & 0o777
		if hdr.Typeflag == tar.TypeSymlink {
			mode = 0 // lstat perms of a symlink differ per OS
		}
		got[hdr.Name] = entry{hdr.Typeflag, string(body), hdr.Linkname, mode}
		assert.Zero(t, hdr.Uid, hdr.Name)
	}
	assert.Equal(t, map[string]entry{
		".migrating/":                      {tar.TypeDir, "", "", 0o777},
		".migrating/mongodb/":              {tar.TypeDir, "", "", 0o755},
		".migrating/mongodb/db/":           {tar.TypeDir, "", "", 0o755},
		".migrating/mongodb/db/WiredTiger": {tar.TypeReg, "db", "", 0o600},
		".migrating/obj":                   {tar.TypeReg, "object", "", 0o644},
		".migrating/link":                  {tar.TypeSymlink, "", "obj", 0},
	}, got)
}
