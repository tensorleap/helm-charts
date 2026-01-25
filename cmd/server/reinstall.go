package server

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

type ReinstallFlags struct {
	server.InstallationSourceFlags `json:",inline"`
	server.InstallFlags            `json:",inline"`
}

func NewReinstallCmd() *cobra.Command {
	flags := &ReinstallFlags{}
	var nonInteractive bool

	cmd := &cobra.Command{
		Use:   "reinstall",
		Short: "Reinstall tensorleap",
		Long:  "Reinstall tensorleap",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Set non-interactive mode via environment variable
			// This signals to use defaults and skip prompts
			if nonInteractive {
				os.Setenv("TL_USE_DEFAULT_OPTION", "true")
			}

			isReinstalled, err := server.InitDataDirFunc(cmd.Context(), flags.DataDir)
			if err != nil {
				return err
			}
			_, err = RunReinstallCmd(cmd, flags, isReinstalled)
			return err
		},
	}

	flags.SetFlags(cmd)
	cmd.Flags().BoolVarP(&nonInteractive, "yes", "y", false, "Run in non-interactive mode (skip prompts)")
	return cmd
}

// RunReinstallCmd runs the reinstall command and returns the installation result.
// Wrapper CLIs can use this to get server info for post-install actions like login.
func RunReinstallCmd(cmd *cobra.Command, flags *ReinstallFlags, isAlreadyReinstalled bool) (*server.InstallationResult, error) {
	flags.BeforeRun(cmd)
	log.SetCommandName("reinstall")

	close, err := local.SetupInfra("reinstall")
	if err != nil {
		return nil, err
	}
	defer close()

	previousMnf, err := manifest.Load(local.GetInstallationManifestPath())
	if err != nil && err != manifest.ErrManifestNotFound {
		return nil, err
	}

	isAirgap := flags.IsAirGap()

	installationParams, err := server.InitInstallationParamsFromFlags(&flags.InstallFlags, isAirgap)
	if err != nil {
		return nil, err
	}

	mnf, isAirgap, infraChart, serverChart, err := server.InitInstallationProcess(&flags.InstallationSourceFlags, previousMnf)
	if err != nil {
		return nil, err
	}

	if err := server.ValidateInstallerVersion(mnf.InstallerVersion); err != nil {
		return nil, err
	}

	log.SendCloudReport("info", "Starting install", "Starting", &map[string]interface{}{"manifest": mnf})
	ctx := cmd.Context()

	var result *server.InstallationResult
	if isAlreadyReinstalled {
		result, err = server.Install(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	} else {
		result, err = server.Reinstall(ctx, mnf, isAirgap, installationParams, infraChart, serverChart)
	}
	if err != nil {
		return nil, err
	}

	log.SendCloudReport("info", "Successfully completed reinstall", "Success", nil)
	return result, nil
}

func (flags *ReinstallFlags) SetFlags(cmd *cobra.Command) {
	flags.InstallFlags.SetFlags(cmd)
	flags.InstallationSourceFlags.SetFlags(cmd)
}

func init() {
	RootCommand.AddCommand(NewReinstallCmd())
}
