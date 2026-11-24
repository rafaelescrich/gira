package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rafaelescrich/gira/internal/jira"
)

var backlogLimit int

func init() {
	cmd := &cobra.Command{
		Use:   "backlog",
		Short: "List a board's backlog issues",
		RunE:  runBacklog,
	}
	cmd.Flags().IntVar(&backlogLimit, "limit", 0, "max issues to return (default 50, hard cap 200)")
	RootCmd.AddCommand(cmd)
}

func runBacklog(cmd *cobra.Command, _ []string) error {
	svc, cfg, err := GetService(cmd)
	if err != nil {
		return err
	}

	issues, total, err := svc.Backlog(cmd.Context(), flagBoard, backlogLimit)
	if err != nil {
		return err
	}

	if UseJSON(cmd, cfg) {
		return WriteJSON(cmd.OutOrStdout(), struct {
			Issues []jira.Issue `json:"issues"`
			Total  int          `json:"total"`
		}{Issues: issues, Total: total})
	}

	out := cmd.OutOrStdout()
	if len(issues) == 0 {
		fmt.Fprintln(out, "Backlog is empty.")
		return nil
	}

	tw := NewTableWriter(out)
	fmt.Fprintln(tw, "KEY\tSTATUS\tTYPE\tASSIGNEE\tSUMMARY")
	for _, iss := range issues {
		fmt.Fprintln(tw, formatListRow(iss))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if total > len(issues) {
		fmt.Fprintf(out, "(%d of %d; raise --limit to see more)\n", len(issues), total)
	}
	return nil
}
