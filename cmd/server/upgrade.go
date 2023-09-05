package server

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/helm"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server"
)

const UpgradeCmdDescription = "Upgrade an existing local tensorleap installation to the latest version"

type UpgradeFlags struct {
	AirGapInstallationFilePath string `json:",omitempty"`
	Tag                        string `json:"tag"`
}

func NewUpgradeCmd() *cobra.Command {

	flags := &UpgradeFlags{}

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: UpgradeCmdDescription,
		Long:  UpgradeCmdDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunUpgradeCmd(cmd, flags)
		},
	}

	SetUpgradeCmdFlags(cmd, flags)

	return cmd
}

func SetUpgradeCmdFlags(cmd *cobra.Command, flags *UpgradeFlags) {
	cmd.Flags().StringVar(&flags.Tag, "tag", "", "Tag to use for tensorleap upgrade , default using latest")
	cmd.Flags().StringVar(&flags.AirGapInstallationFilePath, "airgap", "", "Installation file path for air-gap installation")
}

func RunUpgradeCmd(cmd *cobra.Command, flags *UpgradeFlags) error {
	log.SetCommandName("upgrade")
	close, err := local.SetupInfra("upgrade")
	if err != nil {
		return err
	}
	defer close()

	mnf, isAirgap, chart, clean, err := server.InitInstallationProcess(flags.AirGapInstallationFilePath, flags.Tag)
	if err != nil {
		return err
	}
	defer clean()

	if err := server.ValidateStandaloneDir(); err != nil {
		return err
	}
	ctx := cmd.Context()

	log.SendCloudReport("info", "Starting upgrade", "Starting", &map[string]interface{}{"manifest": mnf})

	err = server.ValidateClusterExists(ctx)
	if err != nil {
		return err
	}

	err = k3d.RunCluster(ctx)
	if err != nil {
		return err
	}

	cluster, err := k3d.GetCluster(ctx)
	if err != nil {
		return err
	}
	isGpuCluster := k3d.IsGpuCluster(cluster)

	kubeConfigPath, clean, err := k3d.CreateTmpClusterKubeConfig(ctx, cluster)
	if err != nil {
		return err
	}
	defer clean()

	helmConfig, err := helm.CreateHelmConfig(kubeConfigPath, server.KUBE_CONTEXT, server.KUBE_NAMESPACE)
	if err != nil {
		return err
	}

	isHelmReleaseExisted, err := helm.IsHelmReleaseExists(helmConfig, mnf.ServerHelmChart)
	if err != nil {
		return err
	} else if !isHelmReleaseExisted {
		return errors.New("not found helm release, Please make sure to install before upgrade")
	}

	imagesToCache, imageToCacheInTheBackground := server.CalcWhichImagesToCache(mnf, isGpuCluster, isAirgap)

	registryPort, err := k3d.GetLocalRegistryPort(ctx)
	if err != nil {
		return err
	}
	k3d.CacheImagesInParallel(ctx, imagesToCache, registryPort, isAirgap)

	if err := helm.UpgradeTensorleapChartVersion(helmConfig, mnf.ServerHelmChart, chart, nil); err != nil {
		return err
	}

	if len(imageToCacheInTheBackground) > 0 {
		if err = k3d.CacheImageInTheBackground(ctx, imageToCacheInTheBackground); err != nil {
			return err
		}
	}

	log.SendCloudReport("info", "Successfully completed upgrade", "Success", nil)
	return nil
}

func init() {
	RootCommand.AddCommand(NewUpgradeCmd())
}
