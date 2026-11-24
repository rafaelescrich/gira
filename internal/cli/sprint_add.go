package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "add <sprint|current> <KEY> [KEY...]",
		Short: "Move issues into a sprint",
		Args:  cobra.MinimumNArgs(2),
		RunE:  runSprintAdd,
	}
	SprintCmd.AddCommand(cmd)
}

func runSprintAdd(cmd *cobra.Command, args []string) error {
	svc, cfg, err := GetService(cmd)
	if err != nil {
		return err
	}

	sprint, err := svc.MoveToSprint(cmd.Context(), flagBoard, args[0], args[1:])
	if err != nil {
		return err
	}

	if UseJSON(cmd, cfg) {
		return WriteJSON(cmd.OutOrStdout(), sprint)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Moved %d issue(s) to sprint %d (%s).\n", len(args[1:]), sprint.ID, sprint.Name)
	return nil
}
