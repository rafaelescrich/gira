package jira

import "encoding/json"

// User is a Jira Cloud user (account).
type User struct {
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
	Email       string `json:"emailAddress,omitempty"`
	Active      bool   `json:"active"`
}

// Status is a workflow status.
type Status struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Category StatusCategory `json:"statusCategory"`
}

// StatusCategory groups statuses (e.g. "To Do", "In Progress", "Done").
type StatusCategory struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// IssueType describes an issue's type (Task, Bug, Story, Epic, Subtask...).
type IssueType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Subtask bool   `json:"subtask"`
}

// Priority is an issue priority (High, Medium, Low, ...).
type Priority struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
}

// Project is a minimal project reference.
type Project struct {
	ID   string `json:"id,omitempty"`
	Key  string `json:"key"`
	Name string `json:"name,omitempty"`
}

// ParentRef references a parent issue (epic link for team-managed
// projects).
type ParentRef struct {
	ID  string `json:"id,omitempty"`
	Key string `json:"key,omitempty"`
}

// IssueFields is the subset of Jira issue fields gira reads and renders.
// The client always requests an explicit field list (see issues.go); this
// struct should stay in sync with that list.
type IssueFields struct {
	Summary     string          `json:"summary"`
	Description json.RawMessage `json:"description,omitempty"` // ADF
	Status      Status          `json:"status"`
	Assignee    *User           `json:"assignee"`
	Reporter    *User           `json:"reporter,omitempty"`
	Priority    *Priority       `json:"priority,omitempty"`
	IssueType   IssueType       `json:"issuetype"`
	Project     Project         `json:"project,omitempty"`
	Parent      *ParentRef      `json:"parent,omitempty"`
	Labels      []string        `json:"labels,omitempty"`
	Updated     string          `json:"updated,omitempty"`
	Created     string          `json:"created,omitempty"`
}

// Issue is a Jira issue as returned by the search and get-issue endpoints.
type Issue struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Self   string      `json:"self,omitempty"`
	Fields IssueFields `json:"fields"`
}

// IssuePage is one page of search results.
type IssuePage struct {
	Issues        []Issue `json:"issues"`
	NextPageToken string  `json:"nextPageToken,omitempty"`
	IsLast        bool    `json:"isLast"`
}

// searchRequest is the POST body for /rest/api/3/search/jql.
type searchRequest struct {
	JQL           string   `json:"jql"`
	Fields        []string `json:"fields"`
	MaxResults    int      `json:"maxResults"`
	NextPageToken string   `json:"nextPageToken,omitempty"`
}

// searchResponse is the raw response shape from /rest/api/3/search/jql.
type searchResponse struct {
	Issues        []Issue `json:"issues"`
	NextPageToken string  `json:"nextPageToken"`
	IsLast        bool    `json:"isLast"`
}

// Comment is a Jira issue comment.
type Comment struct {
	ID      string          `json:"id"`
	Author  *User           `json:"author,omitempty"`
	Body    json.RawMessage `json:"body,omitempty"` // ADF
	Created string          `json:"created,omitempty"`
	Updated string          `json:"updated,omitempty"`
}

// Transition is a workflow transition available on an issue.
type Transition struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	To   Status `json:"to"`
}
