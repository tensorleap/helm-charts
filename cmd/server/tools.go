package server

import (
	"os"

	k3d "github.com/k3d-io/k3d/v5/cmd"
	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/server"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	kubectl "k8s.io/kubectl/pkg/cmd"
)

func NewToolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "3rd party tools to help manage local environment",
		Long:  `3rd party tools to help manage local environment`,
	}

	cmd.AddCommand(k3d.NewCmdK3d())
	cmd.AddCommand(newTensorleapKubectlCommand())

	return cmd
}

func newTensorleapKubectlCommand() *cobra.Command {
	configFlags := genericclioptions.NewConfigFlags(true)
	kubeContext := server.KUBE_CONTEXT
	configFlags.Context = &kubeContext

	validPluginPrefixes := []string{"kubectl"}

	kubectlCmd := kubectl.NewKubectlCommand(kubectl.KubectlOptions{
		PluginHandler: kubectl.NewDefaultPluginHandler(validPluginPrefixes),
		Arguments:     os.Args,
		ConfigFlags:   configFlags,
		IOStreams:     genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr},
	})

	return kubectlCmd
}

func init() {
	RootCommand.AddCommand(NewToolCmd())
}
