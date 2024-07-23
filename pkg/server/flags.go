package server

import (
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

type TLSFlags struct {
	CertPath  string `json:"certPath,omitempty"`
	KeyPath   string `json:"keyPath,omitempty"`
	ChainPath string `json:"chainPath,omitempty"`
	Port      uint   `json:"port,omitempty"`
}

func (flags *TLSFlags) SetFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&flags.CertPath, "cert", "c", "", "Path to the TLS certificate file")
	cmd.Flags().StringVarP(&flags.KeyPath, "key", "k", "", "Path to the TLS key file")
	cmd.Flags().StringVar(&flags.ChainPath, "chain", "", "Path to the TLS chain file (optional)")
	cmd.Flags().UintVar(&flags.Port, "tls-port", defaultHttpsPort, "Port to be used for TLS")
}

func (flags *TLSFlags) IsEnabled() bool {
	return flags.CertPath != "" && flags.KeyPath != ""
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
	Domain           string `json:"domain"`
	DataDir          string `json:"dataDir"`
	BasePath         string `json:"basePath"`
	TLSFlags
}

func (flags *InstallFlags) SetFlags(cmd *cobra.Command) {
	cmd.Flags().UintVarP(&flags.Port, "port", "p", defaultHttpPort, "Port to be used for http server")
	cmd.Flags().UintVar(&flags.RegistryPort, "registry-port", defaultRegistryPort, "Port to be used for docker registry")
	cmd.Flags().StringVar(&flags.GpuDevices, "gpu-devices", "", "GPU devices to be used (e.g. 1 or 0,1,2 or all)")
	cmd.Flags().UintVar(&flags.Gpus, "gpus", 0, "Number of GPUs to be used")
	cmd.Flags().BoolVar(&flags.UseCpu, "cpu", false, "Use CPU for training and evaluating")
	cmd.Flags().StringVar(&flags.DatasetDirectory, "dataset-dir", "", "Dataset directory maps the user's local directory to the container's directory, enabling access to code integration for training and evaluation")
	cmd.Flags().BoolVar(&flags.DisableMetrics, "disable-metrics", false, "Disable metrics collection")
	cmd.Flags().StringVar(&flags.Domain, "domain", "localhost", "Domain to be used for tensorleap server")
	cmd.Flags().StringVar(&flags.BasePath, "base-path", "", "Base path to be used for tensorleap server")
	cmd.Flags().BoolVar(&flags.FixK3dDns, "fix-dns", false, "Fix DNS issue with docker, in case you are having issue with internet connection in the container")
	cmd.Flags().StringVarP(&flags.DataDir, "data-dir", "d", "", "Directory to store tensorleap data, by default using /var/lib/tensorleap/standalone or previous data directory")
	flags.TLSFlags.SetFlags(cmd)
}
