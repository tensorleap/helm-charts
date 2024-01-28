package local

import (
	"bufio"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/tensorleap/helm-charts/pkg/log"
)

type GPU struct {
	Index int
	Name  string
	ID    string
}

func (g GPU) String() string {
	return fmt.Sprintf("GPU %d: %s (%s)", g.Index, g.Name, g.ID)
}

func CheckNvidiaGPU() ([]GPU, error) {
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
	cmd = exec.Command("nvidia-smi", "--query-gpu=index,name,uuid", "--format=csv,noheader")
	listOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error listing NVIDIA GPUs: %s", err)
	}
	gpus := []GPU{}
	scanner := bufio.NewScanner(strings.NewReader(string(listOutput)))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Split(line, ", ")
		index, err := strconv.Atoi(fields[0])
		if err != nil {
			return nil, fmt.Errorf("error parsing GPU index: %s", err)
		}
		gpu := GPU{
			Index: index,
			Name:  fields[1],
			ID:    fields[2],
		}
		gpus = append(gpus, gpu)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning NVIDIA GPU list: %s", err)
	}
	return gpus, nil
}
