package service

import (
	"errors"
	"fmt"

	"github.com/rafaelescrich/gira/internal/jira"
)

// ErrorKind classifies a service error so the CLI (and later the MCP
// server) can map it to a stable outcome — an exit code for the CLI, an
// isError/fix-it hint for MCP.
type ErrorKind int

const (
	// KindUnknown is the zero value; treated like KindUsage for exit-code
	// purposes but should not normally be constructed directly.
	KindUnknown ErrorKind = iota
	// KindUsage: bad input from the caller (missing/invalid arguments).
	KindUsage
	// KindAPI: the Jira API returned an error (excluding auth failures).
	KindAPI
	// KindAuth: the Jira API rejected our credentials (401).
	KindAuth
	// KindGuardrail: gira itself refused the request (allowlist, read-only).
	KindGuardrail
)

// Error is the typed error returned by Service methods.
type Error struct {
	Kind ErrorKind
	Msg  string
	Err  error // wrapped cause, if any (e.g. a *jira.APIError)
}

func (e *Error) Error() string {
	if e.Err != nil && e.Err.Error() != e.Msg {
		return e.Msg
	}
	return e.Msg
}

// Unwrap allows errors.As/errors.Is to see through to the wrapped cause
// (e.g. a *jira.APIError).
func (e *Error) Unwrap() error { return e.Err }

func newUsageError(format string, args ...any) error {
	return &Error{Kind: KindUsage, Msg: fmt.Sprintf(format, args...)}
}

func newGuardrailError(format string, args ...any) error {
	return &Error{Kind: KindGuardrail, Msg: fmt.Sprintf(format, args...)}
}

// wrapAPIError classifies an error returned by internal/jira into a typed
// *Error, distinguishing auth failures (401) from other API errors.
func wrapAPIError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *jira.APIError
	if errors.As(err, &apiErr) {
		if apiErr.IsUnauthorized() {
			return &Error{Kind: KindAuth, Msg: apiErr.Error(), Err: err}
		}
		return &Error{Kind: KindAPI, Msg: apiErr.Error(), Err: err}
	}
	return &Error{Kind: KindAPI, Msg: err.Error(), Err: err}
}

// ExitCode maps a service error to gira's process exit code convention:
// 0 ok, 1 usage, 2 API, 3 auth, 4 guardrail. Non-service errors (or nil)
// map to 0/1 as appropriate.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var svcErr *Error
	if errors.As(err, &svcErr) {
		switch svcErr.Kind {
		case KindUsage, KindUnknown:
			return 1
		case KindAPI:
			return 2
		case KindAuth:
			return 3
		case KindGuardrail:
			return 4
		}
	}
	return 1
}
