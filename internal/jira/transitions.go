package jira

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

type transitionsResponse struct {
	Transitions []Transition `json:"transitions"`
}

// ListTransitions calls GET /rest/api/3/issue/{key}/transitions, returning
// the transitions available from the issue's current status.
func (c *Client) ListTransitions(ctx context.Context, key string) ([]Transition, error) {
	if key == "" {
		return nil, fmt.Errorf("jira: issue key is required")
	}
	var resp transitionsResponse
	if err := c.get(ctx, "/rest/api/3/issue/"+url.PathEscape(key)+"/transitions", &resp); err != nil {
		return nil, err
	}
	return resp.Transitions, nil
}

type doTransitionRequest struct {
	Transition transitionIDRef `json:"transition"`
}

type transitionIDRef struct {
	ID string `json:"id"`
}

// DoTransition calls POST /rest/api/3/issue/{key}/transitions with the
// given transition ID (as returned by ListTransitions).
func (c *Client) DoTransition(ctx context.Context, key, transitionID string) error {
	if key == "" || transitionID == "" {
		return fmt.Errorf("jira: issue key and transition id are required")
	}
	req := doTransitionRequest{Transition: transitionIDRef{ID: transitionID}}
	return c.doJSON(ctx, http.MethodPost, "/rest/api/3/issue/"+url.PathEscape(key)+"/transitions", req, nil)
}
