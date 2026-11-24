package jira

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type issueFieldsRequest struct {
	Fields IssueWriteFields `json:"fields"`
}

// CreateIssue calls POST /rest/api/3/issue. fields.Project, fields.IssueType
// and fields.Summary are required; the caller (internal/service) is
// responsible for validation and name resolution.
func (c *Client) CreateIssue(ctx context.Context, fields IssueWriteFields) (IssueRef, error) {
	if fields.Project == nil || fields.IssueType == nil || fields.Summary == nil {
		return IssueRef{}, fmt.Errorf("jira: project, issue type and summary are required to create an issue")
	}
	req := issueFieldsRequest{Fields: fields}
	var resp IssueRef
	if err := c.doJSON(ctx, http.MethodPost, "/rest/api/3/issue", req, &resp); err != nil {
		return IssueRef{}, err
	}
	return resp, nil
}

// EditIssue calls PUT /rest/api/3/issue/{key}. Only non-nil fields are
// sent, per IssueWriteFields' semantics. The endpoint returns 204 No
// Content on success.
func (c *Client) EditIssue(ctx context.Context, key string, fields IssueWriteFields) error {
	if key == "" {
		return fmt.Errorf("jira: issue key is required")
	}
	req := issueFieldsRequest{Fields: fields}
	return c.doJSON(ctx, http.MethodPut, "/rest/api/3/issue/"+url.PathEscape(key), req, nil)
}

type createMetaIssueTypesResponse struct {
	IssueTypes []CreateMetaIssueType `json:"issueTypes"`
}

// GetCreateMetaIssueTypes calls
// GET /rest/api/3/issue/createmeta/{project}/issuetypes.
func (c *Client) GetCreateMetaIssueTypes(ctx context.Context, project string) ([]CreateMetaIssueType, error) {
	if project == "" {
		return nil, fmt.Errorf("jira: project key is required")
	}
	var resp createMetaIssueTypesResponse
	path := fmt.Sprintf("/rest/api/3/issue/createmeta/%s/issuetypes", url.PathEscape(project))
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp.IssueTypes, nil
}

type createMetaFieldsResponse struct {
	Values     []CreateMetaField `json:"values"`
	IsLast     bool              `json:"isLast"`
	StartAt    int               `json:"startAt"`
	MaxResults int               `json:"maxResults"`
}

// createMetaFieldsPageCap bounds pagination so a misbehaving/mocked
// response can't loop forever.
const createMetaFieldsPageCap = 500

// GetCreateMetaFields calls
// GET /rest/api/3/issue/createmeta/{project}/issuetypes/{issueTypeId},
// paging through all fields for that issue type.
func (c *Client) GetCreateMetaFields(ctx context.Context, project, issueTypeID string) ([]CreateMetaField, error) {
	if project == "" || issueTypeID == "" {
		return nil, fmt.Errorf("jira: project key and issue type id are required")
	}
	base := fmt.Sprintf("/rest/api/3/issue/createmeta/%s/issuetypes/%s", url.PathEscape(project), url.PathEscape(issueTypeID))

	var all []CreateMetaField
	startAt := 0
	for {
		path := fmt.Sprintf("%s?startAt=%d", base, startAt)
		var resp createMetaFieldsResponse
		if err := c.get(ctx, path, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Values...)
		if resp.IsLast || len(resp.Values) == 0 || len(all) >= createMetaFieldsPageCap {
			break
		}
		startAt += len(resp.Values)
	}
	return all, nil
}
