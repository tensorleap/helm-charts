package server

import (
	"fmt"
	"net/url"
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

const CurrentInstallationVersion = "v0.0.1"
const DefaultRegistryPort = 5699
const DefaultHttpPort = 4589
const DefaultHttpsPort = 443
const allGpuDevices = "all"

type InstallationParams struct {
	Version                     string   `json:"version"`
	GpuDevices                  string   `json:"gpuDevices,omitempty"`
	Gpus                        uint     `json:"gpus,omitempty"`
	Port                        uint     `json:"clusterPort"`
	Domain                      string   `json:"domain"`
	ProxyUrl                    string   `json:"proxyUrl"`
	RegistryPort                uint     `json:"registryPort"`
	DisableMetrics              bool     `json:"disableMetrics"`
	DatasetDirectory_DEPRECATED string   `json:"datasetDirectory,omitempty" yaml:"datasetDirectory,omitempty"`
	DatasetVolumes              []string `json:"datasetVolumes"`
	FixK3dDns                   bool     `json:"fixK3dDns"`
	CpuLimit                    string   `json:"cpuLimit,omitempty"`
	ClearInstallationImages     bool     `json:"removeInstallationImages,omitempty"`
	TLSParams
}

type TLSParams struct {
	Enabled bool   `json:"enabled"`
	Cert    string `json:"cert,omitempty"`
	Key     string `json:"key,omitempty"`
	Port    uint   `json:"port,omitempty"`
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
		Port:    flags.Port,
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
		log.SendCloudReport("error", "Failed initializing with GPU", "Failed",
			&map[string]interface{}{"Gpus": flags.Gpus, "GpusDevices": flags.GpuDevices, "error": err.Error()})
		return nil, err
	}

	if err := InitDatasetVolumes(&flags.DatasetVolumes); err != nil {
		log.SendCloudReport("error", "Failed initializing data volume directory", "Failed",
			&map[string]interface{}{"datasetVolumes": flags.DatasetVolumes, "error": err.Error()})
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
		isAskUserToUsePreviousProxyUrl := previousParams.ProxyUrl != "" && flags.ProxyUrl == ""
		if isAskUserToUsePreviousProxyUrl {
			prompt := survey.Confirm{
				Message: fmt.Sprintf("Do you want to use the previous proxy url? (%s)", previousParams.ProxyUrl),
				Default: true,
			}
			usePreviousProxyUrl := true
			err := survey.AskOne(&prompt, &usePreviousProxyUrl)
			if err != nil {
				return nil, err
			}
			if usePreviousProxyUrl {
				flags.ProxyUrl = previousParams.ProxyUrl
			}
		}
		isRemoveInstallationNotSet := flags.ClearInstallationImages == nil
		if isRemoveInstallationNotSet {
			flags.ClearInstallationImages = &previousParams.ClearInstallationImages
		}
	}
	if flags.ClearInstallationImages == nil {
		flags.ClearInstallationImages = new(bool)
	}

	return &InstallationParams{
		Version:                 CurrentInstallationVersion,
		Gpus:                    flags.Gpus,
		GpuDevices:              flags.GpuDevices,
		Port:                    flags.Port,
		RegistryPort:            flags.RegistryPort,
		DisableMetrics:          flags.DisableMetrics,
		DatasetVolumes:          flags.DatasetVolumes,
		FixK3dDns:               flags.FixK3dDns,
		Domain:                  flags.Domain,
		ProxyUrl:                flags.ProxyUrl,
		CpuLimit:                flags.CpuLimit,
		TLSParams:               *tlsParams,
		ClearInstallationImages: *flags.ClearInstallationImages,
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
		log.SendCloudReport("error", "Failed initializing with GPU", "Failed",
			&map[string]interface{}{"Gpus": installationParams.Gpus, "GpuDevices": installationParams.GpuDevices, "error": err.Error()})
		return nil, err
	}

	if err := InitDatasetVolumes(&installationParams.DatasetVolumes); err != nil {
		log.SendCloudReport("error", "Failed initializing data volume directory", "Failed",
			&map[string]interface{}{"datasetVolumes": installationParams.DatasetVolumes, "error": err.Error()})
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

func InitDatasetVolumes(datasetVolumes *[]string) error {
	defaultDatasetVolume := GetDefaultDataVolume()

	if len(*datasetVolumes) == 0 {
		addAnother := true
		for addAnother {
			path := ""
			if len(*datasetVolumes) > 0 {
				defaultDatasetVolume = ""
			}
			prompt := survey.Input{
				Message: "Enter dataset volume:",
				Default: defaultDatasetVolume,
			}
			err := survey.AskOne(&prompt, &path)
			if err != nil {
				return err
			}
			path = strings.TrimSpace(path)
			if path == "" {
				break
			}
			if !strings.Contains(path, ":") {
				path = fmt.Sprintf("%s:%s", path, path)
			}
			*datasetVolumes = append(*datasetVolumes, path)

			confirmPrompt := survey.Confirm{
				Message: "Add another dataset volume?",
				Default: false,
			}
			err = survey.AskOne(&confirmPrompt, &addAnother)
			if err != nil {
				return err
			}
		}
	}

	for i, path := range *datasetVolumes {
		if !strings.Contains(path, ":") {
			(*datasetVolumes)[i] = fmt.Sprintf("%s:%s", path, path)
		}

		dataPath := strings.Split(path, ":")[0]
		err := os.MkdirAll(dataPath, 0777)
		if err != nil {
			return fmt.Errorf("failed to create dataset volume directory: %v", err)
		}
	}

	log.SendCloudReport("info", "Init data volume", "Starting",
		&map[string]interface{}{"params": map[string]interface{}{"datasetVolumes": datasetVolumes}},
	)
	return nil
}

func GetDefaultDataVolume() string {
	defaultDataPath := fmt.Sprintf("%s/tensorleap/data", getHomePath())
	return defaultDataPath
}

func InitClusterPort(clusterPort *uint) error {
	*clusterPort = DefaultHttpPort
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

func (params *InstallationParams) CalcBestPath() string {
	bestPath := ""
	if params.ProxyUrl != "" {
		// parse the proxy url and return the path
		url, err := url.Parse(params.ProxyUrl)
		if err != nil {
			log.Fatalf("Invalid proxy url: %s, error: %v", params.ProxyUrl, err)
		}
		bestPath = url.Path
		bestPath = strings.Trim(bestPath, "/")
	}
	return bestPath
}

func (params *InstallationParams) CalcUrl() string {

	var scheme, url string

	port := params.Port
	if params.TLSParams.Enabled {
		port = params.TLSParams.Port
	}

	if params.TLSParams.Enabled {
		scheme = "https"
	} else {
		scheme = "http"
	}

	if params.Domain == "" {
		params.Domain = "localhost"
	}

	isDefaultPort := params.TLSParams.Enabled && port == 443 || (!params.TLSParams.Enabled && port == 80)

	if isDefaultPort {
		url = fmt.Sprintf("%s://%s", scheme, params.Domain)
	} else {
		url = fmt.Sprintf("%s://%s:%d", scheme, params.Domain, port)
	}

	return url
}

func (params *InstallationParams) GetServerHelmValuesParams() *helm.ServerHelmValuesParams {
	dataContainerPaths := []string{}
	for _, path := range params.DatasetVolumes {
		dataContainerPaths = append(dataContainerPaths, strings.Split(path, ":")[1])
	}

	tlsParams := params.TLSParams.GetTLSHelmParams()

	datadogEnvs := params.GetDatadogEnvs()

	return &helm.ServerHelmValuesParams{
		Gpu:                   params.IsUseGpu(),
		LocalDataDirectories:  dataContainerPaths,
		DisableDatadogMetrics: params.DisableMetrics,
		Domain:                params.Domain,
		BasePath:              params.CalcBestPath(),
		Url:                   params.CalcUrl(),
		ProxyUrl:              params.ProxyUrl,
		Tls:                   *tlsParams,
		DatadogEnv:            datadogEnvs,
	}
}

func (params *InstallationParams) GetDatadogEnvs() map[string]string {
	data := map[string]string{}
	proxyHTTP, httpExists := os.LookupEnv("HTTP_PROXY")
	proxyHTTPS, httpsExists := os.LookupEnv("HTTPS_PROXY")
	proxyNoProxy, noProxyExists := os.LookupEnv("NO_PROXY")

	if httpExists {
		data["DD_PROXY_HTTP"] = proxyHTTP
	}
	if httpsExists {
		data["DD_PROXY_HTTPS"] = proxyHTTPS
	}
	if noProxyExists {
		data["DD_PROXY_NO_PROXY"] = proxyNoProxy
	}

	return data
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
	standaloneDir := local.GetServerDataDir()
	volumes := []string{
		fmt.Sprintf("%v:%v", standaloneDir, local.DEFAULT_DATA_DIR),
	}
	volumes = append(volumes, params.DatasetVolumes...)

	useGpu := params.IsUseGpu()
	var tlsPort *uint
	if params.TLSParams.Enabled {
		tlsPort = &params.TLSParams.Port
	}

	return &k3d.CreateK3sClusterParams{
		WithGpu:  useGpu,
		Port:     params.Port,
		Volumes:  volumes,
		CpuLimit: params.CpuLimit,
		TLSPort:  tlsPort,
	}
}

func (params *InstallationParams) GetCreateRegistryParams() *k3d.CreateRegistryParams {
	volumes := []string{
		fmt.Sprintf("%v:%v", path.Join(local.GetServerDataDir(), "registry"), "/var/lib/registry"),
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
	backwardCompatibility_datasetDirectory(params)

	return params, nil
}

func backwardCompatibility_datasetDirectory(params *InstallationParams) {
	if params.DatasetDirectory_DEPRECATED != "" && len(params.DatasetVolumes) == 0 {
		params.DatasetVolumes = []string{params.DatasetDirectory_DEPRECATED}
	}
	params.DatasetDirectory_DEPRECATED = ""
}

func LoadInstallationParams(paramsBytes []byte) (*InstallationParams, error) {
	params := &InstallationParams{}
	err := yaml.Unmarshal(paramsBytes, params)
	if err != nil {
		return nil, err
	}
	return params, nil
}
