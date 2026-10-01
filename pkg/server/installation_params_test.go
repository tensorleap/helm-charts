package server

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tensorleap/helm-charts/pkg/local"
)

func TestGetCreateK3sClusterParams(t *testing.T) {
	t.Run("GpuDevices with all", func(t *testing.T) {
		params := InstallationParams{
			GpuDevices: "all",
		}
		valueParams := params.GetInfraHelmValuesParams(nil, "")
		assert.Equal(t, valueParams.NvidiaGpuVisibleDevices, "all")
	})

	t.Run("GpuDevices with gpus count 1", func(t *testing.T) {
		params := InstallationParams{
			Gpus: 2,
		}
		valueParams := params.GetInfraHelmValuesParams(nil, "")
		assert.Equal(t, valueParams.NvidiaGpuVisibleDevices, "0,1")
	})

	t.Run("GpuDevices with 0,1", func(t *testing.T) {
		params := InstallationParams{
			GpuDevices: "0,1",
		}
		valueParams := params.GetInfraHelmValuesParams(nil, "")
		assert.Equal(t, valueParams.NvidiaGpuVisibleDevices, "0,1")
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
		helmParams := params.GetServerHelmValuesParams("unknown")
		assert.True(t, helmParams.KeycloakEnabled, "Keycloak should be enabled when DisabledAuth is false")
	})

	t.Run("KeycloakEnabled when DisabledAuth is true", func(t *testing.T) {
		params := InstallationParams{
			DisabledAuth: true,
		}
		helmParams := params.GetServerHelmValuesParams("unknown")
		assert.False(t, helmParams.KeycloakEnabled, "Keycloak should be disabled when DisabledAuth is true")
	})

	t.Run("GpuCount follows the explicit --gpus count", func(t *testing.T) {
		params := InstallationParams{Gpus: 2}
		helmParams := params.GetServerHelmValuesParams("unknown")
		assert.Equal(t, uint(2), helmParams.GpuCount)
	})
}

func TestDetectGpuCount(t *testing.T) {
	t.Cleanup(func() { checkNvidiaGPU = local.CheckNvidiaGPU })
	stub := func(n int, err error) {
		checkNvidiaGPU = func() ([]local.GPU, error) {
			if err != nil {
				return nil, err
			}
			return make([]local.GPU, n), nil
		}
	}

	tests := []struct {
		name     string
		params   InstallationParams
		detected int
		err      error
		expected uint
	}{
		{"cpu install", InstallationParams{}, 4, nil, 0},
		{"explicit count", InstallationParams{Gpus: 2}, 4, nil, 2},
		{"specific uuids", InstallationParams{GpuDevices: "GPU-a,GPU-b,GPU-c"}, 4, nil, 3},
		{"old-style indexes", InstallationParams{GpuDevices: "0,1"}, 4, nil, 2},
		{"all uses detection", InstallationParams{GpuDevices: allGpuDevices}, 4, nil, 4},
		{"all, detection fails", InstallationParams{GpuDevices: allGpuDevices}, 0, errors.New("no nvidia-smi"), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub(tt.detected, tt.err)
			assert.Equal(t, tt.expected, detectGpuCount(&tt.params))
		})
	}
}

func TestGetEngineProxyEnv(t *testing.T) {
	params := &InstallationParams{}

	t.Run("returns nil when no proxy is set", func(t *testing.T) {
		t.Setenv("HTTP_PROXY", "")
		t.Setenv("http_proxy", "")
		t.Setenv("HTTPS_PROXY", "")
		t.Setenv("https_proxy", "")
		assert.Nil(t, params.GetEngineProxyEnv())
	})

	t.Run("captures proxy and augments no_proxy with in-cluster entries", func(t *testing.T) {
		t.Setenv("HTTPS_PROXY", "http://proxy:3128")
		t.Setenv("NO_PROXY", ".renault.fr")

		env := params.GetEngineProxyEnv()
		assert.Equal(t, "http://proxy:3128", env["https_proxy"])
		assert.Contains(t, env["no_proxy"], ".renault.fr")
		assert.Contains(t, env["no_proxy"], "tensorleap-registry")
		assert.Contains(t, env["no_proxy"], "tensorleap-minio")
		assert.Contains(t, env["no_proxy"], "10.43.0.0/16")
	})

	t.Run("prefers uppercase but falls back to lowercase", func(t *testing.T) {
		t.Setenv("HTTP_PROXY", "")
		t.Setenv("http_proxy", "http://lower:3128")
		env := params.GetEngineProxyEnv()
		assert.Equal(t, "http://lower:3128", env["http_proxy"])
	})
}
