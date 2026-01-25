package server

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

func NewCreateManifestCmd() *cobra.Command {

	var fromLocal bool
	var localDir string
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
			isLocal := fromLocal || localDir != ""
			fmt.Println("Creating manifest from local files")
			fmt.Println("fromLocal:", isLocal)
			fmt.Println("localDir:", localDir)
			fmt.Println("serverChartVersion:", serverChartVersion)
			fmt.Println("infraChartVersion:", infraChartVersion)
			fmt.Println("tag:", tag)
			fmt.Println("output:", output)
			if isLocal {
				fileGetter := manifest.BuildLocalFileGetter(localDir)
				mnf, err = manifest.GenerateManifestFromLocal(fileGetter, localDir)
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
	cmd.Flags().BoolVar(&fromLocal, "local", false, "Build manifest from local files (current directory)")
	cmd.Flags().StringVar(&localDir, "local-dir", "", "Build manifest from local files at the specified directory path")
	cmd.Flags().StringVarP(&tag, "tag", "t", "", "The manifest tag")
	cmd.Flags().StringVarP(&output, "output", "o", "manifest.yaml", "Output file path")

	return cmd
}

func init() {
	RootCommand.AddCommand(NewCreateManifestCmd())
}
