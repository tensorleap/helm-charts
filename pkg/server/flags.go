package server

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/log"
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
	cmd.Flags().UintVar(&flags.Port, "tls-port", DefaultHttpsPort, "Port to be used for TLS")
}

func (flags *TLSFlags) IsEnabled() bool {
	return flags.CertPath != "" && flags.KeyPath != ""
}

type InstallFlags struct {
	Port                    uint     `json:"port"`
	RegistryPort            uint     `json:"registryPort"`
	GpuDevices              string   `json:"gpuDevices,omitempty"`
	Gpus                    uint     `json:"gpus,omitempty"`
	UseCpu                  bool     `json:",omitempty"`
	DatasetVolumes          []string `json:"datasetVolumes"`
	DisableMetrics          bool     `json:"disableMetrics"`
	Domain                  string   `json:"domain"`
	DataDir                 string   `json:"dataDir"`
	ProxyUrl                string   `json:"ProxyUrl"`
	CpuLimit                string   `json:"cpuLimit,omitempty"`
	DisableAuth             *bool    `json:"disableAuth,omitempty"`
	ClearInstallationImages *bool    `json:"removeInstallationImages,omitempty"`
	ImageCachingMethod      string   `json:"imageCachingMethod,omitempty"`
	TLSFlags
}

func (flags *InstallFlags) SetFlags(cmd *cobra.Command) {
	cmd.Flags().UintVarP(&flags.Port, "port", "p", DefaultHttpPort, "Port to be used for http server")
	cmd.Flags().UintVar(&flags.RegistryPort, "registry-port", DefaultRegistryPort, "Port to be used for docker registry")
	cmd.Flags().StringVar(&flags.GpuDevices, "gpu-devices", "", "GPU devices to be used (e.g. 1 or 0,1,2 or all)")
	cmd.Flags().UintVar(&flags.Gpus, "gpus", 0, "Number of GPUs to be used")
	cmd.Flags().BoolVar(&flags.UseCpu, "cpu", false, "Use CPU for training and evaluating")
	cmd.Flags().StringArrayVarP(&flags.DatasetVolumes, "dataset-volume", "v", []string{}, "Dataset volume maps the user's local directory to the container's directory, enabling access to code integration for training and evaluation")
	cmd.Flags().BoolVar(&flags.DisableMetrics, "disable-metrics", false, "Disable metrics collection")
	cmd.Flags().StringVar(&flags.Domain, "domain", "localhost", "Domain to be used for tensorleap server")
	cmd.Flags().StringVar(&flags.ProxyUrl, "proxy-url", "", "Proxy URL to be used for tensorleap server")
	cmd.Flags().StringVarP(&flags.DataDir, "data-dir", "d", "", "Directory to store tensorleap data, by default using /var/lib/tensorleap/standalone or previous data directory")
	cmd.Flags().StringVar(&flags.CpuLimit, "cpu-limit", "", "Limit the CPU resources for the k3d cluster (e.g. 2 for 2 cores)")
	setNilBoolFlag(cmd, &flags.DisableAuth, "disable-auth", "Disable authentication for the tensorleap server")
	setNilBoolFlag(cmd, &flags.ClearInstallationImages, "clear-images", "Clear installation images after installation")
	cmd.Flags().StringVar(&flags.ImageCachingMethod, "image-caching", "", "Image caching method: docker-volume (Docker volume to containerd), local-volume (volume from local computer to containerd), or registry (caching by registry). Default is detected based on environment: Linux uses local-volume, macOS uses docker-volume, airgap uses registry")

	deprecatedFlag_datasetDir(cmd)

	flags.TLSFlags.SetFlags(cmd)
}

func (flags *InstallFlags) BeforeRun(cmd *cobra.Command) {
	if !cmd.Flags().Changed("clear-images") {
		flags.ClearInstallationImages = nil
	}
	if !cmd.Flags().Changed("disable-auth") {
		flags.DisableAuth = nil
	}
}

func setNilBoolFlag(cmd *cobra.Command, ref **bool, flagName string, message string) {
	*ref = new(bool)
	cmd.Flags().BoolVar(*ref, flagName, false, message)
}

func deprecatedFlag_datasetDir(cmd *cobra.Command) {
	cmd.Flags().String("dataset-dir", "", "DEPRECATED: Use --dataset-volume or -v instead.")
	_ = cmd.Flags().MarkHidden("dataset-dir")

	args := os.Args[1:]
	isDatasetDirChanged := strings.Contains(strings.Join(args, " "), "--dataset-dir")

	if isDatasetDirChanged {
		log.Fatalf("Error: --dataset-dir is deprecated. Please use --dataset-volume or -v instead.")
	}
}
