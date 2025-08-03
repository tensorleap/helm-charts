package server

import (
	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

func NewCreateManifestCmd() *cobra.Command {

	var fromLocal bool
	var infraChartVersion string
	var serverChartVersion string
	var tag string

	var output string

	cmd := &cobra.Command{
		Use:     "create-manifest",
		Aliases: []string{"manifest"},
		Short:   "Create a manifest for Tensorleap installation",
		Long:    `Create a manifest for Tensorleap installation`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var mnf *manifest.InstallationManifest
			var err error
			if fromLocal {
				fileGetter := manifest.BuildLocalFileGetter("")
				mnf, err = manifest.GenerateManifestFromLocal(fileGetter)
			} else {
				mnf, err = manifest.GenerateManifestFromRemote(serverChartVersion, infraChartVersion)
			}
			if err != nil {
				return err
			}
			if tag != "" {
				mnf.Tag = tag
			}
			return mnf.Save(output)
		},
	}

	cmd.Flags().StringVar(&serverChartVersion, "tensorleap-chart-version", "", "Build manifest with a specific tensorleap helm chart version")
	cmd.Flags().StringVar(&infraChartVersion, "tensorleap-infra-chart-version", "", "Build manifest with a specific tensorleap helm chart version")
	cmd.Flags().BoolVar(&fromLocal, "local", false, "Build manifest from local files")
	cmd.Flags().StringVarP(&tag, "tag", "t", "", "The manifest tag")
	cmd.Flags().StringVarP(&output, "output", "o", "manifest.yaml", "Output file path")

	return cmd
}

func init() {
	RootCommand.AddCommand(NewCreateManifestCmd())
}
