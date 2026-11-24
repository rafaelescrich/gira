package jira

// This file holds the request/response shapes for the Agile API 1.0
// (boards, sprints, backlog, epics). See agile.go for the client methods
// and BLUEPRINT.md §4 for the endpoint list.

// Board is a Jira Agile board (GET /rest/agile/1.0/board/{id}).
type Board struct {
	ID       int            `json:"id"`
	Name     string         `json:"name"`
	Type     string         `json:"type"` // "scrum", "kanban", or "simple" (team-managed)
	Location *BoardLocation `json:"location,omitempty"`
}

// BoardLocation identifies the project a board belongs to.
type BoardLocation struct {
	ProjectID   int    `json:"projectId,omitempty"`
	ProjectKey  string `json:"projectKey,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

// BoardConfiguration is the response from
// GET /rest/agile/1.0/board/{id}/configuration.
type BoardConfiguration struct {
	ID       int            `json:"id"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Location *BoardLocation `json:"location,omitempty"`
}

// Sprint is a board sprint (GET .../board/{id}/sprint, .../sprint/{id}).
type Sprint struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	State         string `json:"state"` // "active", "closed", or "future"
	StartDate     string `json:"startDate,omitempty"`
	EndDate       string `json:"endDate,omitempty"`
	CompleteDate  string `json:"completeDate,omitempty"`
	OriginBoardID int    `json:"originBoardId,omitempty"`
	Goal          string `json:"goal,omitempty"`
}

// Epic is an epic as returned by GET /rest/agile/1.0/board/{id}/epic.
type Epic struct {
	ID      int    `json:"id"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
	Done    bool   `json:"done"`
}

// backlogResponse is the raw shape of the board backlog and sprint-issue
// endpoints, which page with startAt/maxResults/total and do not report
// isLast directly.
type backlogResponse struct {
	StartAt    int     `json:"startAt"`
	MaxResults int     `json:"maxResults"`
	Total      int     `json:"total"`
	Issues     []Issue `json:"issues"`
}

// sprintListResponse is the raw shape of the board sprint-list endpoint,
// which does report isLast.
type sprintListResponse struct {
	StartAt    int      `json:"startAt"`
	MaxResults int      `json:"maxResults"`
	Total      int      `json:"total"`
	IsLast     bool     `json:"isLast"`
	Values     []Sprint `json:"values"`
}

// epicListResponse is the raw shape of the board epic-list endpoint, which
// also reports isLast.
type epicListResponse struct {
	StartAt    int    `json:"startAt"`
	MaxResults int    `json:"maxResults"`
	Total      int    `json:"total"`
	IsLast     bool   `json:"isLast"`
	Values     []Epic `json:"values"`
}

// moveIssuesRequest is the POST body for both the move-to-sprint and
// move-to-backlog endpoints.
type moveIssuesRequest struct {
	Issues []string `json:"issues"`
}
