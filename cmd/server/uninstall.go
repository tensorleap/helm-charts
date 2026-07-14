package server

import (
	"context"
	"fmt"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server"
)

type UninstallFlags struct {
	Purge     bool
	Cleanup   bool
	ClearData bool
	Custom    bool
}

func (flags *UninstallFlags) AddToCommand(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&flags.Purge, "purge", false, "Remove all data and cached files")
	cmd.Flags().BoolVar(&flags.Cleanup, "cleanup", false, "Cleanup cached data (registry, containerd, helm-cache)")
	cmd.Flags().BoolVar(&flags.ClearData, "clear-data", false, "Clear application data (storage, manifests) but keep cache")
	cmd.Flags().BoolVar(&flags.Custom, "custom", false, "Interactively choose exactly which extra data to delete on top of a normal uninstall")
}

// customUninstallOption pairs a user-facing menu label with the target it maps
// to. The order here is the order shown in the prompt.
type customUninstallOption struct {
	label  string
	target server.CustomTarget
}

var customUninstallOptions = []customUninstallOption{
	{"Projects, datasets & job metadata (MongoDB)", server.TargetMongo},
	{"Datasets, model weights & artifacts (MinIO)", server.TargetMinio},
	{"Analyses, insights & sample data (Elasticsearch)", server.TargetElastic},
	{"User accounts & login (Keycloak)", server.TargetKeycloak},
	{"All application data (everything above)", server.TargetAllAppData},
	{"Install config — versions & params (manifests)", server.TargetManifests},
	{"Install hostname", server.TargetHostname},
	{"Container image cache (containerd)", server.TargetImageCache},
	{"In-cluster registry data (Zot)", server.TargetRegistry},
	{"Helm chart cache", server.TargetHelmCache},
}

func NewUninstallCmd() *cobra.Command {
	flags := &UninstallFlags{}
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove local Tensorleap installation",
		Long:  `Remove local Tensorleap installation. Use --custom to interactively pick exactly which extra data to delete.`,
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

	if flags.Custom && (flags.Purge || flags.Cleanup || flags.ClearData) {
		return fmt.Errorf("--custom cannot be combined with --purge, --cleanup, or --clear-data")
	}

	log.SendCloudReport("info", "Starting uninstall", "Starting", &map[string]interface{}{
		"purge":     flags.Purge,
		"cleanup":   flags.Cleanup,
		"clearData": flags.ClearData,
		"custom":    flags.Custom,
	})
	close, err := local.SetupInfra("uninstall")
	if err != nil {
		return err
	}
	defer close()

	ctx := cmd.Context()

	if flags.Custom {
		return runCustomUninstall(ctx)
	}

	err = server.Uninstall(ctx, flags.Purge, flags.Cleanup, flags.ClearData)
	if err != nil {
		log.SendCloudReport("error", "Failed to uninstall", "Failed", &map[string]interface{}{"error": err.Error()})
		return err
	}

	log.SendCloudReport("info", "Successfully completed uninstall", "Success", nil)
	return nil
}

// runCustomUninstall prompts for the extra data to delete, confirms, then runs
// the uninstall. The cluster is always removed regardless of the selection.
func runCustomUninstall(ctx context.Context) error {
	targets, err := promptCustomUninstallTargets()
	if err != nil {
		return err
	}

	if len(targets) > 0 {
		confirmed, err := confirmCustomUninstall(targets)
		if err != nil {
			return err
		}
		if !confirmed {
			log.Println("Uninstall cancelled")
			return nil
		}
	} else {
		log.Println("No extra data selected — performing a normal uninstall (removing the cluster only)")
	}

	log.SendCloudReport("info", "Starting custom uninstall", "Running", &map[string]interface{}{"targets": targets})
	if err := server.UninstallCustom(ctx, targets); err != nil {
		log.SendCloudReport("error", "Failed to uninstall", "Failed", &map[string]interface{}{"error": err.Error()})
		return err
	}

	log.SendCloudReport("info", "Successfully completed uninstall", "Success", nil)
	return nil
}

func promptCustomUninstallTargets() ([]server.CustomTarget, error) {
	options := make([]string, len(customUninstallOptions))
	for i, o := range customUninstallOptions {
		options[i] = o.label
	}

	selectedLabels := []string{}
	prompt := &survey.MultiSelect{
		Message: "Select extra data to delete (space to toggle, enter to confirm). A normal uninstall removes the cluster regardless:",
		Options: options,
	}
	if err := survey.AskOne(prompt, &selectedLabels); err != nil {
		return nil, err
	}

	labelToTarget := make(map[string]server.CustomTarget, len(customUninstallOptions))
	for _, o := range customUninstallOptions {
		labelToTarget[o.label] = o.target
	}

	targets := make([]server.CustomTarget, 0, len(selectedLabels))
	for _, l := range selectedLabels {
		if t, ok := labelToTarget[l]; ok {
			targets = append(targets, t)
		}
	}
	return targets, nil
}

func confirmCustomUninstall(targets []server.CustomTarget) (bool, error) {
	selected := make(map[server.CustomTarget]bool, len(targets))
	for _, t := range targets {
		selected[t] = true
	}

	log.Println("The following will be permanently deleted (in addition to removing the Tensorleap cluster):")
	for _, o := range customUninstallOptions {
		if selected[o.target] {
			log.Printf("  - %s", o.label)
		}
	}

	confirm := false
	prompt := &survey.Confirm{
		Message: "Proceed? This cannot be undone.",
		Default: false,
	}
	if err := survey.AskOne(prompt, &confirm); err != nil {
		return false, err
	}
	return confirm, nil
}

func init() {
	RootCommand.AddCommand(NewUninstallCmd())
}
