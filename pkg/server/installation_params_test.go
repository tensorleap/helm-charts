package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetInfraHelmValuesParams(t *testing.T) {
	t.Run("GPU Operator enabled with all GPUs", func(t *testing.T) {
		params := InstallationParams{
			GpuDevices: "all",
		}
		valueParams := params.GetInfraHelmValuesParams()
		assert.True(t, valueParams.GpuOperatorEnabled)
		assert.Equal(t, "all", valueParams.NvidiaVisibleDevices)
	})

	t.Run("GPU Operator enabled with GPU count uses all", func(t *testing.T) {
		params := InstallationParams{
			Gpus: 2,
		}
		valueParams := params.GetInfraHelmValuesParams()
		assert.True(t, valueParams.GpuOperatorEnabled)
		assert.Equal(t, "all", valueParams.NvidiaVisibleDevices, "GPU count should use 'all' - k8s resource requests control allocation")
	})

	t.Run("GPU Operator enabled with specific devices", func(t *testing.T) {
		params := InstallationParams{
			GpuDevices: "GPU-xxxx-yyyy,GPU-zzzz-wwww",
		}
		valueParams := params.GetInfraHelmValuesParams()
		assert.True(t, valueParams.GpuOperatorEnabled)
		assert.Equal(t, "GPU-xxxx-yyyy,GPU-zzzz-wwww", valueParams.NvidiaVisibleDevices)
	})

	t.Run("GPU Operator disabled when no GPUs", func(t *testing.T) {
		params := InstallationParams{
			Gpus:       0,
			GpuDevices: "",
		}
		valueParams := params.GetInfraHelmValuesParams()
		assert.False(t, valueParams.GpuOperatorEnabled)
		assert.Equal(t, "", valueParams.NvidiaVisibleDevices)
	})
}

func TestCalcGpusUsed(t *testing.T) {
	tests := []struct {
		name       string
		gpus       uint
		gpuDevices string
		expected   string
	}{
		{
			name:       "No GPUs",
			gpus:       0,
			gpuDevices: "",
			expected:   "0 GPUs",
		},
		{
			name:       "All GPUs",
			gpus:       0,
			gpuDevices: allGpuDevices,
			expected:   "all GPUs",
		},
		{
			name:       "Specific GPU devices",
			gpus:       0,
			gpuDevices: "0,1,2",
			expected:   "GPU device(s): 0,1,2",
		},
		{
			name:       "Number of GPUs",
			gpus:       3,
			gpuDevices: "",
			expected:   "3 GPUs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calcGpusUsed(tt.gpus, tt.gpuDevices)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetServerHelmValuesParams(t *testing.T) {
	t.Run("KeycloakEnabled when DisabledAuth is false", func(t *testing.T) {
		params := InstallationParams{
			DisabledAuth: false,
		}
		helmParams := params.GetServerHelmValuesParams()
		assert.True(t, helmParams.KeycloakEnabled, "Keycloak should be enabled when DisabledAuth is false")
	})

	t.Run("KeycloakEnabled when DisabledAuth is true", func(t *testing.T) {
		params := InstallationParams{
			DisabledAuth: true,
		}
		helmParams := params.GetServerHelmValuesParams()
		assert.False(t, helmParams.KeycloakEnabled, "Keycloak should be disabled when DisabledAuth is true")
	})
}
