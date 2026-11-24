package cli

import "github.com/spf13/cobra"

// IssueCmd is the `gira issue` command group. Subcommands (list, view,
// and later write/agile commands added by other agents) register
// themselves onto it from their own init(), in their own files.
var IssueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Manage Jira issues",
}

func init() {
	RootCmd.AddCommand(IssueCmd)
}
