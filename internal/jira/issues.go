package jira

import (
	"context"
	"fmt"
	"net/url"
)

// DefaultSearchFields is the explicit field list requested for search
// results, kept small so responses stay cheap (blueprint §4).
var DefaultSearchFields = []string{
	"summary", "status", "assignee", "priority", "issuetype", "parent", "labels", "updated", "created",
}

// DefaultIssueFields is the field list requested for a single-issue fetch;
// it adds description, reporter and created on top of the search fields.
var DefaultIssueFields = []string{
	"summary", "description", "status", "assignee", "reporter", "priority",
	"issuetype", "parent", "labels", "updated", "created", "project",
}

// MaxSearchResults is the hard cap on issues returned by SearchJQL across
// all pages, regardless of what the caller asks for. It matches the
// blueprint's "max 200 issues" guardrail.
const MaxSearchResults = 200

// DefaultSearchLimit is used when the caller doesn't specify a limit.
const DefaultSearchLimit = 50

// Myself calls GET /rest/api/3/myself, returning the authenticated user.
// This is also the standard way to validate a token (a 401 means the
// token is invalid/expired).
func (c *Client) Myself(ctx context.Context) (User, error) {
	var u User
	if err := c.get(ctx, "/rest/api/3/myself", &u); err != nil {
		return User{}, err
	}
	return u, nil
}

// SearchJQL runs a single page of POST /rest/api/3/search/jql.
//
// jql is sent as-is (callers, i.e. internal/service, are responsible for
// safety/allowlisting). fields defaults to DefaultSearchFields when nil.
// maxResults is clamped to [1, MaxSearchResults]. pageToken continues a
// previous page (empty for the first page).
func (c *Client) SearchJQL(ctx context.Context, jql string, fields []string, maxResults int, pageToken string) (IssuePage, error) {
	if fields == nil {
		fields = DefaultSearchFields
	}
	if maxResults <= 0 {
		maxResults = DefaultSearchLimit
	}
	if maxResults > MaxSearchResults {
		maxResults = MaxSearchResults
	}

	req := searchRequest{
		JQL:           jql,
		Fields:        fields,
		MaxResults:    maxResults,
		NextPageToken: pageToken,
	}
	var resp searchResponse
	if err := c.post(ctx, "/rest/api/3/search/jql", req, &resp); err != nil {
		return IssuePage{}, err
	}
	return IssuePage(resp), nil
}

// SearchAll pages through SearchJQL until IsLast or until total collected
// issues reaches limit (clamped to MaxSearchResults). It exists for
// convenience; callers that want manual pagination should call SearchJQL
// directly.
func (c *Client) SearchAll(ctx context.Context, jql string, fields []string, limit int) ([]Issue, error) {
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxSearchResults {
		limit = MaxSearchResults
	}

	var all []Issue
	token := ""
	for {
		remaining := limit - len(all)
		if remaining <= 0 {
			break
		}
		pageSize := remaining
		page, err := c.SearchJQL(ctx, jql, fields, pageSize, token)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Issues...)
		if page.IsLast || page.NextPageToken == "" || len(page.Issues) == 0 {
			break
		}
		token = page.NextPageToken
	}
	return all, nil
}

// GetIssue calls GET /rest/api/3/issue/{key}, requesting an explicit field
// list to keep the response small. fields defaults to DefaultIssueFields
// when nil.
func (c *Client) GetIssue(ctx context.Context, key string, fields []string) (Issue, error) {
	if key == "" {
		return Issue{}, fmt.Errorf("jira: issue key is required")
	}
	if fields == nil {
		fields = DefaultIssueFields
	}

	path := fmt.Sprintf("/rest/api/3/issue/%s?fields=%s", url.PathEscape(key), joinFields(fields))
	var issue Issue
	if err := c.get(ctx, path, &issue); err != nil {
		return Issue{}, err
	}
	return issue, nil
}

func joinFields(fields []string) string {
	out := ""
	for i, f := range fields {
		if i > 0 {
			out += ","
		}
		out += f
	}
	return out
}
