package cli

import "github.com/spf13/cobra"

// EpicCmd is the `gira epic` command group. Subcommands (list, and any
// later additions) register themselves onto it from their own init(), in
// their own files.
var EpicCmd = &cobra.Command{
	Use:   "epic",
	Short: "View epics",
}

func init() {
	RootCmd.AddCommand(EpicCmd)
}
