package local

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/tensorleap/helm-charts/pkg/log"
)

func CheckNvidiaGPU() ([]string, error) {
	log.Info("Check for NVIDIA GPU")
	var cmd *exec.Cmd
	switch os := runtime.GOOS; os {
	case "darwin": // macOS
		cmd = exec.Command("system_profiler", "SPDisplaysDataType")
	case "linux":
		cmd = exec.Command("lspci")
	default:
		return nil, fmt.Errorf("unsupported operating system: %s", os)
	}
	gpuOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error executing lspci command: %s", err)
	}
	outString := string(gpuOutput)

	if !strings.Contains(outString, "NVIDIA") {
		log.Info("No NVIDIA GPU found.")
		return nil, nil
	}
	log.Info("NVIDIA GPU found.")
	// Check for NVIDIA driver and version
	log.Info("Checking NVIDIA driver and version...")
	cmd = exec.Command("nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader")
	driverOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("NVIDIA driver not found or nvidia-smi not available: %s", err)
	}
	driverVersion := strings.TrimSpace(string(driverOutput))
	fmt.Printf("NVIDIA Driver Version: %s\n", driverVersion)

	// Filter output for NVIDIA GPUs
	gpus := []string{}
	for _, line := range strings.Split(outString, "\n") {
		if strings.Contains(line, "NVIDIA") {
			gpus = append(gpus, line)
		}
	}

	return gpus, nil
}
