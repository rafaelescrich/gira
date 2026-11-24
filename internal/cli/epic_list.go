package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rafaelescrich/gira/internal/jira"
)

func init() {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List a board's epics",
		RunE:  runEpicList,
	}
	EpicCmd.AddCommand(cmd)
}

func runEpicList(cmd *cobra.Command, _ []string) error {
	svc, cfg, err := GetService(cmd)
	if err != nil {
		return err
	}

	epics, err := svc.Epics(cmd.Context(), flagBoard)
	if err != nil {
		return err
	}

	if UseJSON(cmd, cfg) {
		return WriteJSON(cmd.OutOrStdout(), epics)
	}

	out := cmd.OutOrStdout()
	if len(epics) == 0 {
		fmt.Fprintln(out, "No epics found.")
		return nil
	}

	tw := NewTableWriter(out)
	fmt.Fprintln(tw, "KEY\tDONE\tSUMMARY")
	for _, ep := range epics {
		fmt.Fprintln(tw, formatEpicRow(ep))
	}
	return tw.Flush()
}

func formatEpicRow(ep jira.Epic) string {
	name := ep.Name
	if name == "" {
		name = ep.Summary
	}
	return fmt.Sprintf("%s\t%v\t%s", ep.Key, ep.Done, name)
}
