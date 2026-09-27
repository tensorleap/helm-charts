package local

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

			require.NoError(t, WriteFileAtomic(path, []byte("new"), 0o666))

			got, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, "new", string(got))
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o666), info.Mode().Perm(), "perm is exact, not umask-masked")
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			assert.Len(t, entries, 1, "no temp file left behind")
		})
	}
}

func modePtr(m os.FileMode) *os.FileMode { return &m }
