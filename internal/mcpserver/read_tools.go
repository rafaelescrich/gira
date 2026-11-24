package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rafaelescrich/gira/internal/jira"
	"github.com/rafaelescrich/gira/internal/service"
)

// readOnlyAnnotations is shared by every read tool: it never modifies
// state, so DestructiveHint/IdempotentHint don't apply.
var readOnlyAnnotations = &mcp.ToolAnnotations{
	ReadOnlyHint: true,
}

func registerReadTools(server *mcp.Server, svc *service.Service) {
	addWhoamiTool(server, svc)
	addSearchIssuesTool(server, svc)
	addGetIssueTool(server, svc)
	addListTransitionsTool(server, svc)
	addGetBacklogTool(server, svc)
	addListSprintsTool(server, svc)
	addListEpicsTool(server, svc)
}

func addWhoamiTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_whoami",
		Description: "Show the authenticated Jira account (identity/token check).",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ WhoamiInput) (*mcp.CallToolResult, WhoamiOutput, error) {
		me, err := svc.Me(ctx)
		if err != nil {
			return nil, WhoamiOutput{}, err
		}
		out := WhoamiOutput{AccountID: me.AccountID, DisplayName: me.DisplayName, Email: me.Email}
		summary := fmt.Sprintf("Logged in as %s <%s> (accountId %s)", me.DisplayName, me.Email, me.AccountID)
		return textResult(summary), out, nil
	})
}

func addSearchIssuesTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_search_issues",
		Description: "Search issues by structured filters (status, assignee, type, epic, sprint, text) or raw JQL. Returns at most 50 issues; call jira_get_issue for full detail.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchIssuesInput) (*mcp.CallToolResult, SearchIssuesOutput, error) {
		limit := in.Limit
		if limit <= 0 || limit > 50 {
			limit = 50
		}
		page, err := svc.SearchIssues(ctx, service.SearchParams{
			Project:      in.Project,
			Status:       in.Status,
			Assignee:     in.Assignee,
			Type:         in.Type,
			Epic:         in.Epic,
			Sprint:       in.Sprint,
			Text:         in.Text,
			JQL:          in.JQL,
			UpdatedSince: in.UpdatedSince,
			CreatedSince: in.CreatedSince,
			Sort:         in.Sort,
			SortAsc:      in.SortAsc,
			Limit:        limit,
			PageToken:    in.PageToken,
		})
		if err != nil {
			return nil, SearchIssuesOutput{}, err
		}
		out := SearchIssuesOutput{
			Issues:        issueSummaries(page.Issues),
			HasMore:       !page.IsLast,
			NextPageToken: page.NextPageToken,
		}
		summary := fmt.Sprintf("%d issue(s) found", len(out.Issues))
		if out.HasMore {
			summary += " (more available; pass next_page_token to continue)"
		}
		return textResult(summary), out, nil
	})
}

func addGetIssueTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_get_issue",
		Description: "Fetch full detail for a single issue by key, optionally including its comments.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in GetIssueInput) (*mcp.CallToolResult, GetIssueOutput, error) {
		if strings.TrimSpace(in.Key) == "" {
			return nil, GetIssueOutput{}, fmt.Errorf("key is required")
		}
		issue, err := svc.GetIssue(ctx, in.Key)
		if err != nil {
			return nil, GetIssueOutput{}, err
		}

		out := GetIssueOutput{
			Key:         issue.Key,
			Summary:     issue.Fields.Summary,
			Status:      issue.Fields.Status.Name,
			Type:        issue.Fields.IssueType.Name,
			Assignee:    assigneeName(issue.Fields.Assignee),
			Reporter:    assigneeName(issue.Fields.Reporter),
			Labels:      issue.Fields.Labels,
			Description: jira.RenderADF(issue.Fields.Description),
			Created:     issue.Fields.Created,
			Updated:     issue.Fields.Updated,
		}
		if issue.Fields.Priority != nil {
			out.Priority = issue.Fields.Priority.Name
		}
		if issue.Fields.Parent != nil {
			out.Epic = issue.Fields.Parent.Key
		}

		if in.IncludeComments {
			comments, err := svc.ListComments(ctx, in.Key)
			if err != nil {
				return nil, GetIssueOutput{}, err
			}
			out.Comments = make([]CommentOut, len(comments))
			for i, c := range comments {
				out.Comments[i] = CommentOut{
					ID:      c.ID,
					Author:  assigneeName(c.Author),
					Body:    jira.RenderADF(c.Body),
					Created: c.Created,
				}
			}
		}

		summary := fmt.Sprintf("%s [%s]: %s (assignee: %s)", out.Key, out.Status, out.Summary, orNone(out.Assignee))
		return textResult(summary), out, nil
	})
}

func addListTransitionsTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_list_transitions",
		Description: "List the workflow transitions available from an issue's current status.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ListTransitionsInput) (*mcp.CallToolResult, ListTransitionsOutput, error) {
		transitions, err := svc.ListTransitions(ctx, in.Key)
		if err != nil {
			return nil, ListTransitionsOutput{}, err
		}
		out := ListTransitionsOutput{Transitions: make([]TransitionOut, len(transitions))}
		names := make([]string, len(transitions))
		for i, t := range transitions {
			out.Transitions[i] = TransitionOut{ID: t.ID, Name: t.Name, ToStatus: t.To.Name}
			names[i] = t.To.Name
		}
		summary := fmt.Sprintf("%s: %d transition(s) available: %s", in.Key, len(transitions), strings.Join(names, ", "))
		return textResult(summary), out, nil
	})
}

func addGetBacklogTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_get_backlog",
		Description: "List issues in a board's backlog (not yet assigned to a sprint). Defaults to the server's default board.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in GetBacklogInput) (*mcp.CallToolResult, GetBacklogOutput, error) {
		issues, total, err := svc.Backlog(ctx, in.BoardID, in.Limit)
		if err != nil {
			return nil, GetBacklogOutput{}, err
		}
		out := GetBacklogOutput{
			Issues:  issueSummaries(issues),
			Total:   total,
			HasMore: len(issues) < total,
		}
		summary := fmt.Sprintf("%d of %d backlog issue(s)", len(out.Issues), total)
		return textResult(summary), out, nil
	})
}

func addListSprintsTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_list_sprints",
		Description: "List a board's sprints (active and future by default). Defaults to the server's default board.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ListSprintsInput) (*mcp.CallToolResult, ListSprintsOutput, error) {
		sprints, err := svc.Sprints(ctx, in.BoardID, in.IncludeClosed)
		if err != nil {
			return nil, ListSprintsOutput{}, err
		}
		out := ListSprintsOutput{Sprints: make([]SprintOut, len(sprints))}
		for i, sp := range sprints {
			out.Sprints[i] = SprintOut{
				ID: sp.ID, Name: sp.Name, State: sp.State,
				StartDate: sp.StartDate, EndDate: sp.EndDate, Goal: sp.Goal,
			}
		}
		summary := fmt.Sprintf("%d sprint(s)", len(sprints))
		return textResult(summary), out, nil
	})
}

func addListEpicsTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_list_epics",
		Description: "List a board's epics. Defaults to the server's default board.",
		Annotations: readOnlyAnnotations,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ListEpicsInput) (*mcp.CallToolResult, ListEpicsOutput, error) {
		epics, err := svc.Epics(ctx, in.BoardID)
		if err != nil {
			return nil, ListEpicsOutput{}, err
		}
		out := ListEpicsOutput{Epics: make([]EpicOut, len(epics))}
		for i, e := range epics {
			out.Epics[i] = EpicOut{Key: e.Key, Name: e.Name, Summary: e.Summary, Done: e.Done}
		}
		summary := fmt.Sprintf("%d epic(s)", len(epics))
		return textResult(summary), out, nil
	})
}

// textResult builds a *mcp.CallToolResult with summary as its sole text
// content. StructuredContent is populated separately by mcp.AddTool from
// the handler's typed Out return value (see server.go's package doc).
func textResult(summary string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: summary}}}
}

func orNone(s string) string {
	if s == "" {
		return "unassigned"
	}
	return s
}
