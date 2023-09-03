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

func NewInstallCmd() *cobra.Command {
	var port uint
	var registryPort uint
	var useGpu bool
	var useCpu bool
	var datasetDirectory string
	var disableMetrics bool
	var fixK3dDns bool
	var tag string
	var airgapInstallationFilePath string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Installs tensorleap on the local machine, running in a docker container",
		Long:  `Installs tensorleap on the local machine, running in a docker container`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetCommandName("install")

			close, err := local.SetupInfra("install")
			if err != nil {
				return err
			}
			defer close()

			mnf, isAirgap, chart, clean, err := server.InitInstallationProcess(airgapInstallationFilePath, tag)
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

			if err := server.InitUseGPU(&useGpu, useCpu); err != nil {
				log.SendCloudReport("error", "Failed to initializing with gpu", "Failed",
					&map[string]interface{}{"useGpu": useCpu, "error": err.Error()})
				return err
			}

			if err := server.InitDatasetDirectory(&datasetDirectory); err != nil {
				log.SendCloudReport("error", "Failed initializing data volume directory", "Failed",
					&map[string]interface{}{"datasetDirectory": datasetDirectory, "error": err.Error()})
				return err
			}

			log.SendCloudReport("info", "Starting installation", "Starting",
				&map[string]interface{}{"params": map[string]interface{}{"port": port, "registryPort": registryPort,
					"useGpu": useGpu, "datasetDirectory": datasetDirectory, "args": args}})

			ctx := cmd.Context()

			registryVolumes := []string{
				fmt.Sprintf("%v:%v", path.Join(local.STANDALONE_DIR, "registry"), "/var/lib/registry"),
			}

			if fixK3dDns {
				k3d.FixDockerDns()
			}

			registry, err := k3d.CreateLocalRegistry(ctx, mnf.Images.Register, registryPort, registryVolumes)
			if err != nil {
				return err
			}

			registryPortStr, err := k3d.GetRegistryPort(ctx, registry)
			if err != nil {
				return err
			}

			imagesToCache, imageToCacheInTheBackground := server.CalcWhichImagesToCache(mnf, useGpu, isAirgap)

			k3d.CacheImagesInParallel(ctx, imagesToCache, registryPortStr, isAirgap)

			if err := k3d.CreateCluster(
				ctx,
				mnf,
				port,
				[]string{fmt.Sprintf("%v:%v", local.STANDALONE_DIR, local.STANDALONE_DIR), datasetDirectory},
				useGpu,
			); err != nil {
				return err
			}

			dataContainerPath := strings.Split(datasetDirectory, ":")[1]
			if err := server.InstallHelm(ctx, mnf.ServerHelmChart, chart, useGpu, dataContainerPath, disableMetrics); err != nil {
				return err
			}
			if len(imageToCacheInTheBackground) > 0 {
				if err := k3d.CacheImageInTheBackground(ctx, imageToCacheInTheBackground); err != nil {
					return err
				}
			}
			baseLink := fmt.Sprintf("http://127.0.0.1:%v", port)

			// todo call hook on install finished
			// apiLink := fmt.Sprintf("%s/api/v2", baseLink)
			// if err := auth.Login("", apiLink); err != nil {
			// 	return err
			// }

			log.SendCloudReport("info", "Successfully completed installation", "Success", nil)
			log.Info("Successfully completed installation")
			_ = local.OpenLink(baseLink)
			log.Infof("You can now access Tensorleap at %s", baseLink)
			return nil
		},
	}

	cmd.Flags().UintVarP(&port, "port", "p", 4589, "Port to be used for tensorleap installation")
	cmd.Flags().UintVar(&registryPort, "registry-port", 5699, "Port to be used for docker registry")
	cmd.Flags().BoolVar(&useGpu, "gpu", false, "Enable GPU usage for training and evaluating")
	cmd.Flags().BoolVar(&useCpu, "cpu", false, "Use CPU for training and evaluating")
	cmd.Flags().StringVar(&datasetDirectory, "dataset-dir", "", "Dataset directory maps the user's local directory to the container's directory, enabling access to code integration for training and evaluation")
	cmd.Flags().BoolVar(&disableMetrics, "disable-metrics", false, "Disable metrics collection")
	cmd.Flags().BoolVar(&fixK3dDns, "fix-dns", false, "Fix DNS issue with docker, in case you are having issue with internet connection in the container")
	cmd.Flags().StringVarP(&tag, "tag", "t", "", "Tag to be used for tensorleap installation, default is latest")
	cmd.Flags().StringVar(&airgapInstallationFilePath, "airgap", "", "Installation file path for air-gap installation")
	return cmd
}

func init() {
	RootCommand.AddCommand(NewInstallCmd())
}
