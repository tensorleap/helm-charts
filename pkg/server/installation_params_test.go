package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCreateK3sClusterParams(t *testing.T) {
	t.Run("GpuDevices with all", func(t *testing.T) {
		params := InstallationParams{
			GpuDevices: "all",
		}
		valueParams := params.GetInfraHelmValuesParams()
		assert.Equal(t, valueParams.NvidiaGpuVisibleDevices, "all")
	})

	t.Run("GpuDevices with gpus count 1", func(t *testing.T) {
		params := InstallationParams{
			Gpus: 2,
		}
		valueParams := params.GetInfraHelmValuesParams()
		assert.Equal(t, valueParams.NvidiaGpuVisibleDevices, "0,1")
	})

	t.Run("GpuDevices with 0,1", func(t *testing.T) {
		params := InstallationParams{
			GpuDevices: "0,1",
		}
		valueParams := params.GetInfraHelmValuesParams()
		assert.Equal(t, valueParams.NvidiaGpuVisibleDevices, "0,1")
	})

}

func TestInitK3sParams(t *testing.T) {
	t.Run("Parse multi envs", func(t *testing.T) {
		flags := InstallFlags{
			K3sEnvs: []string{"K3S_TOKEN=x"},
		}
		valueParams, err := InitK3sCustomEnvs(&flags)
		assert.Nil(t, err)
		assert.Equal(t, valueParams, map[string]string{
			"K3S_TOKEN": "x",
		})
	})

}
