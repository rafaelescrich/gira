package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rafaelescrich/gira/internal/service"
)

var (
	issueCreateType        string
	issueCreateSummary     string
	issueCreateDescription string
	issueCreateEpic        string
	issueCreateAssignee    string
	issueCreateLabels      string
	issueCreatePriority    string
)

func init() {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an issue",
		RunE:  runIssueCreate,
	}
	cmd.Flags().StringVar(&issueCreateType, "type", "", "issue type (e.g. Task, Bug, Story) (required)")
	cmd.Flags().StringVar(&issueCreateSummary, "summary", "", "issue summary (required)")
	cmd.Flags().StringVar(&issueCreateDescription, "description", "", `description markdown, "-" for stdin, or a file path`)
	cmd.Flags().StringVar(&issueCreateEpic, "epic", "", "parent epic key")
	cmd.Flags().StringVar(&issueCreateAssignee, "assignee", "", "me, unassigned, email, or display name")
	cmd.Flags().StringVar(&issueCreateLabels, "labels", "", "comma-separated labels")
	cmd.Flags().StringVar(&issueCreatePriority, "priority", "", "priority name (e.g. High)")
	IssueCmd.AddCommand(cmd)
}

func runIssueCreate(cmd *cobra.Command, _ []string) error {
	svc, cfg, err := GetService(cmd)
	if err != nil {
		return err
	}

	desc, err := readTextArg(issueCreateDescription)
	if err != nil {
		return err
	}

	ref, err := svc.CreateIssue(cmd.Context(), service.CreateParams{
		Project:             flagProject,
		Type:                issueCreateType,
		Summary:             issueCreateSummary,
		DescriptionMarkdown: desc,
		Epic:                issueCreateEpic,
		Assignee:            issueCreateAssignee,
		Labels:              splitLabels(issueCreateLabels),
		Priority:            issueCreatePriority,
	})
	if err != nil {
		return err
	}

	if UseJSON(cmd, cfg) {
		return WriteJSON(cmd.OutOrStdout(), ref)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", ref.Key)
	return nil
}

// splitLabels splits a comma-separated label list, trimming whitespace and
// dropping empty entries. Returns nil for an empty/blank input.
func splitLabels(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
