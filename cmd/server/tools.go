package server

import (
	"context"
	"os"

	k3dcmd "github.com/k3d-io/k3d/v5/cmd"
	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/k3d"
	"github.com/tensorleap/helm-charts/pkg/log"
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

	cmd.AddCommand(k3dcmd.NewCmdK3d())
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

	// Resolve the kubeconfig lazily at run time — not at construction, which runs
	// for every `leap` invocation — so the Docker self-heal fallback only happens
	// when this command is actually used, and only when needed. Chain kubectl's own
	// persistent pre-run hook so its setup still runs.
	origPreRunE := kubectlCmd.PersistentPreRunE
	origPreRun := kubectlCmd.PersistentPreRun
	kubectlCmd.PersistentPreRun = nil
	kubectlCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		setDefaultKubeConfig(cmd.Context(), configFlags)
		if origPreRunE != nil {
			return origPreRunE(cmd, args)
		}
		if origPreRun != nil {
			origPreRun(cmd, args)
		}
		return nil
	}

	return kubectlCmd
}

// setDefaultKubeConfig points `leap server tools kubectl` at the standalone
// kubeconfig the installer keeps current, so it reaches the live cluster in ALL
// shell contexts — not only login shells where the installer's
// /etc/profile.d/tensorleap-kubeconfig.sh drop-in exports KUBECONFIG. A non-login
// shell (e.g. a CI `run:` step) leaves KUBECONFIG unset and would otherwise fall
// back to ~/.kube/config, which drifts stale because k3d picks a new random API
// port each install (see pkg/k3d.createClusterConfig).
//
// Precedence, highest first:
//  1. an explicit --kubeconfig flag (cobra has already parsed it into the pointer),
//  2. a user-set $KUBECONFIG,
//  3. the standalone kubeconfig (regenerated from the live cluster if missing),
//  4. kubectl's default resolution (~/.kube/config), used if the standalone copy
//     can't be resolved.
func setDefaultKubeConfig(ctx context.Context, configFlags *genericclioptions.ConfigFlags) {
	if configFlags.KubeConfig != nil && *configFlags.KubeConfig != "" {
		return // explicit --kubeconfig wins
	}
	if os.Getenv("KUBECONFIG") != "" {
		return // respect a user-set $KUBECONFIG
	}
	path, err := k3d.ResolveSharedKubeConfig(ctx)
	if err != nil {
		log.Warnf("Could not resolve standalone kubeconfig, using default resolution: %v", err)
		return
	}
	if path != "" {
		configFlags.KubeConfig = &path
	}
}

func init() {
	RootCommand.AddCommand(NewToolCmd())
}
