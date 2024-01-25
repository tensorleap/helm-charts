package local

import (
	"bufio"
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

	if !strings.Contains(string(gpuOutput), "NVIDIA") {
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

	// List NVIDIA GPUs
	cmd = exec.Command("nvidia-smi", "--query-gpu=name", "--format=csv,noheader")
	listOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error listing NVIDIA GPUs: %s", err)
	}
	gpus := []string{}
	scanner := bufio.NewScanner(strings.NewReader(string(listOutput)))
	for scanner.Scan() {
		gpus = append(gpus, strings.TrimSpace(scanner.Text()))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning NVIDIA GPU list: %s", err)
	}
	return gpus, nil
}
