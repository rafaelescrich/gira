package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "remove <KEY> [KEY...]",
		Short: "Move issues out of their sprint and back to the backlog",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runSprintRemove,
	}
	SprintCmd.AddCommand(cmd)
}

func runSprintRemove(cmd *cobra.Command, args []string) error {
	svc, _, err := GetService(cmd)
	if err != nil {
		return err
	}

	if err := svc.MoveToBacklog(cmd.Context(), args); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Moved %d issue(s) to the backlog.\n", len(args))
	return nil
}
