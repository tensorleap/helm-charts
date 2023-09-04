package server

import (
	k3d "github.com/k3d-io/k3d/v5/cmd"
	"github.com/spf13/cobra"
	kubectl "k8s.io/kubectl/pkg/cmd"
)

func NewToolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "3rd party tools to help manage local environment",
		Long:  `3rd party tools to help manage local environment`,
	}

	cmd.AddCommand(k3d.NewCmdK3d())
	cmd.AddCommand(kubectl.NewDefaultKubectlCommand())

	return cmd
}

func init() {
	RootCommand.AddCommand(NewToolCmd())
}
