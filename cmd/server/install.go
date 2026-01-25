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
			_, err = RunInstallCmd(cmd, flags)
			return err
		},
	}

	flags.SetFlags(cmd)
	cmd.Flags().BoolVarP(&nonInteractive, "yes", "y", false, "Run in non-interactive mode (skip prompts)")

	return cmd
}

// RunInstallCmd runs the install command and returns the installation result.
// Wrapper CLIs can use this to get server info for post-install actions like login.
func RunInstallCmd(cmd *cobra.Command, flags *InstallFlags) (*server.InstallationResult, error) {
	flags.BeforeRun(cmd)
	ctx := cmd.Context()
	log.SetCommandName("install")
	close, err := local.SetupInfra("install")
	if err != nil {
		return nil, err
	}
	defer close()

	previousMnf, err := manifest.Load(local.GetInstallationManifestPath())
	if err != nil && err != manifest.ErrManifestNotFound {
		return nil, err
	}

	isAirgap := flags.IsAirGap()

	installationParams, err := server.InitInstallationParamsFromFlags(&flags.InstallFlags, isAirgap)
	if err != nil {
		return nil, err
	}

	// Pre-check reinstall before loading heavy assets (airgap images / chart downloads)
	mnf, _, err := server.LoadManifestOnly(&flags.InstallationSourceFlags, previousMnf)
	if err != nil {
		return nil, err
	}
	previousParams, _ := server.LoadInstallationParamsFromPrevious() // best effort
	needsReinstall, err := server.EnsureReinstallConsent(ctx, mnf, previousMnf, installationParams, previousParams)
	if err != nil {
		return nil, err
	}

	mnf, isAirgap, infraChart, serverChart, err := server.InitInstallationProcess(&flags.InstallationSourceFlags, previousMnf)
	if err != nil {
		return nil, err
	}

	if err := server.ValidateInstallerVersion(mnf.InstallerVersion); err != nil {
		return nil, err
	}

	log.SendCloudReport("info", "Starting install", "Starting", &map[string]interface{}{"manifest": mnf})

	err = k3d.CheckDockerRequirements(mnf.Images.CheckDockerRequirement, isAirgap)
	if err != nil {
		log.SendCloudReport("error", "Docker requirements not met", "Failed",
			&map[string]interface{}{"error": err.Error()})
		return nil, err
	}

	log.SendCloudReport("info", "Starting installation", "Starting",
		&map[string]interface{}{"flags": flags, "manifest": mnf})

	var result *server.InstallationResult
	if needsReinstall {
		result, err = server.Reinstall(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
		if err != nil {
			log.SendCloudReport("error", "Failed reinstall", "Failed",
				&map[string]interface{}{"error": err.Error()})
			return nil, err
		}
	} else {
		result, err = server.Install(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
		if err != nil {
			log.SendCloudReport("error", "Failed installation", "Failed",
				&map[string]interface{}{"error": err.Error()})
			return nil, err
		}
	}

	log.SendCloudReport("info", "Successfully completed installation", "Success", nil)
	log.Info("Successfully completed installation")

	log.Infof("You can now access Tensorleap at %s", result.ServerURL)

	return result, nil
}

func (flags *InstallFlags) SetFlags(cmd *cobra.Command) {
	flags.InstallFlags.SetFlags(cmd)
	flags.InstallationSourceFlags.SetFlags(cmd)
}

func init() {
	RootCommand.AddCommand(NewInstallCmd())
}
