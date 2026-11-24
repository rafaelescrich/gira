// Package mcpserver wires gira's internal/service use cases into an MCP
// server (blueprint §7): typed tools over stdio for Claude Code and other
// MCP clients. Like internal/cli, this package only parses input and
// formats output — every Jira call and safety check happens in
// internal/service, so the CLI and MCP front ends behave identically.
package mcpserver

import "github.com/rafaelescrich/gira/internal/jira"

// --- Read tool input/output types --------------------------------------

// WhoamiInput takes no arguments.
type WhoamiInput struct{}

// WhoamiOutput identifies the authenticated Jira account.
type WhoamiOutput struct {
	AccountID   string `json:"account_id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
}

// SearchIssuesInput mirrors service.SearchParams; at most one of the
// structured filters or JQL is normally used at a time (JQL takes
// precedence when set).
type SearchIssuesInput struct {
	Project      string `json:"project,omitempty" jsonschema:"restrict to this project (defaults to the server's default project)"`
	Status       string `json:"status,omitempty" jsonschema:"filter by status name, e.g. \"In Progress\""`
	Assignee     string `json:"assignee,omitempty" jsonschema:"\"me\", \"unassigned\", an email address, or a display name"`
	Type         string `json:"type,omitempty" jsonschema:"filter by issue type name, e.g. \"Bug\""`
	Epic         string `json:"epic,omitempty" jsonschema:"filter by parent epic issue key, e.g. \"PROJ-3\""`
	Sprint       string `json:"sprint,omitempty" jsonschema:"\"current\" for the active sprint, or a numeric sprint ID"`
	Text         string `json:"text,omitempty" jsonschema:"free-text search over summary/description"`
	JQL          string `json:"jql,omitempty" jsonschema:"raw JQL; wrapped in a project allowlist guard and used instead of the structured filters above"`
	Limit        int    `json:"limit,omitempty" jsonschema:"max issues to return, default 50, hard cap 50"`
	PageToken    string `json:"page_token,omitempty" jsonschema:"continue a previous search from its next_page_token"`
	UpdatedSince string `json:"updated_since,omitempty" jsonschema:"JQL-compatible duration (\"-7d\") or date"`
	CreatedSince string `json:"created_since,omitempty" jsonschema:"JQL-compatible duration (\"-7d\") or date"`
	Sort         string `json:"sort,omitempty" jsonschema:"sort by \"created\" or \"updated\" (default \"updated\"); ignored when jql is set"`
	SortAsc      bool   `json:"sort_asc,omitempty" jsonschema:"sort ascending instead of the default descending"`
}

// IssueSummary is the compact issue shape returned by list-like tools
// (search, backlog, sprint issues): key, summary, status, assignee,
// updated, per blueprint §7.
type IssueSummary struct {
	Key      string `json:"key"`
	Summary  string `json:"summary"`
	Status   string `json:"status"`
	Type     string `json:"type,omitempty"`
	Assignee string `json:"assignee,omitempty"`
	Epic     string `json:"epic,omitempty"`
	Created  string `json:"created,omitempty"`
	Updated  string `json:"updated,omitempty"`
}

// SearchIssuesOutput is jira_search_issues' structured result.
type SearchIssuesOutput struct {
	Issues        []IssueSummary `json:"issues"`
	Total         int            `json:"total,omitempty"`
	HasMore       bool           `json:"has_more"`
	NextPageToken string         `json:"next_page_token,omitempty"`
}

// GetIssueInput requests a single issue.
type GetIssueInput struct {
	Key             string `json:"key" jsonschema:"issue key, e.g. PROJ-12"`
	IncludeComments bool   `json:"include_comments,omitempty" jsonschema:"also fetch and include the issue's comments"`
}

// CommentOut is a rendered (Markdown) issue comment.
type CommentOut struct {
	ID      string `json:"id"`
	Author  string `json:"author,omitempty"`
	Body    string `json:"body"`
	Created string `json:"created,omitempty"`
}

// GetIssueOutput is jira_get_issue's structured result.
type GetIssueOutput struct {
	Key         string       `json:"key"`
	Summary     string       `json:"summary"`
	Status      string       `json:"status"`
	Type        string       `json:"type"`
	Assignee    string       `json:"assignee,omitempty"`
	Reporter    string       `json:"reporter,omitempty"`
	Priority    string       `json:"priority,omitempty"`
	Epic        string       `json:"epic,omitempty"`
	Labels      []string     `json:"labels,omitempty"`
	Description string       `json:"description,omitempty"`
	Created     string       `json:"created,omitempty"`
	Updated     string       `json:"updated,omitempty"`
	Comments    []CommentOut `json:"comments,omitempty"`
}

// ListTransitionsInput requests the transitions available on an issue.
type ListTransitionsInput struct {
	Key string `json:"key" jsonschema:"issue key, e.g. PROJ-12"`
}

// TransitionOut is one available workflow transition.
type TransitionOut struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ToStatus string `json:"to_status"`
}

// ListTransitionsOutput is jira_list_transitions' structured result.
type ListTransitionsOutput struct {
	Transitions []TransitionOut `json:"transitions"`
}

// GetBacklogInput requests a board's backlog.
type GetBacklogInput struct {
	BoardID int `json:"board_id,omitempty" jsonschema:"board ID, defaults to the server's default board"`
	Limit   int `json:"limit,omitempty" jsonschema:"max issues to return, default 50"`
}

// GetBacklogOutput is jira_get_backlog's structured result.
type GetBacklogOutput struct {
	Issues  []IssueSummary `json:"issues"`
	Total   int            `json:"total"`
	HasMore bool           `json:"has_more"`
}

// ListSprintsInput requests a board's sprints.
type ListSprintsInput struct {
	BoardID       int  `json:"board_id,omitempty" jsonschema:"board ID, defaults to the server's default board"`
	IncludeClosed bool `json:"include_closed,omitempty" jsonschema:"also include closed sprints (default: only active and future)"`
}

// SprintOut is one board sprint.
type SprintOut struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
	Goal      string `json:"goal,omitempty"`
}

// ListSprintsOutput is jira_list_sprints' structured result.
type ListSprintsOutput struct {
	Sprints []SprintOut `json:"sprints"`
}

// ListEpicsInput requests a board's epics.
type ListEpicsInput struct {
	BoardID int `json:"board_id,omitempty" jsonschema:"board ID, defaults to the server's default board"`
}

// EpicOut is one epic.
type EpicOut struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
	Done    bool   `json:"done"`
}

// ListEpicsOutput is jira_list_epics' structured result.
type ListEpicsOutput struct {
	Epics []EpicOut `json:"epics"`
}

// --- Write tool input/output types --------------------------------------

// CreateIssueInput creates a new issue.
type CreateIssueInput struct {
	Project             string   `json:"project,omitempty" jsonschema:"project key, defaults to the server's default project"`
	Type                string   `json:"type" jsonschema:"issue type name, e.g. Task, Story, Bug, Epic"`
	Summary             string   `json:"summary" jsonschema:"issue summary/title"`
	DescriptionMarkdown string   `json:"description_markdown,omitempty" jsonschema:"issue description, as Markdown"`
	Epic                string   `json:"epic,omitempty" jsonschema:"parent epic issue key, e.g. PROJ-3 (team-managed projects link epics via parent)"`
	Assignee            string   `json:"assignee,omitempty" jsonschema:"\"me\", \"unassigned\", an email address, or a display name"`
	Labels              []string `json:"labels,omitempty" jsonschema:"labels to set on the issue"`
	Priority            string   `json:"priority,omitempty" jsonschema:"priority name, e.g. High, Medium, Low"`
}

// IssueRefOutput identifies a created issue.
type IssueRefOutput struct {
	Key  string `json:"key"`
	Self string `json:"self,omitempty"`
}

// UpdateIssueInput edits an existing issue. A nil/omitted field is left
// unchanged; only fields explicitly present in the call are sent to Jira.
type UpdateIssueInput struct {
	Key                 string    `json:"key" jsonschema:"issue key, e.g. PROJ-12"`
	Summary             *string   `json:"summary,omitempty" jsonschema:"new summary/title"`
	DescriptionMarkdown *string   `json:"description_markdown,omitempty" jsonschema:"new description, as Markdown"`
	Labels              *[]string `json:"labels,omitempty" jsonschema:"replace the issue's labels with this set (empty array clears them)"`
	Priority            *string   `json:"priority,omitempty" jsonschema:"new priority name"`
	Epic                *string   `json:"epic,omitempty" jsonschema:"new parent epic issue key"`
	Assignee            *string   `json:"assignee,omitempty" jsonschema:"\"me\", \"unassigned\", an email address, or a display name"`
}

// UpdateIssueOutput confirms an edit.
type UpdateIssueOutput struct {
	Key     string `json:"key"`
	Updated bool   `json:"updated"`
}

// TransitionIssueInput moves an issue along its workflow.
type TransitionIssueInput struct {
	Key      string `json:"key" jsonschema:"issue key, e.g. PROJ-12"`
	ToStatus string `json:"to_status" jsonschema:"destination status name (e.g. \"In Progress\") or transition name (e.g. \"Start Progress\")"`
}

// TransitionIssueOutput reports the issue's new status.
type TransitionIssueOutput struct {
	Key    string `json:"key"`
	Status string `json:"status"`
}

// AssignIssueInput reassigns an issue.
type AssignIssueInput struct {
	Key      string `json:"key" jsonschema:"issue key, e.g. PROJ-12"`
	Assignee string `json:"assignee" jsonschema:"\"me\", \"unassigned\", an email address, or a display name"`
}

// AssignIssueOutput confirms an assignment.
type AssignIssueOutput struct {
	Key      string `json:"key"`
	Assignee string `json:"assignee"`
}

// AddCommentInput posts a comment.
type AddCommentInput struct {
	Key          string `json:"key" jsonschema:"issue key, e.g. PROJ-12"`
	BodyMarkdown string `json:"body_markdown" jsonschema:"comment text, as Markdown"`
}

// AddCommentOutput identifies the created comment.
type AddCommentOutput struct {
	Key       string `json:"key"`
	CommentID string `json:"comment_id"`
}

// MoveToSprintInput moves issues into a sprint.
type MoveToSprintInput struct {
	Sprint  string   `json:"sprint" jsonschema:"\"current\", \"next\", a sprint ID, or a sprint name"`
	Keys    []string `json:"keys" jsonschema:"issue keys to move, at most 20"`
	BoardID int      `json:"board_id,omitempty" jsonschema:"board ID, defaults to the server's default board"`
}

// MoveToSprintOutput reports the resolved sprint and moved keys.
type MoveToSprintOutput struct {
	SprintID   int      `json:"sprint_id"`
	SprintName string   `json:"sprint_name"`
	Keys       []string `json:"keys"`
}

// MoveToBacklogInput moves issues back to the backlog.
type MoveToBacklogInput struct {
	Keys []string `json:"keys" jsonschema:"issue keys to move, at most 20"`
}

// MoveToBacklogOutput confirms the move.
type MoveToBacklogOutput struct {
	Keys []string `json:"keys"`
}

// --- Shared rendering helpers --------------------------------------------

func issueSummary(iss jira.Issue) IssueSummary {
	s := IssueSummary{
		Key:     iss.Key,
		Summary: iss.Fields.Summary,
		Status:  iss.Fields.Status.Name,
		Type:    iss.Fields.IssueType.Name,
		Created: iss.Fields.Created,
		Updated: iss.Fields.Updated,
	}
	if iss.Fields.Assignee != nil {
		s.Assignee = iss.Fields.Assignee.DisplayName
	}
	if iss.Fields.Parent != nil {
		s.Epic = iss.Fields.Parent.Key
	}
	return s
}

func issueSummaries(issues []jira.Issue) []IssueSummary {
	out := make([]IssueSummary, len(issues))
	for i, iss := range issues {
		out[i] = issueSummary(iss)
	}
	return out
}

func assigneeName(u *jira.User) string {
	if u == nil {
		return ""
	}
	return u.DisplayName
}
