package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rafaelescrich/gira/internal/jira"
)

var issueViewComments bool

func init() {
	cmd := &cobra.Command{
		Use:   "view <key>",
		Short: "Show a single issue",
		Args:  cobra.ExactArgs(1),
		RunE:  runIssueView,
	}
	cmd.Flags().BoolVar(&issueViewComments, "comments", false, "include comments")
	IssueCmd.AddCommand(cmd)
}

func runIssueView(cmd *cobra.Command, args []string) error {
	svc, cfg, err := GetService(cmd)
	if err != nil {
		return err
	}

	issue, err := svc.GetIssue(cmd.Context(), args[0])
	if err != nil {
		return err
	}

	var comments []jira.Comment
	if issueViewComments {
		comments, err = svc.ListComments(cmd.Context(), args[0])
		if err != nil {
			return err
		}
	}

	if UseJSON(cmd, cfg) {
		if issueViewComments {
			return WriteJSON(cmd.OutOrStdout(), struct {
				jira.Issue
				Comments []jira.Comment `json:"comments"`
			}{Issue: issue, Comments: comments})
		}
		return WriteJSON(cmd.OutOrStdout(), issue)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%s: %s\n", issue.Key, issue.Fields.Summary)
	fmt.Fprintf(out, "Status:   %s\n", issue.Fields.Status.Name)
	fmt.Fprintf(out, "Type:     %s\n", issue.Fields.IssueType.Name)

	assignee := "unassigned"
	if issue.Fields.Assignee != nil {
		assignee = issue.Fields.Assignee.DisplayName
	}
	fmt.Fprintf(out, "Assignee: %s\n", assignee)

	if issue.Fields.Priority != nil {
		fmt.Fprintf(out, "Priority: %s\n", issue.Fields.Priority.Name)
	}
	if issue.Fields.Parent != nil {
		fmt.Fprintf(out, "Epic:     %s\n", issue.Fields.Parent.Key)
	}
	if len(issue.Fields.Labels) > 0 {
		fmt.Fprintf(out, "Labels:   %s\n", strings.Join(issue.Fields.Labels, ", "))
	}
	fmt.Fprintf(out, "Created:  %s\n", formatLocalDateTime(issue.Fields.Created))
	fmt.Fprintf(out, "Updated:  %s\n", formatLocalDateTime(issue.Fields.Updated))

	if desc := jira.RenderADF(issue.Fields.Description); desc != "" {
		fmt.Fprintf(out, "\n%s\n", desc)
	}

	if issueViewComments {
		fmt.Fprintf(out, "\nComments (%d):\n", len(comments))
		for _, c := range comments {
			author := "unknown"
			if c.Author != nil {
				author = c.Author.DisplayName
			}
			fmt.Fprintf(out, "- %s (%s):\n", author, c.Created)
			if body := jira.RenderADF(c.Body); body != "" {
				for _, line := range strings.Split(body, "\n") {
					fmt.Fprintf(out, "    %s\n", line)
				}
			}
		}
	}
	return nil
}
