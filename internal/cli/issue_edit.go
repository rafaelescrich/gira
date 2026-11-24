package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rafaelescrich/gira/internal/service"
)

var (
	issueEditSummary     string
	issueEditDescription string
	issueEditLabels      string
	issueEditPriority    string
	issueEditEpic        string
	issueEditAssignee    string
)

func init() {
	cmd := &cobra.Command{
		Use:   "edit <key>",
		Short: "Edit an issue",
		Args:  cobra.ExactArgs(1),
		RunE:  runIssueEdit,
	}
	cmd.Flags().StringVar(&issueEditSummary, "summary", "", "new summary")
	cmd.Flags().StringVar(&issueEditDescription, "description", "", `new description markdown, "-" for stdin, or a file path`)
	cmd.Flags().StringVar(&issueEditLabels, "labels", "", "comma-separated labels (replaces existing labels)")
	cmd.Flags().StringVar(&issueEditPriority, "priority", "", "priority name (e.g. High)")
	cmd.Flags().StringVar(&issueEditEpic, "epic", "", "parent epic key")
	cmd.Flags().StringVar(&issueEditAssignee, "assignee", "", "me, unassigned, email, or display name")
	IssueCmd.AddCommand(cmd)
}

func runIssueEdit(cmd *cobra.Command, args []string) error {
	svc, _, err := GetService(cmd)
	if err != nil {
		return err
	}

	flags := cmd.Flags()
	var p service.UpdateParams

	if flags.Changed("summary") {
		p.Summary = &issueEditSummary
	}
	if flags.Changed("description") {
		desc, err := readTextArg(issueEditDescription)
		if err != nil {
			return err
		}
		p.DescriptionMarkdown = &desc
	}
	if flags.Changed("labels") {
		labels := splitLabels(issueEditLabels)
		p.Labels = &labels
	}
	if flags.Changed("priority") {
		p.Priority = &issueEditPriority
	}
	if flags.Changed("epic") {
		p.Epic = &issueEditEpic
	}
	if flags.Changed("assignee") {
		p.Assignee = &issueEditAssignee
	}

	if err := svc.UpdateIssue(cmd.Context(), args[0], p); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Updated %s\n", args[0])
	return nil
}
