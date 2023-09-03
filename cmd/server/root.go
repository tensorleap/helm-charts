package server

import (
	"github.com/spf13/cobra"
)

var RootCommand = &cobra.Command{
	Use:   "server",
	Short: "Manage local server installation of Tensorleap",
	Long:  `Manage local server installation of Tensorleap`,
}
