package server

import (
	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
)

func NewUninstallCmd() *cobra.Command {
	var purge bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove local Tensorleap installation",
		Long:  `Remove local Tensorleap installation`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetCommandName("uninstall")
			log.SendCloudReport("info", "Starting uninstall", "Starting", &map[string]interface{}{"args": args})
			close, err := local.SetupInfra("uninstall")
			if err != nil {
				return err
			}
			defer close()

			ctx := cmd.Context()
			err = k3d.UninstallCluster(ctx)
			if err != nil {
				return err
			}

			err = k3d.UninstallRegister()
			if err != nil {
				return err
			}

			if purge {
				err = local.PurgeData()
				if err != nil {
					return err
				}
			}

			log.SendCloudReport("info", "Successfully completed uninstall", "Success", nil)
			return nil
		},
	}

	cmd.Flags().BoolVar(&purge, "purge", false, "Remove all data and cached files")

	return cmd
}

func init() {
	RootCommand.AddCommand(NewUninstallCmd())
}
