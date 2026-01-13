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

			return RunUpgradeCmd(cmd, flags)
		},
	}

	flags.SetFlags(cmd)
	cmd.Flags().BoolVarP(&nonInteractive, "yes", "y", false, "Run in non-interactive mode (skip prompts)")
	return cmd
}

func (flags *UpgradeFlags) SetFlags(cmd *cobra.Command) {
	flags.InstallationSourceFlags.SetFlags(cmd)
}

func RunUpgradeCmd(cmd *cobra.Command, flags *UpgradeFlags) error {
	log.SetCommandName("upgrade")
	close, err := local.SetupInfra("upgrade")
	if err != nil {
		return err
	}
	defer close()
	ctx := cmd.Context()

	if err := server.ValidateStandaloneDir(); err != nil {
		return err
	}

	installationParams, found, err := server.InitInstallationParamsFromPreviousOrAsk()
	if err != nil {
		return err
	}

	mnf, isAirgap, infraChart, serverChart, err := server.InitInstallationProcess(&flags.InstallationSourceFlags, nil)
	if err != nil {
		return err
	}

	if err := server.ValidateInstallerVersion(mnf.InstallerVersion); err != nil {
		return err
	}

	log.SendCloudReport("info", "Starting upgrade", "Starting", &map[string]interface{}{"manifest": mnf})

	reinstall := func() error {
		return server.Reinstall(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	}

	if !found {
		return reinstall()
	}

	// If params were loaded (found), check once up front if reinstall is needed
	if found {
		needsReinstall, err := server.EnsureReinstallConsent(ctx, mnf, nil, installationParams, nil)
		if err != nil {
			return err
		}
		if needsReinstall {
			log.SendCloudReport("info", "Reinstall required during upgrade", "Running", nil)
			return reinstall()
		}
	}

	err = server.Install(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	if err != nil {
		return err
	}

	log.SendCloudReport("info", "Successfully completed upgrade", "Success", nil)
	return nil
}

func init() {
	RootCommand.AddCommand(NewUpgradeCmd())
}
