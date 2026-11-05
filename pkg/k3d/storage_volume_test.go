package k3d

import (
	"strings"
	"testing"

	k3d "github.com/k3d-io/k3d/v5/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestClusterHasVolume(t *testing.T) {
	node := func(role k3d.Role, volumes ...string) *k3d.Node {
		return &k3d.Node{Role: role, Volumes: volumes}
	}
	tests := []struct {
		name  string
		nodes []*k3d.Node
		want  bool
	}{
		{"server mounts it", []*k3d.Node{node(k3d.ServerRole, "/data:/data", StorageVolumeSpec())}, true},
		{"server mounts other volumes only", []*k3d.Node{node(k3d.ServerRole, "/data:/data", CONTAINERD_VOLUME_NAME+":/x")}, false},
		{"only an agent mounts it", []*k3d.Node{node(k3d.AgentRole, StorageVolumeSpec())}, false},
		{"name is a prefix of another volume", []*k3d.Node{node(k3d.ServerRole, STORAGE_VOLUME_NAME+"-old:/x")}, false},
		{"no nodes", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClusterHasVolume(&Cluster{Nodes: tt.nodes}, STORAGE_VOLUME_NAME))
		})
	}
}

func TestStorageSetupScript(t *testing.T) {
	t.Run("fresh volume only prepares dirs", func(t *testing.T) {
		script := storageSetupScript(false)
		assert.NotContains(t, script, "cp -a")
		assert.NotContains(t, script, "du -sk")
		assert.Contains(t, script, "mkdir -p /storage/elasticsearch /storage/keycloak")
		assert.Contains(t, script, "chmod a+rwx /storage /storage/elasticsearch /storage/keycloak")
	})
	t.Run("migration copies through a staging dir, never over live data", func(t *testing.T) {
		script := storageSetupScript(true)
		assert.True(t, strings.HasPrefix(script, "if [ ! -e /storage/mongodb ]; then"), script)
		assert.Contains(t, script, "cp -a /src/. /storage/.migrating/")
		assert.Contains(t, script, "mv /storage/.migrating/* /storage/")
		assert.Less(t, strings.Index(script, "mv /storage/.migrating"), strings.Index(script, "mkdir -p"))
		// leftovers of a failed copy are dropped before measuring, and the copy
		// only starts when the data fits
		assert.Less(t, strings.Index(script, "rm -rf /storage/.migrating"), strings.Index(script, "du -sk /src"))
		assert.Less(t, strings.Index(script, "du -sk /src"), strings.Index(script, "cp -a"))
	})
	t.Run("space check alone skips a finished migration", func(t *testing.T) {
		script := storageSpaceCheckScript()
		assert.True(t, strings.HasPrefix(script, "if [ -e /storage/mongodb ]; then exit 0; fi"), script)
		assert.Contains(t, script, "du -sk /src")
		assert.NotContains(t, script, "cp -a")
	})
}
