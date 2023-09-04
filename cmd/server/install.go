package server

import (
	"fmt"
	"path"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server"
)

const InstallCmdDescription = "Installs tensorleap on the local machine, running in a docker container"

type InstallFlags struct {
	Port                       uint   `json:"port"`
	RegistryPort               uint   `json:"registryPort"`
	UseGpu                     bool   `json:"useGpu"`
	UseCpu                     bool   `json:",omitempty"`
	DatasetDirectory           string `json:"datasetDirectory"`
	DisableMetrics             bool   `json:"disableMetrics"`
	FixK3dDns                  bool   `json:"fixK3dDns"`
	Tag                        string `json:"tag"`
	AirGapInstallationFilePath string `json:",omitempty"`
}

func NewInstallCmd(onInstalled func(url string)) *cobra.Command {
	flags := &InstallFlags{}

	cmd := &cobra.Command{
		Use:   "install",
		Short: InstallCmdDescription,
		Long:  InstallCmdDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunInstallCmd(cmd, flags)
		},
	}

	SetInstallCmdFlags(cmd, flags)

	return cmd
}

func RunInstallCmd(cmd *cobra.Command, flags *InstallFlags) error {
	log.SetCommandName("install")

	close, err := local.SetupInfra("install")
	if err != nil {
		return err
	}
	defer close()

	mnf, isAirgap, chart, clean, err := server.InitInstallationProcess(flags.AirGapInstallationFilePath, flags.Tag)
	if err != nil {
		return err
	}
	defer clean()

	log.SendCloudReport("info", "Starting install", "Starting", &map[string]interface{}{"manifest": mnf})

	err = k3d.CheckDockerRequirements(mnf.Images.CheckDockerRequirement, isAirgap)
	if err != nil {
		log.SendCloudReport("error", "Docker requirements not met", "Failed",
			&map[string]interface{}{"error": err.Error()})
		return err
	}

	if err := server.InitUseGPU(&flags.UseGpu, flags.UseCpu); err != nil {
		log.SendCloudReport("error", "Failed to initializing with gpu", "Failed",
			&map[string]interface{}{"useGpu": flags.UseGpu, "error": err.Error()})
		return err
	}

	if err := server.InitDatasetDirectory(&flags.DatasetDirectory); err != nil {
		log.SendCloudReport("error", "Failed initializing data volume directory", "Failed",
			&map[string]interface{}{"datasetDirectory": flags.DatasetDirectory, "error": err.Error()})
		return err
	}

	log.SendCloudReport("info", "Starting installation", "Starting",
		&map[string]interface{}{"params": flags, "manifest": mnf})

	ctx := cmd.Context()

	registryVolumes := []string{
		fmt.Sprintf("%v:%v", path.Join(local.STANDALONE_DIR, "registry"), "/var/lib/registry"),
	}

	if flags.FixK3dDns {
		k3d.FixDockerDns()
	}

	registry, err := k3d.CreateLocalRegistry(ctx, mnf.Images.Register, flags.RegistryPort, registryVolumes)
	if err != nil {
		return err
	}

	registryPortStr, err := k3d.GetRegistryPort(ctx, registry)
	if err != nil {
		return err
	}

	imagesToCache, imageToCacheInTheBackground := server.CalcWhichImagesToCache(mnf, flags.UseGpu, isAirgap)

	k3d.CacheImagesInParallel(ctx, imagesToCache, registryPortStr, isAirgap)

	if err := k3d.CreateCluster(
		ctx,
		mnf,
		flags.Port,
		[]string{fmt.Sprintf("%v:%v", local.STANDALONE_DIR, local.STANDALONE_DIR), flags.DatasetDirectory},
		flags.UseGpu,
	); err != nil {
		return err
	}

	dataContainerPath := strings.Split(flags.DatasetDirectory, ":")[1]
	if err := server.InstallHelm(ctx, mnf.ServerHelmChart, chart, flags.UseGpu, dataContainerPath, flags.DisableMetrics); err != nil {
		return err
	}
	if len(imageToCacheInTheBackground) > 0 {
		if err := k3d.CacheImageInTheBackground(ctx, imageToCacheInTheBackground); err != nil {
			return err
		}
	}
	baseLink := fmt.Sprintf("http://127.0.0.1:%v", flags.Port)

	log.SendCloudReport("info", "Successfully completed installation", "Success", nil)
	log.Info("Successfully completed installation")
	_ = local.OpenLink(baseLink)
	log.Infof("You can now access Tensorleap at %s", baseLink)
	return nil
}

func SetInstallCmdFlags(cmd *cobra.Command, flags *InstallFlags) {
	cmd.Flags().UintVarP(&flags.Port, "port", "p", 4589, "Port to be used for tensorleap installation")
	cmd.Flags().UintVar(&flags.RegistryPort, "registry-port", 5699, "Port to be used for docker registry")
	cmd.Flags().BoolVar(&flags.UseGpu, "gpu", false, "Enable GPU usage for training and evaluating")
	cmd.Flags().BoolVar(&flags.UseCpu, "cpu", false, "Use CPU for training and evaluating")
	cmd.Flags().StringVar(&flags.DatasetDirectory, "dataset-dir", "", "Dataset directory maps the user's local directory to the container's directory, enabling access to code integration for training and evaluation")
	cmd.Flags().BoolVar(&flags.DisableMetrics, "disable-metrics", false, "Disable metrics collection")
	cmd.Flags().BoolVar(&flags.FixK3dDns, "fix-dns", false, "Fix DNS issue with docker, in case you are having issue with internet connection in the container")
	cmd.Flags().StringVarP(&flags.Tag, "tag", "t", "", "Tag to be used for tensorleap installation, default is latest")
	cmd.Flags().StringVar(&flags.AirGapInstallationFilePath, "airgap", "", "Installation file path for air-gap installation")
}

func init() {
	RootCommand.AddCommand(NewInstallCmd(nil))
}
