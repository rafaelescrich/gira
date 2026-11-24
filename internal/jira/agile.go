package jira

import (
	"context"
	"fmt"
)

// This file covers the Agile API 1.0 (boards, sprints, backlog, epics):
// blueprint §4. Like issues.go, it owns HTTP/paging concerns only; safety
// checks (allowlist, read-only, batch caps for MCP/CLI) live in
// internal/service.

const (
	// MaxAgileResults is the hard cap on items returned by the *All
	// convenience methods below, across all pages, mirroring
	// MaxSearchResults for the platform search API.
	MaxAgileResults = 200
	// DefaultAgileLimit is used when the caller doesn't specify a limit.
	DefaultAgileLimit = 50
	// maxMoveIssuesPerCall is Jira's own limit on how many issues may be
	// included in a single move-to-sprint/move-to-backlog request; larger
	// key lists are chunked transparently.
	maxMoveIssuesPerCall = 50
)

func clampAgileLimit(n int) int {
	if n <= 0 {
		return DefaultAgileLimit
	}
	if n > MaxAgileResults {
		return MaxAgileResults
	}
	return n
}

// GetBoard calls GET /rest/agile/1.0/board/{id}.
func (c *Client) GetBoard(ctx context.Context, boardID int) (Board, error) {
	var b Board
	if err := c.get(ctx, fmt.Sprintf("/rest/agile/1.0/board/%d", boardID), &b); err != nil {
		return Board{}, err
	}
	return b, nil
}

// GetBoardConfiguration calls GET /rest/agile/1.0/board/{id}/configuration,
// which reports the board's type and project even for team-managed boards.
func (c *Client) GetBoardConfiguration(ctx context.Context, boardID int) (BoardConfiguration, error) {
	var cfg BoardConfiguration
	if err := c.get(ctx, fmt.Sprintf("/rest/agile/1.0/board/%d/configuration", boardID), &cfg); err != nil {
		return BoardConfiguration{}, err
	}
	return cfg, nil
}

// Backlog runs a single page of GET /rest/agile/1.0/board/{id}/backlog.
// maxResults is clamped to [1, MaxAgileResults]. Returns the page's issues
// plus the server-reported total (isLast isn't reported by this endpoint;
// callers compare startAt+len(issues) to total, as BacklogAll does).
func (c *Client) Backlog(ctx context.Context, boardID, startAt, maxResults int) ([]Issue, int, error) {
	maxResults = clampAgileLimit(maxResults)
	path := fmt.Sprintf("/rest/agile/1.0/board/%d/backlog?startAt=%d&maxResults=%d&fields=%s",
		boardID, startAt, maxResults, joinFields(DefaultSearchFields))
	var resp backlogResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, 0, err
	}
	return resp.Issues, resp.Total, nil
}

// BacklogAll pages through Backlog until the server reports no more
// issues or limit (clamped to MaxAgileResults) is reached, returning the
// combined issues plus the server-reported total.
func (c *Client) BacklogAll(ctx context.Context, boardID, limit int) ([]Issue, int, error) {
	limit = clampAgileLimit(limit)

	var all []Issue
	total := 0
	startAt := 0
	for len(all) < limit {
		page, pageTotal, err := c.Backlog(ctx, boardID, startAt, limit-len(all))
		if err != nil {
			return nil, 0, err
		}
		all = append(all, page...)
		total = pageTotal
		if len(page) == 0 || startAt+len(page) >= pageTotal {
			break
		}
		startAt += len(page)
	}
	return all, total, nil
}

// Sprints runs a single page of GET /rest/agile/1.0/board/{id}/sprint.
// states, if non-empty, is sent as the comma-separated `state` filter
// (e.g. "active,future"). maxResults is clamped to [1, MaxAgileResults].
func (c *Client) Sprints(ctx context.Context, boardID, startAt, maxResults int, states []string) ([]Sprint, int, bool, error) {
	maxResults = clampAgileLimit(maxResults)
	path := fmt.Sprintf("/rest/agile/1.0/board/%d/sprint?startAt=%d&maxResults=%d", boardID, startAt, maxResults)
	if len(states) > 0 {
		path += "&state=" + joinFields(states)
	}
	var resp sprintListResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, 0, false, err
	}
	return resp.Values, resp.Total, resp.IsLast, nil
}

// SprintsAll pages through Sprints until isLast or limit (clamped to
// MaxAgileResults) is reached.
func (c *Client) SprintsAll(ctx context.Context, boardID int, states []string, limit int) ([]Sprint, int, error) {
	limit = clampAgileLimit(limit)

	var all []Sprint
	total := 0
	startAt := 0
	for len(all) < limit {
		page, pageTotal, isLast, err := c.Sprints(ctx, boardID, startAt, limit-len(all), states)
		if err != nil {
			return nil, 0, err
		}
		all = append(all, page...)
		total = pageTotal
		if isLast || len(page) == 0 {
			break
		}
		startAt += len(page)
	}
	return all, total, nil
}

// SprintIssues runs a single page of GET /rest/agile/1.0/sprint/{id}/issue.
func (c *Client) SprintIssues(ctx context.Context, sprintID, startAt, maxResults int) ([]Issue, int, error) {
	maxResults = clampAgileLimit(maxResults)
	path := fmt.Sprintf("/rest/agile/1.0/sprint/%d/issue?startAt=%d&maxResults=%d&fields=%s",
		sprintID, startAt, maxResults, joinFields(DefaultSearchFields))
	var resp backlogResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, 0, err
	}
	return resp.Issues, resp.Total, nil
}

// SprintIssuesAll pages through SprintIssues until exhausted or limit
// (clamped to MaxAgileResults) is reached.
func (c *Client) SprintIssuesAll(ctx context.Context, sprintID, limit int) ([]Issue, int, error) {
	limit = clampAgileLimit(limit)

	var all []Issue
	total := 0
	startAt := 0
	for len(all) < limit {
		page, pageTotal, err := c.SprintIssues(ctx, sprintID, startAt, limit-len(all))
		if err != nil {
			return nil, 0, err
		}
		all = append(all, page...)
		total = pageTotal
		if len(page) == 0 || startAt+len(page) >= pageTotal {
			break
		}
		startAt += len(page)
	}
	return all, total, nil
}

// MoveToSprint calls POST /rest/agile/1.0/sprint/{id}/issue to move keys
// into sprintID. Jira caps this endpoint at 50 issues per call; larger key
// lists are chunked into multiple sequential requests transparently.
func (c *Client) MoveToSprint(ctx context.Context, sprintID int, keys []string) error {
	if sprintID <= 0 {
		return fmt.Errorf("jira: sprint id is required")
	}
	for _, chunk := range chunkStrings(keys, maxMoveIssuesPerCall) {
		if err := c.post(ctx, fmt.Sprintf("/rest/agile/1.0/sprint/%d/issue", sprintID), moveIssuesRequest{Issues: chunk}, nil); err != nil {
			return err
		}
	}
	return nil
}

// MoveToBacklog calls POST /rest/agile/1.0/backlog/issue to move keys back
// to the backlog (out of whatever sprint they're in), chunking into
// batches of at most 50 as MoveToSprint does.
func (c *Client) MoveToBacklog(ctx context.Context, keys []string) error {
	for _, chunk := range chunkStrings(keys, maxMoveIssuesPerCall) {
		if err := c.post(ctx, "/rest/agile/1.0/backlog/issue", moveIssuesRequest{Issues: chunk}, nil); err != nil {
			return err
		}
	}
	return nil
}

// BoardEpics runs a single page of GET /rest/agile/1.0/board/{id}/epic.
// Note: this endpoint is documented for company-managed boards; on a
// team-managed ("simple") board it may error or return an empty set, in
// which case internal/service falls back to a JQL search for issuetype =
// Epic (some boards reject it).
func (c *Client) BoardEpics(ctx context.Context, boardID, startAt, maxResults int) ([]Epic, int, bool, error) {
	maxResults = clampAgileLimit(maxResults)
	path := fmt.Sprintf("/rest/agile/1.0/board/%d/epic?startAt=%d&maxResults=%d", boardID, startAt, maxResults)
	var resp epicListResponse
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, 0, false, err
	}
	return resp.Values, resp.Total, resp.IsLast, nil
}

// BoardEpicsAll pages through BoardEpics until isLast or limit (clamped to
// MaxAgileResults) is reached.
func (c *Client) BoardEpicsAll(ctx context.Context, boardID, limit int) ([]Epic, int, error) {
	limit = clampAgileLimit(limit)

	var all []Epic
	total := 0
	startAt := 0
	for len(all) < limit {
		page, pageTotal, isLast, err := c.BoardEpics(ctx, boardID, startAt, limit-len(all))
		if err != nil {
			return nil, 0, err
		}
		all = append(all, page...)
		total = pageTotal
		if isLast || len(page) == 0 {
			break
		}
		startAt += len(page)
	}
	return all, total, nil
}

// chunkStrings splits keys into slices of at most size elements each,
// dropping empty entries. size <= 0 is treated as "no chunking".
func chunkStrings(keys []string, size int) [][]string {
	var filtered []string
	for _, k := range keys {
		if k != "" {
			filtered = append(filtered, k)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	if size <= 0 || size >= len(filtered) {
		return [][]string{filtered}
	}
	var chunks [][]string
	for i := 0; i < len(filtered); i += size {
		end := i + size
		if end > len(filtered) {
			end = len(filtered)
		}
		chunks = append(chunks, filtered[i:end])
	}
	return chunks
}
