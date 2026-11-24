package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rafaelescrich/gira/internal/jira"
)

var sprintListAll bool

func init() {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List a board's sprints (active and future by default)",
		RunE:  runSprintList,
	}
	cmd.Flags().BoolVar(&sprintListAll, "all", false, "also include closed sprints")
	SprintCmd.AddCommand(cmd)
}

func runSprintList(cmd *cobra.Command, _ []string) error {
	svc, cfg, err := GetService(cmd)
	if err != nil {
		return err
	}

	sprints, err := svc.Sprints(cmd.Context(), flagBoard, sprintListAll)
	if err != nil {
		return err
	}

	if UseJSON(cmd, cfg) {
		return WriteJSON(cmd.OutOrStdout(), sprints)
	}

	out := cmd.OutOrStdout()
	if len(sprints) == 0 {
		fmt.Fprintln(out, "No sprints found.")
		return nil
	}

	tw := NewTableWriter(out)
	fmt.Fprintln(tw, "ID\tNAME\tSTATE\tSTART\tEND")
	for _, sp := range sprints {
		fmt.Fprintln(tw, formatSprintRow(sp))
	}
	return tw.Flush()
}

func formatSprintRow(sp jira.Sprint) string {
	return fmt.Sprintf("%d\t%s\t%s\t%s\t%s", sp.ID, sp.Name, sp.State, sp.StartDate, sp.EndDate)
}
