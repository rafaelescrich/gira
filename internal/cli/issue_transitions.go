package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	cmd := &cobra.Command{
		Use:   "transitions <key>",
		Short: "List the transitions available from an issue's current status",
		Args:  cobra.ExactArgs(1),
		RunE:  runIssueTransitions,
	}
	IssueCmd.AddCommand(cmd)
}

func runIssueTransitions(cmd *cobra.Command, args []string) error {
	svc, cfg, err := GetService(cmd)
	if err != nil {
		return err
	}

	transitions, err := svc.ListTransitions(cmd.Context(), args[0])
	if err != nil {
		return err
	}

	if UseJSON(cmd, cfg) {
		return WriteJSON(cmd.OutOrStdout(), transitions)
	}

	out := cmd.OutOrStdout()
	if len(transitions) == 0 {
		fmt.Fprintln(out, "No transitions available.")
		return nil
	}
	tw := NewTableWriter(out)
	fmt.Fprintln(tw, "TRANSITION\tTO STATUS")
	for _, t := range transitions {
		fmt.Fprintf(tw, "%s\t%s\n", t.Name, t.To.Name)
	}
	return tw.Flush()
}
