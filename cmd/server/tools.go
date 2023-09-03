package server

import (
	k3d "github.com/k3d-io/k3d/v5/cmd"
	"github.com/spf13/cobra"
	kubectl "k8s.io/kubectl/pkg/cmd"
)

var ToolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "3rd party tools to help manage local environment",
	Long:  `3rd party tools to help manage local environment`,
}

func init() {
	ToolsCmd.AddCommand(k3d.NewCmdK3d())
	ToolsCmd.AddCommand(kubectl.NewDefaultKubectlCommand())
	RootCommand.AddCommand(ToolsCmd)
}
