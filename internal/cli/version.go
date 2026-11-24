package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// version is gira's version string. Set via SetVersion from main, and
// mirrored into internal/jira's User-Agent by main as well.
var version = "dev"

func init() {
	RootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print gira's version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), "gira "+version)
			return nil
		},
	})
}
