package local

import (
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageHasData(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, dataDir string)
		want  bool
	}{
		{"no storage dir", func(*testing.T, string) {}, false},
		{"pre-created empty store dirs", func(t *testing.T, d string) {
			require.NoError(t, os.MkdirAll(path.Join(d, ELASTIC_STORAGE_DIR_NAME), 0o777))
			require.NoError(t, os.MkdirAll(path.Join(d, KEYCLOAK_DB_STORAGE_DIR_NAME), 0o777))
		}, false},
		{"a file inside a store", func(t *testing.T, d string) {
			require.NoError(t, os.MkdirAll(path.Join(d, MONGODB_STORAGE_DIR_NAME, "data"), 0o777))
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataDir := t.TempDir()
			t.Setenv(DATA_DIR_ENV_NAME, dataDir)
			tt.setup(t, dataDir)
			got, err := StorageHasData()
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
