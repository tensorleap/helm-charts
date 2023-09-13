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
const DefaultRegistryPort = 5699
const DefaultClusterPort = 4589

type InstallationParams struct {
	Version          string `json:"version"`
	UseGpu           bool   `json:"useGpu"`
	ClusterPort      uint   `json:"clusterPort"`
	RegistryPort     uint   `json:"registryPort"`
	DisableMetrics   bool   `json:"disableMetrics"`
	DatasetDirectory string `json:"datasetDirectory"`
	FixK3dDns        bool   `json:"fixK3dDns"`
}

func InitInstallationParamsFromFlags(flags *InstallFlags) (*InstallationParams, error) {

	if err := InitUseGPU(&flags.UseGpu, flags.UseCpu); err != nil {
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

	if err := InitUseGPU(&installationParams.UseGpu, false); err != nil {
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

func InitUseGPU(useGpu *bool, useCpu bool) error {
	if *useGpu || useCpu {
		return nil
	}

	prompt := survey.Confirm{
		Message: "Do you want to use GPU?",
		Default: false,
	}

	err := survey.AskOne(&prompt, useGpu)
	return err
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

	return &k3d.CreateK3sClusterParams{
		WithGpu: params.UseGpu,
		Port:    params.ClusterPort,
		Volumes: volumes,
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
