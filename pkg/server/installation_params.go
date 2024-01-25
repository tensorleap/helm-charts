package server

import (
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/tensorleap/helm-charts/pkg/helm"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/kustomize/kyaml/sliceutil"
)

const currentInstallationVersion = "v0.0.1"
const DefaultRegistryPort = 5699
const DefaultClusterPort = 4589
const AllGpuDevices = "all"

type InstallationParams struct {
	Version          string `json:"version"`
	UseGpu           bool   `json:"useGpu"`
	GpuDevices       string `json:"gpuDevices"`
	ClusterPort      uint   `json:"clusterPort"`
	RegistryPort     uint   `json:"registryPort"`
	DisableMetrics   bool   `json:"disableMetrics"`
	DatasetDirectory string `json:"datasetDirectory"`
	FixK3dDns        bool   `json:"fixK3dDns"`
}

func InitInstallationParamsFromFlags(flags *InstallFlags) (*InstallationParams, error) {

	if err := InitUseGPU(&flags.UseGpu, flags.UseCpu, &flags.GpuDevices); err != nil {
		log.SendCloudReport("error", "Failed to initializing with gpu", "Failed",
			&map[string]interface{}{"useGpu": flags.UseGpu, "error": err.Error()})
		return nil, err
	}

	if err := InitDatasetDirectory(&flags.DatasetDirectory); err != nil {
		log.SendCloudReport("error", "Failed initializing data volume directory", "Failed",
			&map[string]interface{}{"datasetDirectory": flags.DatasetDirectory, "error": err.Error()})
		return nil, err
	}

	return &InstallationParams{
		Version:          currentInstallationVersion,
		UseGpu:           flags.UseGpu,
		ClusterPort:      flags.Port,
		RegistryPort:     flags.RegistryPort,
		DisableMetrics:   flags.DisableMetrics,
		DatasetDirectory: flags.DatasetDirectory,
		FixK3dDns:        flags.FixK3dDns,
	}, nil
}

func InitInstallationParamsFromPreviousOrAsk() (params *InstallationParams, found bool, err error) {
	params, err = LoadInstallationParamsFromPrevious()

	if err == ErrNoInstallationParams {
		params, err = AskInstallationParams()
		if err != nil {
			return nil, false, err
		}
		return
	} else if err != nil {
		return nil, false, err
	}
	found = true
	return
}

func AskInstallationParams() (*InstallationParams, error) {
	installationParams := &InstallationParams{}

	if err := InitUseGPU(&installationParams.UseGpu, false, &installationParams.GpuDevices); err != nil {
		log.SendCloudReport("error", "Failed to initializing with gpu", "Failed",
			&map[string]interface{}{"useGpu": installationParams.UseGpu, "error": err.Error()})
		return nil, err
	}

	if err := InitDatasetDirectory(&installationParams.DatasetDirectory); err != nil {
		log.SendCloudReport("error", "Failed initializing data volume directory", "Failed",
			&map[string]interface{}{"datasetDirectory": installationParams.DatasetDirectory, "error": err.Error()})
		return nil, err
	}

	if err := InitClusterPort(&installationParams.ClusterPort); err != nil {
		log.SendCloudReport("error", "Failed initializing cluster port", "Failed",

			&map[string]interface{}{"clusterPort": installationParams.ClusterPort, "error": err.Error()})
		return nil, err
	}

	if err := InitRegistryPort(&installationParams.RegistryPort); err != nil {
		log.SendCloudReport("error", "Failed initializing registry port", "Failed",
			&map[string]interface{}{"registryPort": installationParams.RegistryPort, "error": err.Error()})
		return nil, err
	}

	return installationParams, nil
}

func InitUseGPU(useGpu *bool, useCpu bool, gpuDevices *string) error {
	if useCpu {
		return nil
	}

	availableDevices, err := local.CheckNvidiaGPU()
	if err != nil {
		log.Warnf("Failed to check NVIDIA GPU: %s", err)
		prompt := survey.Confirm{
			Message: "Do you want to continue without GPU?",
			Default: false,
		}
		continueWithoutGpu := false
		err := survey.AskOne(&prompt, &continueWithoutGpu)
		if err != nil {
			return err
		}
		if continueWithoutGpu {
			*useGpu = false
			return nil
		} else {
			return fmt.Errorf("failed to check NVIDIA GPU: %s", err)
		}

	} else if availableDevices == nil {
		*useGpu = false
		return nil
	}

	gpuOptions := []string{
		"Use all",
		"Not use GPU",
		"Select how many",
		"Select specific",
	}

	if len(availableDevices) == 1 {
		gpuOptions = []string{
			"Use GPU",
			"Not use GPU",
		}
	}

	prompt := survey.Select{
		Message: "Select GPU option:",
		Default: 0,
		Options: gpuOptions,
	}
	gpuOptionIndex := 0
	err = survey.AskOne(&prompt, &gpuOptionIndex)
	if err != nil {
		return err
	}

	switch gpuOptionIndex {
	case 0:
		*useGpu = true
		*gpuDevices = AllGpuDevices
	case 1:
		*useGpu = false
	case 2:
		*useGpu = true
		err := selectHowManyGPUs(availableDevices, gpuDevices)
		if err != nil {
			return err
		}
	case 3:
		*useGpu = true
		err := selectGpuDevices(availableDevices, gpuDevices)
		if err != nil {
			return err
		}
	}

	return nil
}

func selectHowManyGPUs(availableDevices []string, selectedGpuDevices *string) error {
	defaultCount := 1
	if count, err := strconv.Atoi(*selectedGpuDevices); err == nil {
		defaultCount = count
	}

	prompt := survey.Input{
		Message: fmt.Sprintf("How many GPUs (1-%d):", len(availableDevices)),
		Default: fmt.Sprint(defaultCount),
	}
	validate := func(val interface{}) error {
		input, ok := val.(string)
		if !ok {
			return fmt.Errorf("invalid input")
		}

		num, err := strconv.Atoi(input)
		if err != nil {
			return fmt.Errorf("invalid number: %v", err)
		}

		if num < 1 || num > len(availableDevices) {
			return fmt.Errorf("number must be between 1 and %d", len(availableDevices))
		}
		return nil
	}
	count := 0
	err := survey.AskOne(&prompt, &count, survey.WithValidator(validate))
	if err != nil {
		return err
	}
	*selectedGpuDevices = fmt.Sprint(count)
	return nil
}

func selectGpuDevices(availableDevices []string, selectedGpuDevices *string) error {
	defaultDevices := []string{}

	if *selectedGpuDevices == AllGpuDevices {
		defaultDevices = availableDevices
	} else if count, err := strconv.Atoi(*selectedGpuDevices); err == nil {
		defaultDevices = availableDevices[:count]
	} else if *selectedGpuDevices != "" {
		selectedDeviceArray := strings.Split(*selectedGpuDevices, ",")
		for _, device := range selectedDeviceArray {
			trimedDevice := strings.TrimSpace(device)
			if !sliceutil.Contains(availableDevices, trimedDevice) {
				log.Warnf("Device %s is not available", trimedDevice)
				continue
			}
			defaultDevices = append(defaultDevices, trimedDevice)
		}
	}

	prompt := survey.MultiSelect{
		Message: "Select GPU devices:",
		Options: availableDevices,
		Default: defaultDevices,
	}
	selected := []string{}
	validate := func(val interface{}) error {
		if slice, ok := val.([]survey.OptionAnswer); ok {
			if len(slice) == 0 {
				return fmt.Errorf("you must select at least one GPU device")
			}
		} else {
			return fmt.Errorf("invalid selection")
		}
		return nil
	}
	err := survey.AskOne(&prompt, &selected, survey.WithValidator(validate))
	if err != nil {
		return err
	}
	*selectedGpuDevices = strings.Join(selected, ",")
	return nil
}

func InitDatasetDirectory(datasetDirectory *string) error {
	defaultDatasetDirectory := GetDefaultDataVolume()

	if *datasetDirectory == "" {
		fromPath := ""
		prompt := survey.Input{
			Message: "Enter dataset directory:",
			Default: defaultDatasetDirectory,
		}
		err := survey.AskOne(&prompt, &fromPath)
		if err != nil {
			return err
		}
		*datasetDirectory = fromPath
	}
	if !strings.Contains(*datasetDirectory, ":") {
		toPath := ""
		prompt := survey.Input{
			Message: "Enter container dataset directory:",
			Default: *datasetDirectory,
		}
		err := survey.AskOne(&prompt, &toPath)
		if err != nil {
			return err
		}
		*datasetDirectory = fmt.Sprintf("%s:%s", *datasetDirectory, toPath)
	}
	log.SendCloudReport("info", "Init data volume", "Starting",
		&map[string]interface{}{"params": map[string]interface{}{"datasetDirectory": datasetDirectory}},
	)

	dataPath := strings.Split(*datasetDirectory, ":")[0]
	return os.MkdirAll(dataPath, 0777)
}

func GetDefaultDataVolume() string {
	defaultDataPath := fmt.Sprintf("%s/tensorleap/data", getHomePath())
	return defaultDataPath
}

func InitClusterPort(clusterPort *uint) error {
	*clusterPort = DefaultClusterPort
	return nil
	// will add this later
	// return InitPort(clusterPort, DefaultClusterPort, "Enter cluster port:")
}

func InitRegistryPort(registryPort *uint) error {
	*registryPort = DefaultRegistryPort
	return nil
	// will add this later
	// return InitPort(registryPort, DefaultRegistryPort, "Enter registry port:")
}

func InitPort(port *uint, defaultPort uint, message string) error {
	if *port == 0 {
		value := "0"
		portValidator := func(val interface{}) error {
			input, ok := val.(string)
			if !ok {
				return fmt.Errorf("invalid input")
			}
			num, err := strconv.Atoi(input)
			if err != nil {
				return fmt.Errorf("invalid number: %v", err)
			}
			if num < 0 || num > 65535 {
				return fmt.Errorf("port number must be between 0 and 65535")
			}
			return nil
		}
		prompt := survey.Input{
			Message: message,
			Default: fmt.Sprint(defaultPort),
		}
		err := survey.AskOne(&prompt, &value, survey.WithValidator(portValidator))
		if err != nil {
			return err
		}
		port64, _ := strconv.ParseUint(value, 10, 32)
		*port = uint(port64)
	}
	return nil
}

func (params *InstallationParams) GetServerHelmValuesParams() *helm.ServerHelmValuesParams {
	dataContainerPath := strings.Split(params.DatasetDirectory, ":")[1]

	return &helm.ServerHelmValuesParams{
		Gpu:                   params.UseGpu,
		LocalDataDirectory:    dataContainerPath,
		DisableDatadogMetrics: params.DisableMetrics,
	}
}

func (params *InstallationParams) GetCreateK3sClusterParams() *k3d.CreateK3sClusterParams {
	volumes := []string{
		fmt.Sprintf("%v:%v", local.STANDALONE_DIR, local.STANDALONE_DIR),
		params.DatasetDirectory,
	}
	gpuRequest := ""
	if params.UseGpu {
		if params.GpuDevices == AllGpuDevices || params.GpuDevices == "" {
			gpuRequest = AllGpuDevices
		} else if count, err := strconv.Atoi(params.GpuDevices); err == nil {
			gpuRequest = fmt.Sprintf("count=%d", count)
		} else {
			gpuRequest = fmt.Sprintf("devices=%s", params.GpuDevices)
		}
	}

	return &k3d.CreateK3sClusterParams{
		WithGpu:    params.UseGpu,
		Port:       params.ClusterPort,
		Volumes:    volumes,
		GpuRequest: gpuRequest,
	}
}

func (params *InstallationParams) GetCreateRegistryParams() *k3d.CreateRegistryParams {
	volumes := []string{
		fmt.Sprintf("%v:%v", path.Join(local.STANDALONE_DIR, "registry"), "/var/lib/registry"),
	}

	return &k3d.CreateRegistryParams{
		Port:    params.RegistryPort,
		Volumes: volumes,
	}
}

func (params *InstallationParams) Save() error {
	b, err := yaml.Marshal(params)
	if err != nil {
		return err
	}
	return os.WriteFile(local.GetInstallationParamsPath(), b, 0644)
}

var ErrNoInstallationParams = fmt.Errorf("no installation params")

func LoadInstallationParamsFromPrevious() (*InstallationParams, error) {
	b, err := os.ReadFile(local.GetInstallationParamsPath())
	if os.IsNotExist(err) {
		return nil, ErrNoInstallationParams
	} else if err != nil {
		return nil, err
	}
	params, err := LoadInstallationParams(b)
	if err != nil {
		return nil, err
	}
	return params, nil
}

func LoadInstallationParams(paramsBytes []byte) (*InstallationParams, error) {
	params := &InstallationParams{}
	err := yaml.Unmarshal(paramsBytes, params)
	if err != nil {
		return nil, err
	}
	return params, nil
}
