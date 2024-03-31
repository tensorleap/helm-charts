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
)

const currentInstallationVersion = "v0.0.1"
const defaultRegistryPort = 5699
const defaultHttpPort = 80
const defaultHttpsPort = 443
const allGpuDevices = "all"

type InstallationParams struct {
	Version          string `json:"version"`
	GpuDevices       string `json:"gpuDevices,omitempty"`
	Gpus             uint   `json:"gpus,omitempty"`
	Port             uint   `json:"clusterPort"`
	Domain           string `json:"domain"`
	RegistryPort     uint   `json:"registryPort"`
	DisableMetrics   bool   `json:"disableMetrics"`
	DatasetDirectory string `json:"datasetDirectory"`
	FixK3dDns        bool   `json:"fixK3dDns"`
	TLSParams
}

type TLSParams struct {
	Enabled bool   `json:"enabled"`
	Cert    string `json:"cert,omitempty"`
	Key     string `json:"key,omitempty"`
}

func GetTLSParams(flags TLSFlags) (*TLSParams, error) {
	if !flags.IsEnabled() {
		return &TLSParams{
			Enabled: false,
		}, nil
	}

	cert, err := os.ReadFile(flags.CertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read certificate file: %v", err)
	}
	key, err := os.ReadFile(flags.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %v", err)
	}

	// If chain file is provided, append it to the cert
	if flags.ChainPath != "" {
		chain, err := os.ReadFile(flags.ChainPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read chain file: %v", err)
		}
		cert = append(cert, chain...)
	}

	return &TLSParams{
		Enabled: true,
		Cert:    string(cert),
		Key:     string(key),
	}, nil
}

func (params *TLSParams) GetTLSHelmParams() *helm.TLSParams {
	return &helm.TLSParams{
		Enabled: params.Enabled,
		Cert:    params.Cert,
		Key:     params.Key,
	}
}

func (params *InstallationParams) IsUseGpu() bool {
	return params.Gpus > 0 || params.GpuDevices != ""
}

func InitInstallationParamsFromFlags(flags *InstallFlags) (*InstallationParams, error) {

	if err := InitUseGPU(&flags.Gpus, &flags.GpuDevices, flags.UseCpu); err != nil {
		log.SendCloudReport("error", "Failed to initializing with gpu", "Failed",
			&map[string]interface{}{"Gpus": flags.Gpus, "GpusDevices": flags.GpuDevices, "error": err.Error()})
		return nil, err
	}

	if err := InitDatasetDirectory(&flags.DatasetDirectory); err != nil {
		log.SendCloudReport("error", "Failed initializing data volume directory", "Failed",
			&map[string]interface{}{"datasetDirectory": flags.DatasetDirectory, "error": err.Error()})
		return nil, err
	}

	tlsParams, err := GetTLSParams(flags.TLSFlags)
	if err != nil {
		return nil, fmt.Errorf("failed to get TLS params: %v", err)
	}

	
	previousParams, err := LoadInstallationParamsFromPrevious()
	if err == nil {
		isAskUserToUsePreviousTlsConfig := previousParams.TLSParams.Enabled && !tlsParams.Enabled
		if isAskUserToUsePreviousTlsConfig {
			prompt := survey.Confirm{
				Message: "Do you want to use the previous TLS configuration?",
				Default: true,
			}
			usePreviousTlsConfig := true
			err := survey.AskOne(&prompt, &usePreviousTlsConfig)
			if err != nil {
				return nil, err
			}
			if usePreviousTlsConfig {
				tlsParams = &previousParams.TLSParams
				flags.Domain = previousParams.Domain
			}
		}
	}

	port := flags.Port
	if tlsParams.Enabled {
		port = flags.TLSFlags.Port
	}

	return &InstallationParams{
		Version:          currentInstallationVersion,
		Gpus:             flags.Gpus,
		GpuDevices:       flags.GpuDevices,
		Port:             port,
		RegistryPort:     flags.RegistryPort,
		DisableMetrics:   flags.DisableMetrics,
		DatasetDirectory: flags.DatasetDirectory,
		FixK3dDns:        flags.FixK3dDns,
		Domain:           flags.Domain,
		TLSParams:        *tlsParams,
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

	if err := InitUseGPU(&installationParams.Gpus, &installationParams.GpuDevices, false); err != nil {
		log.SendCloudReport("error", "Failed to initializing with gpu", "Failed",
			&map[string]interface{}{"Gpus": installationParams.Gpus, "GpuDevices": installationParams.GpuDevices, "error": err.Error()})
		return nil, err
	}

	if err := InitDatasetDirectory(&installationParams.DatasetDirectory); err != nil {
		log.SendCloudReport("error", "Failed initializing data volume directory", "Failed",
			&map[string]interface{}{"datasetDirectory": installationParams.DatasetDirectory, "error": err.Error()})
		return nil, err
	}

	if err := InitClusterPort(&installationParams.Port); err != nil {
		log.SendCloudReport("error", "Failed initializing cluster port", "Failed",

			&map[string]interface{}{"clusterPort": installationParams.Port, "error": err.Error()})
		return nil, err
	}

	if err := InitRegistryPort(&installationParams.RegistryPort); err != nil {
		log.SendCloudReport("error", "Failed initializing registry port", "Failed",
			&map[string]interface{}{"registryPort": installationParams.RegistryPort, "error": err.Error()})
		return nil, err
	}

	return installationParams, nil
}

func InitUseGPU(gpus *uint, gpuDevices *string, useCpu bool) error {
	if useCpu {
		return nil
	}

	availableDevices, checkNvidiaErr := local.CheckNvidiaGPU()

	if checkNvidiaErr != nil {
		log.Warnf("Failed to check NVIDIA GPU: %s", checkNvidiaErr)
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
			*gpus = 0
			return nil
		} else {
			return fmt.Errorf("failed to check NVIDIA GPU: %s", checkNvidiaErr)
		}

	} else if availableDevices == nil {
		*gpus = 0
		return nil
	}

	gpuOptions := []string{
		"Use all",
		"Not use GPU",
		"Select how many",
		"Select specific",
	}

	prompt := survey.Select{
		Message: "Select GPU option:",
		Default: 0,
		Options: gpuOptions,
	}
	gpuOptionIndex := 0
	err := survey.AskOne(&prompt, &gpuOptionIndex)
	if err != nil {
		return err
	}

	switch gpuOptionIndex {
	case 0:
		*gpuDevices = allGpuDevices
	case 1:
		*gpus = 0
		*gpuDevices = ""
	case 2:
		err := selectHowManyGPUs(availableDevices, gpus)
		if err != nil {
			return err
		}
	case 3:
		err := selectGpuDevices(availableDevices, gpuDevices)
		if err != nil {
			return err
		}
	}

	return nil
}

func selectHowManyGPUs(availableDevices []local.GPU, selectedGpus *uint) error {
	defaultCount := 1
	if *selectedGpus > 0 {
		defaultCount = int(*selectedGpus)
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
	*selectedGpus = uint(count)
	return nil
}

func selectGpuDevices(availableDevices []local.GPU, selectedGpuDevices *string) error {
	defaultDevices := []string{}
	availableGpusNames := []string{}
	for _, device := range availableDevices {
		availableGpusNames = append(availableGpusNames, device.String())
	}

	if *selectedGpuDevices == allGpuDevices {
		defaultDevices = availableGpusNames
	} else {
		selectedDeviceArray := strings.Split(*selectedGpuDevices, ",")
		for _, device := range selectedDeviceArray {
			trimedDevice := strings.TrimSpace(device)
			asNumber, err := strconv.Atoi(trimedDevice)
			if err != nil || asNumber >= len(availableDevices) {
				log.Warnf("Device %s is not available", device)
				continue
			}
			defaultDevices = append(defaultDevices, availableGpusNames[asNumber])
		}
	}

	prompt := survey.MultiSelect{
		Message: "Select GPU devices:",
		Options: availableGpusNames,
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

	devicesIndexes := []string{}
	for _, device := range selected {
		for i, availableDevice := range availableGpusNames {
			if device == availableDevice {
				devicesIndexes = append(devicesIndexes, fmt.Sprint(i))
			}
		}
	}

	*selectedGpuDevices = strings.Join(devicesIndexes, ",")

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
	*clusterPort = defaultHttpPort
	return nil
	// will add this later
	// return InitPort(clusterPort, DefaultClusterPort, "Enter cluster port:")
}

func InitRegistryPort(registryPort *uint) error {
	*registryPort = defaultRegistryPort
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

func (params *InstallationParams) CalcUrl() string {
	var scheme, url string

	port := params.Port
	if params.TLSParams.Enabled {
		scheme = "https"
	} else {
		scheme = "http"
	}

	if params.Domain == "" {
		params.Domain = "localhost"
	}

	isDefaultPort := params.TLSParams.Enabled && port == defaultHttpsPort || (!params.TLSParams.Enabled && port == defaultHttpPort)

	if isDefaultPort {
		url = fmt.Sprintf("%s://%s", scheme, params.Domain)
	} else {
		url = fmt.Sprintf("%s://%s:%d", scheme, params.Domain, port)
	}

	return url
}

func (params *InstallationParams) GetServerHelmValuesParams() *helm.ServerHelmValuesParams {
	dataContainerPath := strings.Split(params.DatasetDirectory, ":")[1]

	tlsParams := params.TLSParams.GetTLSHelmParams()
	url := params.CalcUrl()

	return &helm.ServerHelmValuesParams{
		Gpu:                   params.IsUseGpu(),
		LocalDataDirectory:    dataContainerPath,
		DisableDatadogMetrics: params.DisableMetrics,
		Domain:                params.Domain,
		Url:                   url,
		Tls:                   *tlsParams,
	}
}

func (params *InstallationParams) GetInfraHelmValuesParams() *helm.InfraHelmValuesParams {

	nvidiaGpuVisibleDevices := ""
	nvidiaGpuEnable := params.IsUseGpu()

	if nvidiaGpuEnable {
		if params.GpuDevices == allGpuDevices {
			nvidiaGpuVisibleDevices = allGpuDevices
		} else if params.GpuDevices != "" {
			nvidiaGpuVisibleDevices = params.GpuDevices
		} else if params.Gpus > 0 {
			devices := []string{}
			for i := 0; i < int(params.Gpus); i++ {
				devices = append(devices, fmt.Sprint(i))
			}
			nvidiaGpuVisibleDevices = strings.Join(devices, ",")
		} else {
			nvidiaGpuVisibleDevices = allGpuDevices
		}
	}

	return &helm.InfraHelmValuesParams{
		NvidiaGpuEnable:         nvidiaGpuEnable,
		NvidiaGpuVisibleDevices: nvidiaGpuVisibleDevices,
	}
}

func (params *InstallationParams) GetCreateK3sClusterParams() *k3d.CreateK3sClusterParams {
	volumes := []string{
		fmt.Sprintf("%v:%v", local.STANDALONE_DIR, local.STANDALONE_DIR),
		params.DatasetDirectory,
	}

	useGpu := params.IsUseGpu()

	return &k3d.CreateK3sClusterParams{
		WithGpu: useGpu,
		Port:    params.Port,
		Volumes: volumes,
		IsHttps: params.TLSParams.Enabled,
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
