package jira

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// permissionsResponse is the raw shape of
// GET /rest/api/3/mypermissions?projectKey=...&permissions=....
type permissionsResponse struct {
	Permissions map[string]struct {
		ID             string `json:"id"`
		Key            string `json:"key"`
		Name           string `json:"name"`
		HavePermission bool   `json:"havePermission"`
	} `json:"permissions"`
}

// MyPermissions calls GET /rest/api/3/mypermissions, scoped to projectKey,
// for the given permission keys (e.g. "CREATE_ISSUES", "EDIT_ISSUES"),
// returning a map of permission key -> granted. Used by the MCP server
// (blueprint §7) to register only the write tools the current token is
// actually allowed to use.
func (c *Client) MyPermissions(ctx context.Context, projectKey string, permissions []string) (map[string]bool, error) {
	if projectKey == "" {
		return nil, fmt.Errorf("jira: project key is required")
	}
	if len(permissions) == 0 {
		return nil, fmt.Errorf("jira: at least one permission is required")
	}
	path := "/rest/api/3/mypermissions?projectKey=" + url.QueryEscape(projectKey) +
		"&permissions=" + url.QueryEscape(strings.Join(permissions, ","))

	var resp permissionsResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}

	out := make(map[string]bool, len(permissions))
	for _, key := range permissions {
		out[key] = resp.Permissions[key].HavePermission
	}
	return out, nil
}
