package server

import (
	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
)

func NewStopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stop",
		Aliases: []string{"down"},
		Short:   "Stop Tensorleap server",
		Long:    `Stop Tensorleap server`,
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetCommandName("stop")

			close, err := local.SetupInfra("stop")
			if err != nil {
				return err
			}
			defer close()

			err = k3d.StopCluster(cmd.Context())
			if err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}

func init() {
	RootCommand.AddCommand(NewStopCmd())
}
