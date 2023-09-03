package server

import (
	"os"
	"path"

	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server/airgap"
	"github.com/tensorleap/helm-charts/pkg/server/manifest"
)

func NewPackInstallationCmd() *cobra.Command {
	var output string
	var tag string
	var branch string

	cmd := &cobra.Command{
		Use:     "pack-installation [installConfigPath]",
		Aliases: []string{"pack"},
		Short:   "Pack an air-gap installation of Tensorleap",
		Long:    `Pack an air-gap installation of Tensorleap`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var mnf *manifest.InstallationManifest
			var err error
			if len(args) > 0 {
				installConfigPath := args[0]
				mnf, err = manifest.Load(installConfigPath)
				if err != nil {
					return err
				}

			} else {
				mnf, err = manifest.GenerateManifest(branch, tag)
				if err != nil {
					return err
				}
			}
			err = os.MkdirAll(path.Dir(output), 0755)
			if err != nil {
				return err
			}

			outputFile, err := os.Create(output)
			if err != nil {
				return err
			}
			defer outputFile.Close()

			err = airgap.Pack(mnf, outputFile)
			if err != nil {
				return err
			}
			log.Info("Successfully pack air-gap installation")
			return nil
		},
	}

	cmd.Flags().StringVarP(&tag, "tag", "t", "", "Build manifest for a specific tag")
	cmd.Flags().StringVarP(&branch, "branch", "b", "", "Build manifest for a specific branch")
	cmd.Flags().StringVarP(&output, "output", "o", "pack.tar", "Output file path")
	return cmd
}

func init() {
	RootCommand.AddCommand(NewPackInstallationCmd())
}
