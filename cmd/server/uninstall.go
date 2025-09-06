package server

import (
	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server"
)

func NewUninstallCmd() *cobra.Command {
	var purge bool
	var cleanup bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove local Tensorleap installation",
		Long:  `Remove local Tensorleap installation`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := server.InitDataDirFunc(cmd.Context(), "")
			if err != nil {
				return err
			}
			return RunUninstallCmd(cmd, purge, cleanup)
		},
	}

	cmd.Flags().BoolVar(&purge, "purge", false, "Remove all data and cached files")
	cmd.Flags().BoolVar(&cleanup, "cleanup", false, "Cleanup cached data")

	return cmd
}

func RunUninstallCmd(cmd *cobra.Command, purge bool, cleanup bool) error {
	log.SetCommandName("uninstall")
	log.SendCloudReport("info", "Starting uninstall", "Starting", &map[string]interface{}{"purge": purge})
	close, err := local.SetupInfra("uninstall")
	if err != nil {
		return err
	}
	defer close()

	ctx := cmd.Context()
	err = server.Uninstall(ctx, purge, cleanup)
	if err != nil {
		log.SendCloudReport("error", "Failed to uninstall", "Failed", &map[string]interface{}{"error": err.Error()})
		return err
	}

	log.SendCloudReport("info", "Successfully completed uninstall", "Success", nil)
	return nil
}

func init() {
	RootCommand.AddCommand(NewUninstallCmd())
}
