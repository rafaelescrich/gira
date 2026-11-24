package jira

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// Assign calls PUT /rest/api/3/issue/{key}/assignee. accountID nil means
// "unassigned" (sent as an explicit JSON null, per Jira's contract).
func (c *Client) Assign(ctx context.Context, key string, accountID *string) error {
	if key == "" {
		return fmt.Errorf("jira: issue key is required")
	}
	body := AssigneeRef{AccountID: accountID}
	return c.doJSON(ctx, http.MethodPut, "/rest/api/3/issue/"+url.PathEscape(key)+"/assignee", body, nil)
}

// SearchUsers calls GET /rest/api/3/user/search?query=..., matching an
// email address or display name substring.
func (c *Client) SearchUsers(ctx context.Context, query string) ([]User, error) {
	if query == "" {
		return nil, fmt.Errorf("jira: search query is required")
	}
	path := "/rest/api/3/user/search?query=" + url.QueryEscape(query)
	var users []User
	if err := c.get(ctx, path, &users); err != nil {
		return nil, err
	}
	return users, nil
}
