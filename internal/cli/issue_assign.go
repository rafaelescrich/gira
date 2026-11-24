package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "assign <key> <who>",
		Short: "Assign an issue: me, unassigned, an email, or a display name",
		Args:  cobra.ExactArgs(2),
		RunE:  runIssueAssign,
	}
	IssueCmd.AddCommand(cmd)
}

func runIssueAssign(cmd *cobra.Command, args []string) error {
	svc, _, err := GetService(cmd)
	if err != nil {
		return err
	}

	if err := svc.Assign(cmd.Context(), args[0], args[1]); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s assigned to %s\n", args[0], args[1])
	return nil
}
