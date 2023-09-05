package server

import (
	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

func NewCreateManifestCmd() *cobra.Command {

	var serverChartVersion string
	var output string

	cmd := &cobra.Command{
		Use:     "create-manifest",
		Aliases: []string{"manifest"},
		Short:   "Create a manifest for Tensorleap installation",
		Long:    `Create a manifest for Tensorleap installation`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mnf, err := manifest.GenerateManifest(serverChartVersion)
			if err != nil {
				return err
			}
			return mnf.Save(output)

		},
	}

	cmd.Flags().StringVar(&serverChartVersion, "tensorleap-chart-version", "", "Build manifest with a specific tensorleap helm chart version")
	cmd.Flags().StringVarP(&output, "output", "o", "manifest.yaml", "Output file path")

	return cmd
}

func init() {
	RootCommand.AddCommand(NewCreateManifestCmd())
}
