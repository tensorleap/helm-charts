package server

import (
	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server"
)

type ReinstallFlags struct {
	server.InstallationSourceFlags `json:",inline"`
	server.InstallFlags            `json:",inline"`
}

func NewReinstallCmd() *cobra.Command {
	flags := &ReinstallFlags{}
	cmd := &cobra.Command{
		Use:   "reinstall",
		Short: "Reinstall tensorleap",
		Long:  "Reinstall tensorleap",
		RunE: func(cmd *cobra.Command, args []string) error {
			isReinstalled, err := server.InitDataDirFunc(cmd.Context(), flags.DataDir)
			if err != nil {
				return err
			}
			return RunReinstallCmd(cmd, flags, isReinstalled)
		},
	}

	flags.SetFlags(cmd)
	return cmd
}

func RunReinstallCmd(cmd *cobra.Command, flags *ReinstallFlags, isAlreadyReinstalled bool) error {
	flags.BeforeRun(cmd)
	log.SetCommandName("reinstall")

	close, err := local.SetupInfra("reinstall")
	if err != nil {
		return err
	}
	defer close()

	mnf, isAirgap, infraChart, serverChart, err := server.InitInstallationProcess(&flags.InstallationSourceFlags)
	if err != nil {
		return err
	}

	if err := server.ValidateInstallerVersion(mnf.InstallerVersion); err != nil {
		return err
	}

	log.SendCloudReport("info", "Starting install", "Starting", &map[string]interface{}{"manifest": mnf})
	ctx := cmd.Context()
	installationParams, err := server.InitInstallationParamsFromFlags(&flags.InstallFlags)
	if err != nil {
		return err
	}
	if isAlreadyReinstalled {
		err = server.Install(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	} else {
		err = server.Reinstall(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	}
	if err != nil {
		return err
	}
	return nil

}

func (flags *ReinstallFlags) SetFlags(cmd *cobra.Command) {
	flags.InstallFlags.SetFlags(cmd)
	flags.InstallationSourceFlags.SetFlags(cmd)
}

func init() {
	RootCommand.AddCommand(NewReinstallCmd())
}
