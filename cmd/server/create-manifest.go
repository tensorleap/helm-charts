package server

import (
	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

func NewCreateManifestCmd() *cobra.Command {

	var tag string
	var branch string
	var output string

	cmd := &cobra.Command{
		Use:     "create-manifest",
		Aliases: []string{"manifest"},
		Short:   "Create a manifest for Tensorleap installation",
		Long:    `Create a manifest for Tensorleap installation`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mnf, err := manifest.GenerateManifest(branch, tag)
			if err != nil {
				return err
			}
			return mnf.Save(output)

		},
	}

	cmd.Flags().StringVarP(&tag, "tag", "t", "", "Build manifest for a specific tag")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Build manifest for a specific branch")
	cmd.Flags().StringVarP(&output, "output", "o", "manifest.yaml", "Output file path")

	return cmd
}

func init() {
	RootCommand.AddCommand(NewCreateManifestCmd())
}
