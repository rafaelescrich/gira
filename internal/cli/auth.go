package cli

import "github.com/spf13/cobra"

// AuthCmd is the `gira auth` command group. Subcommands (status, login,
// logout) register themselves onto it from their own init(), in their own
// files.
var AuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage Jira authentication",
}

func init() {
	RootCmd.AddCommand(AuthCmd)
}
