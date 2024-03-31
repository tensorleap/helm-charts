package server

import (
	"fmt"

	"github.com/spf13/cobra"
)

type InstallationSourceFlags struct {
	Tag                        string `json:"tag,omitempty"`
	AirGapInstallationFilePath string `json:",omitempty"`
	Local                      bool   `json:"local,omitempty"`
}

func (flags *InstallationSourceFlags) SetFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&flags.Tag, "tag", "t", "", "Tag to be used for tensorleap installation, default is latest")
	cmd.Flags().StringVar(&flags.AirGapInstallationFilePath, "airgap", "", "Installation file path for air-gap installation")
	cmd.Flags().BoolVar(&flags.Local, "local", false, "Install tensorleap from local helm charts")
}

type InstallFlags struct {
	Port             uint   `json:"port"`
	RegistryPort     uint   `json:"registryPort"`
	GpuDevices       string `json:"gpuDevices,omitempty"`
	Gpus             uint   `json:"gpus,omitempty"`
	UseCpu           bool   `json:",omitempty"`
	DatasetDirectory string `json:"datasetDirectory"`
	DisableMetrics   bool   `json:"disableMetrics"`
	FixK3dDns        bool   `json:"fixK3dDns"`
	EndpointUrl      string `json:"endpointUrl,omitempty"`
}

func (flags *InstallFlags) SetFlags(cmd *cobra.Command) {
	cmd.Flags().UintVarP(&flags.Port, "port", "p", defaultHttpPort, "Port to be used for http server")
	cmd.Flags().UintVar(&flags.RegistryPort, "registry-port", defaultRegistryPort, "Port to be used for docker registry")
	cmd.Flags().StringVar(&flags.GpuDevices, "gpu-devices", "", "GPU devices to be used (e.g. 1 or 0,1,2 or all)")
	cmd.Flags().UintVar(&flags.Gpus, "gpus", 0, "Number of GPUs to be used")
	cmd.Flags().BoolVar(&flags.UseCpu, "cpu", false, "Use CPU for training and evaluating")
	cmd.Flags().StringVar(&flags.DatasetDirectory, "dataset-dir", "", "Dataset directory maps the user's local directory to the container's directory, enabling access to code integration for training and evaluation")
	cmd.Flags().BoolVar(&flags.DisableMetrics, "disable-metrics", false, "Disable metrics collection")
	cmd.Flags().StringVar(&flags.EndpointUrl, "url", "", "Endpoint URL for tensorleap installation (by default use http://localhost:[port])")
	cmd.Flags().BoolVar(&flags.FixK3dDns, "fix-dns", false, "Fix DNS issue with docker, in case you are having issue with internet connection in the container")
}

func (flags *InstallFlags) CalcEndpointUrl() string {
	if flags.EndpointUrl == "" {
		endPoint := "http://localhost"
		if flags.Port == defaultHttpPort {
			return endPoint
		}
		return fmt.Sprintf("%s:%d", endPoint, flags.Port)
	}
	return flags.EndpointUrl
}
