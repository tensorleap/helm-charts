package server

import (
	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server"
)

type UninstallFlags struct {
	Purge     bool
	Cleanup   bool
	ClearData bool
}

func (flags *UninstallFlags) AddToCommand(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&flags.Purge, "purge", false, "Remove all data and cached files")
	cmd.Flags().BoolVar(&flags.Cleanup, "cleanup", false, "Cleanup cached data (registry, containerd, helm-cache)")
	cmd.Flags().BoolVar(&flags.ClearData, "clear-data", false, "Clear application data (storage, manifests) but keep cache")
}

func NewUninstallCmd() *cobra.Command {
	flags := &UninstallFlags{}
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove local Tensorleap installation",
		Long:  `Remove local Tensorleap installation`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := server.InitDataDirFunc(cmd.Context(), "")
			if err != nil {
				return err
			}
			return RunUninstallCmd(cmd, flags)
		},
	}

	flags.AddToCommand(cmd)

	return cmd
}

func RunUninstallCmd(cmd *cobra.Command, flags *UninstallFlags) error {
	log.SetCommandName("uninstall")
	log.SendCloudReport("info", "Starting uninstall", "Starting", &map[string]interface{}{
		"purge":     flags.Purge,
		"cleanup":   flags.Cleanup,
		"clearData": flags.ClearData,
	})
	close, err := local.SetupInfra("uninstall")
	if err != nil {
		return err
	}
	defer close()

	ctx := cmd.Context()
	err = server.Uninstall(ctx, flags.Purge, flags.Cleanup, flags.ClearData)
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
