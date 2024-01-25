package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCreateK3sClusterParams(t *testing.T) {
	t.Run("GpuDevices with all", func(t *testing.T) {
		params := InstallationParams{
			UseGpu:     true,
			GpuDevices: "all",
		}
		createK3sClusterParams := params.GetCreateK3sClusterParams()
		assert.Equal(t, createK3sClusterParams.GpuRequest, "all")
	})

	t.Run("GpuDevices with 1", func(t *testing.T) {
		params := InstallationParams{
			UseGpu:     true,
			GpuDevices: "1",
		}
		createK3sClusterParams := params.GetCreateK3sClusterParams()
		assert.Equal(t, createK3sClusterParams.GpuRequest, "count=1")
	})

	t.Run("GpuDevices with GPU-0,GPU-1", func(t *testing.T) {
		params := InstallationParams{
			UseGpu:     true,
			GpuDevices: "device=0,1",
		}
		createK3sClusterParams := params.GetCreateK3sClusterParams()
		assert.Equal(t, createK3sClusterParams.GpuRequest, "device=0,1")
	})

}
