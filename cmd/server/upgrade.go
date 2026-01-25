package server

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server"
)

const UpgradeCmdDescription = "Upgrade an existing local tensorleap installation to the latest version"

type UpgradeFlags struct {
	server.InstallationSourceFlags `json:",inline"`
}

func NewUpgradeCmd() *cobra.Command {
	flags := &UpgradeFlags{}
	var nonInteractive bool

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: UpgradeCmdDescription,
		Long:  UpgradeCmdDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Set non-interactive mode via environment variable
			// This signals to use defaults and skip prompts
			if nonInteractive {
				os.Setenv("TL_USE_DEFAULT_OPTION", "true")
			}

			_, err := server.InitDataDirFunc(cmd.Context(), "")
			if err != nil {
				return err
			}

			_, err = RunUpgradeCmd(cmd, flags)
			return err
		},
	}

	flags.SetFlags(cmd)
	cmd.Flags().BoolVarP(&nonInteractive, "yes", "y", false, "Run in non-interactive mode (skip prompts)")
	return cmd
}

func (flags *UpgradeFlags) SetFlags(cmd *cobra.Command) {
	flags.InstallationSourceFlags.SetFlags(cmd)
}

// RunUpgradeCmd runs the upgrade command and returns the installation result.
// Wrapper CLIs can use this to get server info for post-install actions like login.
func RunUpgradeCmd(cmd *cobra.Command, flags *UpgradeFlags) (*server.InstallationResult, error) {
	log.SetCommandName("upgrade")
	close, err := local.SetupInfra("upgrade")
	if err != nil {
		return nil, err
	}
	defer close()
	ctx := cmd.Context()

	if err := server.ValidateStandaloneDir(); err != nil {
		return nil, err
	}

	installationParams, found, err := server.InitInstallationParamsFromPreviousOrAsk()
	if err != nil {
		return nil, err
	}

	mnf, isAirgap, infraChart, serverChart, err := server.InitInstallationProcess(&flags.InstallationSourceFlags, nil)
	if err != nil {
		return nil, err
	}

	if err := server.ValidateInstallerVersion(mnf.InstallerVersion); err != nil {
		return nil, err
	}

	log.SendCloudReport("info", "Starting upgrade", "Starting", &map[string]interface{}{"manifest": mnf})

	var result *server.InstallationResult

	reinstall := func() (*server.InstallationResult, error) {
		return server.Reinstall(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	}

	if !found {
		result, err = reinstall()
		if err != nil {
			return nil, err
		}
		log.SendCloudReport("info", "Successfully completed upgrade", "Success", nil)
		return result, nil
	}

	// If params were loaded (found), check once up front if reinstall is needed
	needsReinstall, err := server.EnsureReinstallConsent(ctx, mnf, nil, installationParams, nil)
	if err != nil {
		return nil, err
	}
	if needsReinstall {
		log.SendCloudReport("info", "Reinstall required during upgrade", "Running", nil)
		result, err = reinstall()
		if err != nil {
			return nil, err
		}
		log.SendCloudReport("info", "Successfully completed upgrade", "Success", nil)
		return result, nil
	}

	result, err = server.Install(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	if err != nil {
		return nil, err
	}

	log.SendCloudReport("info", "Successfully completed upgrade", "Success", nil)
	return result, nil
}

func init() {
	RootCommand.AddCommand(NewUpgradeCmd())
}
