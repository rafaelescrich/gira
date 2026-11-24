package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rafaelescrich/gira/internal/secrets"
)

func init() {
	AuthCmd.AddCommand(&cobra.Command{
		Use:   "logout",
		Short: "Remove the stored Jira API token from the OS keychain",
		RunE:  runAuthLogout,
	})
}

func runAuthLogout(cmd *cobra.Command, _ []string) error {
	cfg, err := LoadConfig(cmd)
	if err != nil {
		return err
	}
	if cfg.Email == "" {
		return fmt.Errorf("no Jira account email configured; nothing to log out of")
	}
	if err := secrets.DeleteToken(cfg.Email); err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			fmt.Fprintf(cmd.OutOrStdout(), "No token stored for %s.\n", cfg.Email)
			return nil
		}
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Removed stored token for %s.\n", cfg.Email)
	return nil
}
