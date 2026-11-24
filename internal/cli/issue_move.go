package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "move <key> <status>",
		Short: "Transition an issue by status name or transition name",
		Args:  cobra.ExactArgs(2),
		RunE:  runIssueMove,
	}
	IssueCmd.AddCommand(cmd)
}

func runIssueMove(cmd *cobra.Command, args []string) error {
	svc, cfg, err := GetService(cmd)
	if err != nil {
		return err
	}

	status, err := svc.Transition(cmd.Context(), args[0], args[1])
	if err != nil {
		return err
	}

	if UseJSON(cmd, cfg) {
		return WriteJSON(cmd.OutOrStdout(), status)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", args[0], status.Name)
	return nil
}
