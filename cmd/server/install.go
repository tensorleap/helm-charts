package server

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

const InstallCmdDescription = "Installs tensorleap on the local machine, running in a docker container"

type InstallFlags struct {
	server.InstallationSourceFlags `json:",inline"`
	server.InstallFlags            `json:",inline"`
}

func NewInstallCmd() *cobra.Command {
	flags := &InstallFlags{}
	var nonInteractive bool

	cmd := &cobra.Command{
		Use:   "install",
		Short: InstallCmdDescription,
		Long:  InstallCmdDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Set non-interactive mode via environment variable
			// This signals to use defaults and skip prompts
			if nonInteractive {
				os.Setenv("TL_USE_DEFAULT_OPTION", "true")
			}

			_, err := server.InitDataDirFunc(cmd.Context(), flags.DataDir)
			if err != nil {
				return err
			}
			return RunInstallCmd(cmd, flags)
		},
	}

	flags.SetFlags(cmd)
	cmd.Flags().BoolVarP(&nonInteractive, "yes", "y", false, "Run in non-interactive mode (skip prompts)")

	return cmd
}

func RunInstallCmd(cmd *cobra.Command, flags *InstallFlags) error {
	flags.BeforeRun(cmd)
	ctx := cmd.Context()
	log.SetCommandName("install")
	close, err := local.SetupInfra("install")
	if err != nil {
		return err
	}
	defer close()

	previousMnf, err := manifest.Load(local.GetInstallationManifestPath())
	if err != nil && err != manifest.ErrManifestNotFound {
		return err
	}

	mnf, isAirgap, infraChart, serverChart, err := server.InitInstallationProcess(&flags.InstallationSourceFlags, previousMnf)
	if err != nil {
		return err
	}

	if err := server.ValidateInstallerVersion(mnf.InstallerVersion); err != nil {
		return err
	}

	log.SendCloudReport("info", "Starting install", "Starting", &map[string]interface{}{"manifest": mnf})

	err = k3d.CheckDockerRequirements(mnf.Images.CheckDockerRequirement, isAirgap)
	if err != nil {
		log.SendCloudReport("error", "Docker requirements not met", "Failed",
			&map[string]interface{}{"error": err.Error()})
		return err
	}

	installationParams, err := server.InitInstallationParamsFromFlags(&flags.InstallFlags, isAirgap)
	if err != nil {
		return err
	}

	log.SendCloudReport("info", "Starting installation", "Starting",
		&map[string]interface{}{"flags": flags, "manifest": mnf})

	err = server.Install(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	if err != nil {
		log.SendCloudReport("error", "Failed installation", "Failed",
			&map[string]interface{}{"error": err.Error()})
		return err
	}

	baseLink := installationParams.CalcUrl()

	log.SendCloudReport("info", "Successfully completed installation", "Success", nil)
	log.Info("Successfully completed installation")

	_ = local.OpenLink(baseLink)
	log.Infof("You can now access Tensorleap at %s", baseLink)

	return nil
}

func (flags *InstallFlags) SetFlags(cmd *cobra.Command) {
	flags.InstallFlags.SetFlags(cmd)
	flags.InstallationSourceFlags.SetFlags(cmd)
}

func init() {
	RootCommand.AddCommand(NewInstallCmd())
}
