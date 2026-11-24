package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rafaelescrich/gira/internal/jira"
	"github.com/rafaelescrich/gira/internal/service"
)

var (
	issueListStatus       string
	issueListAssignee     string
	issueListType         string
	issueListEpic         string
	issueListSprint       string
	issueListJQL          string
	issueListLimit        int
	issueListUpdatedSince string
	issueListCreatedSince string
	issueListSort         string
	issueListAsc          bool
)

func init() {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "Search issues",
		RunE:  runIssueList,
	}
	cmd.Flags().StringVar(&issueListStatus, "status", "", "filter by status name")
	cmd.Flags().StringVar(&issueListAssignee, "assignee", "", "filter by assignee: me, unassigned, email, or name")
	cmd.Flags().StringVar(&issueListType, "type", "", "filter by issue type")
	cmd.Flags().StringVar(&issueListEpic, "epic", "", "filter by parent epic key")
	cmd.Flags().StringVar(&issueListSprint, "sprint", "", `filter by sprint: "current" or a sprint ID`)
	cmd.Flags().StringVar(&issueListJQL, "jql", "", "raw JQL, always wrapped in the project allowlist guard")
	cmd.Flags().IntVar(&issueListLimit, "limit", 0, "max issues to return (default 50, hard cap 200)")
	cmd.Flags().StringVar(&issueListUpdatedSince, "updated-since", "", `filter by updated date, e.g. "-7d" or "2026-09-01"`)
	cmd.Flags().StringVar(&issueListCreatedSince, "created-since", "", `filter by created date, e.g. "-7d" or "2026-09-01"`)
	cmd.Flags().StringVar(&issueListSort, "sort", "updated", `sort by "created" or "updated" (structured filters only)`)
	cmd.Flags().BoolVar(&issueListAsc, "asc", false, "sort ascending instead of the default descending")
	IssueCmd.AddCommand(cmd)
}

func runIssueList(cmd *cobra.Command, _ []string) error {
	svc, cfg, err := GetService(cmd)
	if err != nil {
		return err
	}

	page, err := svc.SearchIssues(cmd.Context(), service.SearchParams{
		Status:       issueListStatus,
		Assignee:     issueListAssignee,
		Type:         issueListType,
		Epic:         issueListEpic,
		Sprint:       issueListSprint,
		JQL:          issueListJQL,
		Limit:        issueListLimit,
		UpdatedSince: issueListUpdatedSince,
		CreatedSince: issueListCreatedSince,
		Sort:         issueListSort,
		SortAsc:      issueListAsc,
	})
	if err != nil {
		return err
	}

	if UseJSON(cmd, cfg) {
		return WriteJSON(cmd.OutOrStdout(), page)
	}

	out := cmd.OutOrStdout()
	if len(page.Issues) == 0 {
		fmt.Fprintln(out, "No issues found.")
		return nil
	}

	tw := NewTableWriter(out)
	fmt.Fprintln(tw, "KEY\tSTATUS\tTYPE\tASSIGNEE\tCREATED\tUPDATED\tSUMMARY")
	for _, iss := range page.Issues {
		fmt.Fprintln(tw, formatListRow(iss))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if !page.IsLast {
		fmt.Fprintln(out, "(more results available; refine filters or raise --limit)")
	}
	return nil
}

func formatListRow(iss jira.Issue) string {
	assignee := "unassigned"
	if iss.Fields.Assignee != nil {
		assignee = iss.Fields.Assignee.DisplayName
	}
	return fmt.Sprintf("%s\t%s\t%s\t%s\t%s\t%s\t%s",
		iss.Key, iss.Fields.Status.Name, iss.Fields.IssueType.Name, assignee,
		formatShortDate(iss.Fields.Created), formatShortDate(iss.Fields.Updated), iss.Fields.Summary)
}
