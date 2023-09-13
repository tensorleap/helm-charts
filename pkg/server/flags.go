package server

import "github.com/spf13/cobra"

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
	UseGpu           bool   `json:"useGpu"`
	UseCpu           bool   `json:",omitempty"`
	DatasetDirectory string `json:"datasetDirectory"`
	DisableMetrics   bool   `json:"disableMetrics"`
	FixK3dDns        bool   `json:"fixK3dDns"`
}

func (flags *InstallFlags) SetFlags(cmd *cobra.Command) {
	cmd.Flags().UintVarP(&flags.Port, "port", "p", DefaultClusterPort, "Port to be used for tensorleap installation")
	cmd.Flags().UintVar(&flags.RegistryPort, "registry-port", DefaultRegistryPort, "Port to be used for docker registry")
	cmd.Flags().BoolVar(&flags.UseGpu, "gpu", false, "Enable GPU usage for training and evaluating")
	cmd.Flags().BoolVar(&flags.UseCpu, "cpu", false, "Use CPU for training and evaluating")
	cmd.Flags().StringVar(&flags.DatasetDirectory, "dataset-dir", "", "Dataset directory maps the user's local directory to the container's directory, enabling access to code integration for training and evaluation")
	cmd.Flags().BoolVar(&flags.DisableMetrics, "disable-metrics", false, "Disable metrics collection")
	cmd.Flags().BoolVar(&flags.FixK3dDns, "fix-dns", false, "Fix DNS issue with docker, in case you are having issue with internet connection in the container")
}
