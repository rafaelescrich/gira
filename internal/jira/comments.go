package jira

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type commentsResponse struct {
	Comments   []Comment `json:"comments"`
	StartAt    int       `json:"startAt"`
	MaxResults int       `json:"maxResults"`
	Total      int       `json:"total"`
}

// ListComments calls GET /rest/api/3/issue/{key}/comment.
func (c *Client) ListComments(ctx context.Context, key string) ([]Comment, error) {
	if key == "" {
		return nil, fmt.Errorf("jira: issue key is required")
	}
	var resp commentsResponse
	if err := c.get(ctx, "/rest/api/3/issue/"+url.PathEscape(key)+"/comment", &resp); err != nil {
		return nil, err
	}
	return resp.Comments, nil
}

type addCommentRequest struct {
	Body ADFDoc `json:"body"`
}

// AddComment calls POST /rest/api/3/issue/{key}/comment with an ADF body.
func (c *Client) AddComment(ctx context.Context, key string, body ADFDoc) (Comment, error) {
	if key == "" {
		return Comment{}, fmt.Errorf("jira: issue key is required")
	}
	req := addCommentRequest{Body: body}
	var resp Comment
	if err := c.doJSON(ctx, http.MethodPost, "/rest/api/3/issue/"+url.PathEscape(key)+"/comment", req, &resp); err != nil {
		return Comment{}, err
	}
	return resp, nil
}
