// Package service holds gira's use cases and safety guardrails. Both the
// CLI (internal/cli) and, eventually, the MCP server call only into this
// package; internal/jira is never called directly from either front end,
// so behavior stays identical across both.
package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/rafaelescrich/gira/internal/jira"
)

// Options configures a Service's guardrails and defaults. It mirrors the
// relevant fields of internal/config.Config.
type Options struct {
	AllowedProjects []string
	DefaultProject  string
	DefaultBoard    int
	ReadOnly        bool
}

// Service is the concrete implementation of gira's use cases. Methods are
// free to live in additional files in this package (e.g. write.go,
// agile.go, comments.go) as new capabilities are added; all of them should
// share this struct and its guardrail helpers (checkKeyAllowed,
// checkProjectAllowed, requireWritable) rather than re-implementing safety
// checks.
type Service struct {
	// Client is the underlying Jira REST client. It's exported so
	// additional service files can call newly-added jira.Client methods
	// without needing changes here.
	Client *jira.Client

	Options
}

// New builds a Service around client with the given guardrail options.
func New(client *jira.Client, opts Options) *Service {
	if len(opts.AllowedProjects) == 0 && opts.DefaultProject != "" {
		opts.AllowedProjects = []string{opts.DefaultProject}
	}
	return &Service{Client: client, Options: opts}
}

// Me returns the authenticated user (GET /rest/api/3/myself). Also used by
// `gira auth status` to validate the stored token.
func (s *Service) Me(ctx context.Context) (jira.User, error) {
	u, err := s.Client.Myself(ctx)
	if err != nil {
		return jira.User{}, wrapAPIError(err)
	}
	return u, nil
}

// GetIssue fetches a single issue by key, after checking it against
// AllowedProjects.
func (s *Service) GetIssue(ctx context.Context, key string) (jira.Issue, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if key == "" {
		return jira.Issue{}, newUsageError("issue key is required")
	}
	if err := s.checkKeyAllowed(key); err != nil {
		return jira.Issue{}, err
	}
	issue, err := s.Client.GetIssue(ctx, key, nil)
	if err != nil {
		return jira.Issue{}, wrapAPIError(err)
	}
	return issue, nil
}

// --- Guardrails -------------------------------------------------------

// requireWritable returns a typed guardrail error when the service is
// configured read-only. Every mutating method (added by other files in
// this package) should call this first, before making any Jira request.
func (s *Service) requireWritable() error {
	if s.ReadOnly {
		return newGuardrailError("gira is in read-only mode; refusing write operation")
	}
	return nil
}

// checkProjectAllowed returns a guardrail error if project is not in
// AllowedProjects (case-insensitive).
func (s *Service) checkProjectAllowed(project string) error {
	for _, p := range s.AllowedProjects {
		if strings.EqualFold(p, project) {
			return nil
		}
	}
	return newGuardrailError("project %q is not in the allowed list %v", project, s.AllowedProjects)
}

// checkKeyAllowed extracts the project key prefix from an issue key
// ("PROJ-12" -> "AI") and checks it against AllowedProjects.
func (s *Service) checkKeyAllowed(key string) error {
	project, err := projectFromKey(key)
	if err != nil {
		return newUsageError("%s", err.Error())
	}
	return s.checkProjectAllowed(project)
}

// issueKeyRe matches a well-formed Jira issue key: a project key (a letter
// followed by letters/digits) and a numeric issue number, e.g. "PROJ-12".
// Enforcing this shape here — the single choke point every service method
// uses before touching the network (see checkKeyAllowed) — matters beyond
// cosmetics: internal/jira splices the key straight into a URL path
// (e.g. "/rest/api/3/issue/"+key+"/transitions") without escaping it, so a
// key containing "/" or ".." could otherwise reach a different REST
// endpoint entirely, bypassing the allowlist check below.
var issueKeyRe = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9]+$`)

func projectFromKey(key string) (string, error) {
	if !issueKeyRe.MatchString(key) {
		return "", fmt.Errorf("invalid issue key %q (expected e.g. PROJ-12)", key)
	}
	idx := strings.LastIndex(key, "-")
	return key[:idx], nil
}
