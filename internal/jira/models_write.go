package jira

// Model types for the write-path client calls (issues_write.go,
// transitions.go, comments.go, users.go). Kept in their own file per the
// package's convention of not overloading models.go.

// IssueRef is the minimal issue reference returned by CreateIssue.
type IssueRef struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self,omitempty"`
}

// IssueTypeRef names an issue type by name only, as accepted by the create
// and edit issue endpoints (no need to resolve an ID).
type IssueTypeRef struct {
	Name string `json:"name"`
}

// AssigneeRef is the "assignee" field payload for create/edit and the body
// of the assign-issue endpoint. AccountID has no `omitempty` so an explicit
// nil marshals to `"accountId": null`, which is how Jira represents
// "unassigned" — distinct from omitting the field entirely (which means
// "leave unchanged" on edit).
type AssigneeRef struct {
	AccountID *string `json:"accountId"`
}

// IssueWriteFields is the "fields" payload shared by CreateIssue and
// EditIssue. Every field is a pointer so callers can distinguish "leave
// unchanged" (nil, edit only) from an explicit value. Labels is a pointer
// to a slice so an explicit empty slice can still clear existing labels.
type IssueWriteFields struct {
	Project     *Project      `json:"project,omitempty"`
	IssueType   *IssueTypeRef `json:"issuetype,omitempty"`
	Summary     *string       `json:"summary,omitempty"`
	Description *ADFDoc       `json:"description,omitempty"`
	Labels      *[]string     `json:"labels,omitempty"`
	Priority    *Priority     `json:"priority,omitempty"`
	Assignee    *AssigneeRef  `json:"assignee,omitempty"`
	Parent      *ParentRef    `json:"parent,omitempty"`
}

// CreateMetaIssueType is one entry from
// GET /rest/api/3/issue/createmeta/{project}/issuetypes.
type CreateMetaIssueType struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Subtask bool   `json:"subtask"`
}

// CreateMetaField is one entry from
// GET /rest/api/3/issue/createmeta/{project}/issuetypes/{issueTypeId}.
type CreateMetaField struct {
	FieldID  string `json:"fieldId"`
	Name     string `json:"name"`
	Required bool   `json:"required"`
}
