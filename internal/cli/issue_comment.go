package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "comment <key> <text|->",
		Short: `Add a comment (markdown). Use "-" to read the comment from stdin`,
		Args:  cobra.ExactArgs(2),
		RunE:  runIssueComment,
	}
	IssueCmd.AddCommand(cmd)
}

func runIssueComment(cmd *cobra.Command, args []string) error {
	svc, cfg, err := GetService(cmd)
	if err != nil {
		return err
	}

	text, err := readTextArg(args[1])
	if err != nil {
		return err
	}

	comment, err := svc.AddComment(cmd.Context(), args[0], text)
	if err != nil {
		return err
	}

	if UseJSON(cmd, cfg) {
		return WriteJSON(cmd.OutOrStdout(), comment)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Comment added to %s (id %s)\n", args[0], comment.ID)
	return nil
}
