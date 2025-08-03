package server

import (
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

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: UpgradeCmdDescription,
		Long:  UpgradeCmdDescription,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := server.InitDataDirFunc(cmd.Context(), "")
			if err != nil {
				return err
			}

			return RunUpgradeCmd(cmd, flags)
		},
	}

	flags.SetFlags(cmd)
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

	mnf, isAirgap, infraChart, serverChart, err := server.InitInstallationProcess(&flags.InstallationSourceFlags, nil)

	if err := server.ValidateInstallerVersion(mnf.InstallerVersion); err != nil {
		return err
	}

	log.SendCloudReport("info", "Starting upgrade", "Starting", &map[string]interface{}{"manifest": mnf})

	if err != nil {
		return err
	}

	installationParams, found, err := server.InitInstallationParamsFromPreviousOrAsk()
	if err != nil {
		return err
	}

	reinstall := func() error {
		return server.SafetyReinstall(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	}

	if !found {
		return reinstall()
	}

	err = server.Install(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	if err == server.ErrReinstallRequired {
		return reinstall()
	} else if err != nil {
		return err
	}

	log.SendCloudReport("info", "Successfully completed upgrade", "Success", nil)
	return nil
}

func init() {
	RootCommand.AddCommand(NewUpgradeCmd())
}
