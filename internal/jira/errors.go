package jira

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// APIError represents a parsed error response from the Jira REST API.
// Jira error bodies look like:
//
//	{"errorMessages": ["..."], "errors": {"summary": "is required"}}
//
// Field-level errors are surfaced verbatim so an agent (human or MCP) can
// fix its next call.
type APIError struct {
	StatusCode    int
	ErrorMessages []string
	Errors        map[string]string
	// raw holds the unparsed body for debugging when parsing fails; it is
	// never printed since it may (rarely) echo request data, but is kept
	// available via RawBody() if a caller needs it. It never contains the
	// Authorization header.
	raw string
}

func (e *APIError) Error() string {
	if e.StatusCode == http.StatusUnauthorized {
		return "jira: authentication failed (401) — token expired or revoked; run `gira auth login`"
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("jira: request failed (%d)", e.StatusCode))
	if len(e.ErrorMessages) > 0 {
		parts = append(parts, strings.Join(e.ErrorMessages, "; "))
	}
	for field, msg := range e.Errors {
		parts = append(parts, fmt.Sprintf("%s: %s", field, msg))
	}
	if len(parts) == 1 && e.raw != "" {
		parts = append(parts, e.raw)
	}
	return strings.Join(parts, ": ")
}

// RawBody returns the unparsed response body, if any.
func (e *APIError) RawBody() string { return e.raw }

// IsNotFound reports whether the error represents a 404.
func (e *APIError) IsNotFound() bool { return e.StatusCode == http.StatusNotFound }

// IsUnauthorized reports whether the error represents a 401 (bad/expired
// token).
func (e *APIError) IsUnauthorized() bool { return e.StatusCode == http.StatusUnauthorized }

// IsForbidden reports whether the error represents a 403 (permission
// denied by Jira, distinct from gira's own guardrails).
func (e *APIError) IsForbidden() bool { return e.StatusCode == http.StatusForbidden }

type errorBody struct {
	ErrorMessages []string          `json:"errorMessages"`
	Errors        map[string]string `json:"errors"`
}

func parseAPIError(status int, body []byte) *APIError {
	apiErr := &APIError{StatusCode: status}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return apiErr
	}

	var parsed errorBody
	if err := json.Unmarshal(body, &parsed); err == nil {
		apiErr.ErrorMessages = parsed.ErrorMessages
		apiErr.Errors = parsed.Errors
	}

	// Keep the raw body only if we couldn't extract anything structured,
	// so callers/messages still have something useful to show.
	if len(apiErr.ErrorMessages) == 0 && len(apiErr.Errors) == 0 {
		apiErr.raw = trimmed
	}

	return apiErr
}
