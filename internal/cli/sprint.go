package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// SprintCmd is the `gira sprint` command group. Subcommands (list, add,
// remove, issues) register themselves onto it from their own init(), in
// their own files.
var SprintCmd = &cobra.Command{
	Use:   "sprint",
	Short: "Manage sprints and their issues",
}

var sprintIssuesLimit int

func init() {
	RootCmd.AddCommand(SprintCmd)

	cmd := &cobra.Command{
		Use:   "issues <sprint>",
		Short: `List a sprint's issues ("current", "next", an ID, or a name)`,
		Args:  cobra.ExactArgs(1),
		RunE:  runSprintIssues,
	}
	cmd.Flags().IntVar(&sprintIssuesLimit, "limit", 0, "max issues to return (default 50, hard cap 200)")
	SprintCmd.AddCommand(cmd)
}

func runSprintIssues(cmd *cobra.Command, args []string) error {
	svc, cfg, err := GetService(cmd)
	if err != nil {
		return err
	}

	issues, sprint, err := svc.SprintIssues(cmd.Context(), flagBoard, args[0], sprintIssuesLimit)
	if err != nil {
		return err
	}

	if UseJSON(cmd, cfg) {
		return WriteJSON(cmd.OutOrStdout(), struct {
			Sprint any `json:"sprint"`
			Issues any `json:"issues"`
		}{Sprint: sprint, Issues: issues})
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Sprint %d: %s (%s)\n", sprint.ID, sprint.Name, sprint.State)
	if len(issues) == 0 {
		fmt.Fprintln(out, "No issues in this sprint.")
		return nil
	}

	tw := NewTableWriter(out)
	fmt.Fprintln(tw, "KEY\tSTATUS\tTYPE\tASSIGNEE\tSUMMARY")
	for _, iss := range issues {
		fmt.Fprintln(tw, formatListRow(iss))
	}
	return tw.Flush()
}
