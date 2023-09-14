package server

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/version"
)

var showVersion = false

var RootCommand = &cobra.Command{
	Use:   "server",
	Short: "Manage local server installation of Tensorleap",
	Long:  `Manage local server installation of Tensorleap`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf(version.Version)
	},
}

func init() {
	RootCommand.Flags().BoolVarP(&showVersion, "version", "v", false, "Show version")
}
