package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rafaelescrich/gira/internal/service"
)

// Every write tool shares DestructiveHint: false (gira has no delete
// tools; see blueprint §8). IdempotentHint is set per tool below:
// true for transition/assign/move (repeating the call with the same
// arguments leaves the issue in the same end state), false for
// create/update/comment (repeating the call creates another issue/
// comment, or is not obviously side-effect-free).

func addCreateIssueTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_create_issue",
		Description: "Create a new Jira issue. Defaults to the server's default project; team-managed projects link epics via the epic field (mapped to fields.parent).",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CreateIssueInput) (*mcp.CallToolResult, IssueRefOutput, error) {
		ref, err := svc.CreateIssue(ctx, service.CreateParams{
			Project:             in.Project,
			Type:                in.Type,
			Summary:             in.Summary,
			DescriptionMarkdown: in.DescriptionMarkdown,
			Epic:                in.Epic,
			Assignee:            in.Assignee,
			Labels:              in.Labels,
			Priority:            in.Priority,
		})
		if err != nil {
			return nil, IssueRefOutput{}, err
		}
		out := IssueRefOutput{Key: ref.Key, Self: ref.Self}
		return textResult(fmt.Sprintf("Created %s: %s", ref.Key, in.Summary)), out, nil
	})
}

func addUpdateIssueTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_update_issue",
		Description: "Edit an existing issue's summary, description, labels, priority, epic, or assignee. Only fields explicitly set are changed.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UpdateIssueInput) (*mcp.CallToolResult, UpdateIssueOutput, error) {
		err := svc.UpdateIssue(ctx, in.Key, service.UpdateParams{
			Summary:             in.Summary,
			DescriptionMarkdown: in.DescriptionMarkdown,
			Labels:              in.Labels,
			Priority:            in.Priority,
			Epic:                in.Epic,
			Assignee:            in.Assignee,
		})
		if err != nil {
			return nil, UpdateIssueOutput{}, err
		}
		out := UpdateIssueOutput{Key: strings.ToUpper(strings.TrimSpace(in.Key)), Updated: true}
		return textResult(fmt.Sprintf("Updated %s", out.Key)), out, nil
	})
}

func addTransitionIssueTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_transition_issue",
		Description: "Move an issue to a new status, by status name (\"In Progress\") or transition name (\"Start Progress\"). Call jira_list_transitions first if unsure which are valid.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in TransitionIssueInput) (*mcp.CallToolResult, TransitionIssueOutput, error) {
		status, err := svc.Transition(ctx, in.Key, in.ToStatus)
		if err != nil {
			return nil, TransitionIssueOutput{}, err
		}
		out := TransitionIssueOutput{Key: strings.ToUpper(strings.TrimSpace(in.Key)), Status: status.Name}
		return textResult(fmt.Sprintf("%s is now %s", out.Key, out.Status)), out, nil
	})
}

func addAssignIssueTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_assign_issue",
		Description: "Assign (or unassign) an issue. assignee is \"me\", \"unassigned\", an email address, or a display name.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in AssignIssueInput) (*mcp.CallToolResult, AssignIssueOutput, error) {
		if err := svc.Assign(ctx, in.Key, in.Assignee); err != nil {
			return nil, AssignIssueOutput{}, err
		}
		out := AssignIssueOutput{Key: strings.ToUpper(strings.TrimSpace(in.Key)), Assignee: in.Assignee}
		return textResult(fmt.Sprintf("%s assigned to %s", out.Key, out.Assignee)), out, nil
	})
}

func addAddCommentTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_add_comment",
		Description: "Post a comment (Markdown) on an issue.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: boolPtr(false),
			IdempotentHint:  false,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in AddCommentInput) (*mcp.CallToolResult, AddCommentOutput, error) {
		comment, err := svc.AddComment(ctx, in.Key, in.BodyMarkdown)
		if err != nil {
			return nil, AddCommentOutput{}, err
		}
		out := AddCommentOutput{Key: strings.ToUpper(strings.TrimSpace(in.Key)), CommentID: comment.ID}
		return textResult(fmt.Sprintf("Comment added to %s", out.Key)), out, nil
	})
}

func addMoveToSprintTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_move_to_sprint",
		Description: "Move up to 20 issues into a sprint (\"current\", \"next\", an ID, or a name).",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in MoveToSprintInput) (*mcp.CallToolResult, MoveToSprintOutput, error) {
		sprint, err := svc.MoveToSprint(ctx, in.BoardID, in.Sprint, in.Keys)
		if err != nil {
			return nil, MoveToSprintOutput{}, err
		}
		out := MoveToSprintOutput{SprintID: sprint.ID, SprintName: sprint.Name, Keys: in.Keys}
		return textResult(fmt.Sprintf("Moved %d issue(s) to sprint %q (%d)", len(in.Keys), sprint.Name, sprint.ID)), out, nil
	})
}

func addMoveToBacklogTool(server *mcp.Server, svc *service.Service) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "jira_move_to_backlog",
		Description: "Move up to 20 issues out of their sprint and back to the backlog.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: boolPtr(false),
			IdempotentHint:  true,
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in MoveToBacklogInput) (*mcp.CallToolResult, MoveToBacklogOutput, error) {
		if err := svc.MoveToBacklog(ctx, in.Keys); err != nil {
			return nil, MoveToBacklogOutput{}, err
		}
		out := MoveToBacklogOutput(in)
		return textResult(fmt.Sprintf("Moved %d issue(s) to the backlog", len(in.Keys))), out, nil
	})
}
