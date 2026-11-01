package server

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tensorleap/helm-charts/pkg/local"
)

var testGpus = []local.GPU{
	{Index: 0, Name: "NVIDIA A10", ID: "GPU-aaa"},
	{Index: 1, Name: "NVIDIA A10", ID: "GPU-bbb"},
}

// stubPromptEnv simulates the installer's surroundings: the GPUs nvidia-smi
// reports, whether --yes was passed, and whether stdin is a terminal.
func stubPromptEnv(t *testing.T, detected []local.GPU, detectErr error, useDefault, isTerminal bool) {
	originalIsStdinTerminal := local.IsStdinTerminal
	t.Cleanup(func() {
		checkNvidiaGPU = local.CheckNvidiaGPU
		checkDockerNvidia2Driver = local.CheckDockerNvidia2Driver
		local.IsStdinTerminal = originalIsStdinTerminal
	})
	checkNvidiaGPU = func() ([]local.GPU, error) { return detected, detectErr }
	checkDockerNvidia2Driver = func() (bool, error) { return true, nil }
	local.IsStdinTerminal = func() bool { return isTerminal }
	if useDefault {
		t.Setenv("TL_USE_DEFAULT_OPTION", "true")
	} else {
		t.Setenv("TL_USE_DEFAULT_OPTION", "")
	}
}

func TestInitUseGPU(t *testing.T) {
	previousTwoGpus := &InstallationParams{Gpus: 2}

	tests := []struct {
		name           string
		gpus           uint
		gpuDevices     string
		useCpu         bool
		previous       *InstallationParams
		detected       []local.GPU
		detectErr      error
		useDefault     bool
		wantGpus       uint
		wantGpuDevices string
		wantErr        string
	}{
		// Explicit flags are the answer: no picker (no terminal here, so a prompt would fail) and --yes keeps them.
		{name: "--gpus without --yes", gpus: 1, detected: testGpus, wantGpus: 1},
		{name: "--gpus with --yes is not replaced by all", gpus: 1, detected: testGpus, useDefault: true, wantGpus: 1},
		{name: "--gpu-devices index with --yes", gpuDevices: "1", detected: testGpus, useDefault: true, wantGpuDevices: "1"},
		{name: "--gpu-devices uuids", gpuDevices: "GPU-aaa, GPU-bbb", detected: testGpus, wantGpuDevices: "GPU-aaa, GPU-bbb"},
		{name: "--gpu-devices all", gpuDevices: allGpuDevices, detected: testGpus, wantGpuDevices: allGpuDevices},
		{name: "explicit flags win over previous settings", gpuDevices: "GPU-bbb", previous: previousTwoGpus, detected: testGpus, wantGpuDevices: "GPU-bbb"},
		{name: "--gpus more than detected", gpus: 3, detected: testGpus, wantErr: "--gpus 3 is more than the 2 GPU(s) detected"},
		{name: "--gpu-devices unknown uuid", gpuDevices: "GPU-zzz", detected: testGpus, wantErr: `GPU "GPU-zzz" was not detected`},
		{name: "--gpu-devices unknown index", gpuDevices: "0,5", detected: testGpus, wantErr: `GPU "5" was not detected`},
		{name: "--gpus and --gpu-devices together", gpus: 1, gpuDevices: "0", detected: testGpus, wantErr: "not both"},
		{name: "--gpus without a GPU on the machine", gpus: 1, detected: nil, wantErr: "no NVIDIA GPU was detected"},
		{name: "--gpus, detection fails, --yes keeps them", gpus: 1, detectErr: errors.New("nvidia-smi failed"), useDefault: true, wantGpus: 1},
		{name: "--gpus, detection fails, no terminal", gpus: 1, detectErr: errors.New("nvidia-smi failed"), wantErr: "no terminal to ask whether to continue without GPU validation"},
		// --cpu
		{name: "--cpu", useCpu: true, detected: testGpus},
		{name: "--cpu with --gpus", useCpu: true, gpus: 1, detected: testGpus, wantErr: "--cpu cannot be combined"},
		// No GPU flag: unchanged behavior, but no EOF without a terminal.
		{name: "no flag with --yes uses all GPUs", detected: testGpus, useDefault: true, wantGpuDevices: allGpuDevices},
		{name: "no flag without terminal", detected: testGpus, wantErr: "no terminal to ask which GPUs to use: pass --gpus <n>, --gpu-devices <ids>, --cpu, or --yes"},
		{name: "no flag, no GPU on the machine", detected: nil, wantGpus: 0},
		{name: "no flag, previous settings with --yes", previous: previousTwoGpus, detected: testGpus, useDefault: true, wantGpus: 2},
		{name: "no flag, previous settings without terminal", previous: previousTwoGpus, detected: testGpus, wantErr: "no terminal to ask whether to use the previous GPU settings (2 GPUs)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubPromptEnv(t, tt.detected, tt.detectErr, tt.useDefault, false)
			gpus, gpuDevices := tt.gpus, tt.gpuDevices

			err := InitUseGPU(&gpus, &gpuDevices, tt.useCpu, tt.previous)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantGpus, gpus)
			assert.Equal(t, tt.wantGpuDevices, gpuDevices)
		})
	}
}

func TestInitDatasetVolumesWithoutTerminal(t *testing.T) {
	t.Run("no -v and no --yes fails with how to answer", func(t *testing.T) {
		stubPromptEnv(t, nil, nil, false, false)
		volumes := []string{}
		err := InitDatasetVolumes(&volumes, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no terminal to ask which dataset volume to mount: pass -v <host-dir>[:<container-dir>], or --yes")
	})

	t.Run("previous volumes and no --yes fails with how to answer", func(t *testing.T) {
		stubPromptEnv(t, nil, nil, false, false)
		volumes := []string{}
		err := InitDatasetVolumes(&volumes, &InstallationParams{DatasetVolumes: []string{"/data:/data"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no terminal to ask whether to use the previous dataset volumes")
	})

	t.Run("-v needs no terminal", func(t *testing.T) {
		stubPromptEnv(t, nil, nil, false, false)
		dir := t.TempDir()
		volumes := []string{dir}
		require.NoError(t, InitDatasetVolumes(&volumes, nil))
		assert.Equal(t, []string{dir + ":" + dir}, volumes)
	})
}

func TestConfirmWithoutTerminal(t *testing.T) {
	t.Run("--yes answers the default", func(t *testing.T) {
		stubPromptEnv(t, nil, nil, true, false)
		answer, err := confirm("Move the storage?", true)
		require.NoError(t, err)
		assert.True(t, answer)

		answer, err = confirm("Overwrite the storage?", false)
		require.NoError(t, err)
		assert.False(t, answer)
	})

	t.Run("no --yes fails with how to answer", func(t *testing.T) {
		stubPromptEnv(t, nil, nil, false, false)
		_, err := confirm("Move the storage?", true)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `pass --yes to answer "yes" to: Move the storage?`)
	})
}

func TestAskForReinstallWithoutTerminal(t *testing.T) {
	stubPromptEnv(t, nil, nil, false, false)
	_, err := AskForReinstall()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pass --yes to reinstall")
}
