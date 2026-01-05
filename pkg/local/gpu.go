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

	switch runtime.GOOS {
	case "darwin":
		return checkNvidiaOnMac()
	case "linux":
		return checkNvidiaOnLinux()
	default:
		// Same semantics as before for unsupported OSs
		return nil, fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func checkNvidiaOnMac() ([]GPU, error) {
	log.Info("Using system_profiler to detect NVIDIA GPU on macOS...")

	cmd := exec.Command("system_profiler", "SPDisplaysDataType")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error executing system_profiler: %w", err)
	}

	if !strings.Contains(string(out), "NVIDIA") {
		log.Info("No NVIDIA GPU found.")
		return nil, nil
	}

	log.Info("NVIDIA GPU found on macOS. Querying details via nvidia-smi...")
	return queryGPUsViaNvidiaSMI()
}

func checkNvidiaOnLinux() ([]GPU, error) {
	// On Linux we *used* to rely on lspci. Now we just let nvidia-smi
	// tell us whether there is a usable NVIDIA GPU.
	log.Info("Using nvidia-smi to detect NVIDIA GPU...")

	// Check if nvidia-smi exists first
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		log.Info("not found nvidia-smi")
		return nil, nil
	}

	return queryGPUsViaNvidiaSMI()
}

func queryGPUsViaNvidiaSMI() ([]GPU, error) {
	// Make sure nvidia-smi exists at all.
	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		return nil, fmt.Errorf("nvidia-smi not found in PATH: %w", err)
	}

	log.Info("Checking NVIDIA driver and version...")
	cmd := exec.Command("nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader")
	driverOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("NVIDIA driver not found or nvidia-smi not available: %w", err)
	}
	driverVersion := strings.TrimSpace(string(driverOutput))
	log.Infof("NVIDIA Driver Version: %s", driverVersion)

	// List GPUs.
	cmd = exec.Command("nvidia-smi", "--query-gpu=index,name,uuid", "--format=csv,noheader")
	listOutput, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error listing NVIDIA GPUs: %w", err)
	}

	gpus := []GPU{}
	scanner := bufio.NewScanner(strings.NewReader(string(listOutput)))
	failedToParseGPUsCount := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Split(line, ", ")
		if len(fields) < 3 {
			return nil, fmt.Errorf("unexpected nvidia-smi output line: %q", line)
		}

		index, err := strconv.Atoi(fields[0])
		if err != nil {
			failedToParseGPUsCount++
			log.Warnf("Failed to parse GPU index (gpu row:%q) Try to continue. error: %v", strings.Join(fields, ", "), err)
			continue
		}

		gpus = append(gpus, GPU{
			Index: index,
			Name:  fields[1],
			ID:    fields[2],
		})
	}
	if failedToParseGPUsCount > 0 && len(gpus) == 0 {
		return nil, fmt.Errorf("failed to parse any GPU(s)")
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning NVIDIA GPU list: %w", err)
	}

	if len(gpus) == 0 {
		log.Info("No NVIDIA GPU found (nvidia-smi returned an empty list).")
		return nil, nil
	}

	log.Infof("Found %d NVIDIA GPU(s).", len(gpus))
	return gpus, nil
}

func CheckDockerNvidia2Driver() (bool, error) {
	log.Info("Checking docker-nvidia2 driver...")
	cmd := exec.Command("docker", "info", "--format", "{{json .Runtimes}}")
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to execute docker info: %w", err)
	}

	hasNvidia := strings.Contains(strings.ToLower(string(out)), "nvidia")
	if hasNvidia {
		log.Info("docker-nvidia2 driver found.")
	} else {
		log.Warn("Missing docker-nvidia2 driver. Install nvidia-docker2 to enable GPU support.")
	}

	return hasNvidia, nil
}
