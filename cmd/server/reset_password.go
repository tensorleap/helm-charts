package server

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/tensorleap/helm-charts/pkg/local"
	"github.com/tensorleap/helm-charts/pkg/log"
	"github.com/tensorleap/helm-charts/pkg/server"
)

func NewResetPasswordCmd() *cobra.Command {
	var password string
	cmd := &cobra.Command{
		Use:   "reset-password <email>",
		Short: "Set a temporary password for a user; they must choose a new one at next browser login",
		Long: `Set a temporary password for a Tensorleap user (Keycloak). Existing sessions are
revoked and the user must choose a new password at the next browser login.
Run it on the machine where the Tensorleap server is installed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			log.SetCommandName("reset-password")

			if _, err := server.InitDataDirFunc(cmd.Context(), ""); err != nil {
				return err
			}
			close, err := local.SetupInfra("reset-password")
			if err != nil {
				return err
			}
			defer close()

			temporaryPassword, err := server.ResetPassword(cmd.Context(), args[0], password)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Temporary password for %s: %s\nThe user must sign in through the browser and set a new password; `leap auth login -u/-p` works again after that.\n", args[0], temporaryPassword)
			return nil
		},
	}
	cmd.Flags().StringVar(&password, "password", "", "Temporary password to set (generated when omitted)")
	return cmd
}

func init() {
	RootCommand.AddCommand(NewResetPasswordCmd())
}
