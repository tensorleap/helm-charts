package server

import (
	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server"
)

func NewRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "run",
		Aliases: []string{"up", "start"},
		Short:   "Run Tensorleap server",
		Long:    `Run Tensorleap server`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetCommandName("run")

			_, err := server.InitDataDirFunc(cmd.Context(), "")
			if err != nil {
				return err
			}

			close, err := local.SetupInfra("run")
			if err != nil {
				return err
			}
			defer close()

			err = k3d.RunCluster(cmd.Context())
			if err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}

func init() {
	RootCommand.AddCommand(NewRunCmd())
}
