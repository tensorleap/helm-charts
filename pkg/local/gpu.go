package local

import (
	"fmt"
	"os/exec"
	"regexp"
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

	// Regular expression to match NVIDIA GPU entries and extract device IDs
	re := regexp.MustCompile(`([0-9a-f]{2}:[0-9a-f]{2}\.[0-9a-f]) .* NVIDIA`)

	var deviceIDs []string
	for _, line := range strings.Split(outString, "\n") {
		if re.MatchString(line) {
			matches := re.FindStringSubmatch(line)
			if len(matches) > 1 {
				deviceIDs = append(deviceIDs, matches[1])
			}
		}
	}

	return deviceIDs, nil

}
