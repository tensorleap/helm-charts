package k3d

import (
	"testing"

	k3d "github.com/k3d-io/k3d/v5/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestIsGpuCluster(t *testing.T) {
	testCases := []struct {
		name     string
		cluster  *Cluster
		expected bool
	}{
		{
			name: "Non-GPU cluster",
			cluster: &Cluster{
				Nodes: []*k3d.Node{
					{Image: "docker.io/rancher/k3s:v1.26.4-k3s1"},
				},
			},
			expected: false,
		},
		{
			name: "GPU cluster",
			cluster: &Cluster{
				Nodes: []*k3d.Node{
					{Image: "docker.io/rancher/k3s:v1.26.4-k3s1-cuda-11.8.0-ubuntu-22.04"},
				},
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := IsGpuCluster(tc.cluster)
			assert.Equal(t, tc.expected, result)
		})
	}
}
