package server

import (
	"fmt"
	"github.com/spf13/cobra"
)

func init() {
	RootCommand.AddCommand(&cobra.Command{
		Use:   "check",
		Short: "Troubleshoot local installation issues",
		Long: `Troubleshoot local installation issues
  Run this if your local installation stopped working.
    `,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Check command")
		},
	})
}
