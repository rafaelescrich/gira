package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rafaelescrich/gira/internal/jira"
)

// CreateParams describes a new issue. Project defaults to DefaultProject
// when empty. Epic is a parent epic key (on team-managed projects
// epics link via fields.parent rather than a separate "epic link" field).
type CreateParams struct {
	Project             string
	Type                string
	Summary             string
	DescriptionMarkdown string
	Epic                string
	Assignee            string // me | unassigned | email | display name
	Labels              []string
	Priority            string
}

// UpdateParams describes an issue edit. A nil field means "leave
// unchanged"; a non-nil field (including a pointer to an empty string or
// empty slice) is sent as an explicit change.
type UpdateParams struct {
	Summary             *string
	DescriptionMarkdown *string
	Labels              *[]string
	Priority            *string
	Epic                *string
	Assignee            *string // me | unassigned | email | display name
}

// CreateIssue creates an issue (POST /rest/api/3/issue), after the
// read-only and project-allowlist guardrails, resolving the issue type
// name and assignee against Jira.
func (s *Service) CreateIssue(ctx context.Context, p CreateParams) (jira.IssueRef, error) {
	if err := s.requireWritable(); err != nil {
		return jira.IssueRef{}, err
	}

	project := strings.TrimSpace(p.Project)
	if project == "" {
		project = s.DefaultProject
	}
	if err := s.checkProjectAllowed(project); err != nil {
		return jira.IssueRef{}, err
	}

	summary := strings.TrimSpace(p.Summary)
	if summary == "" {
		return jira.IssueRef{}, newUsageError("summary is required")
	}
	typeName := strings.TrimSpace(p.Type)
	if typeName == "" {
		return jira.IssueRef{}, newUsageError("issue type is required")
	}

	resolvedType, err := s.resolveIssueType(ctx, project, typeName)
	if err != nil {
		return jira.IssueRef{}, err
	}

	fields := jira.IssueWriteFields{
		Project:   &jira.Project{Key: project},
		IssueType: &jira.IssueTypeRef{Name: resolvedType},
		Summary:   &summary,
	}
	if desc := p.DescriptionMarkdown; strings.TrimSpace(desc) != "" {
		doc := jira.MarkdownToADF(desc)
		fields.Description = &doc
	}
	if epic := strings.ToUpper(strings.TrimSpace(p.Epic)); epic != "" {
		fields.Parent = &jira.ParentRef{Key: epic}
	}
	if len(p.Labels) > 0 {
		labels := append([]string(nil), p.Labels...)
		fields.Labels = &labels
	}
	if priority := strings.TrimSpace(p.Priority); priority != "" {
		fields.Priority = &jira.Priority{Name: priority}
	}
	if assignee := strings.TrimSpace(p.Assignee); assignee != "" {
		accountID, err := s.resolveAssignee(ctx, assignee)
		if err != nil {
			return jira.IssueRef{}, err
		}
		fields.Assignee = &jira.AssigneeRef{AccountID: accountID}
	}

	ref, err := s.Client.CreateIssue(ctx, fields)
	if err != nil {
		return jira.IssueRef{}, wrapAPIError(err)
	}
	return ref, nil
}

// UpdateIssue edits an issue (PUT /rest/api/3/issue/{key}), sending only
// the fields explicitly set in p.
func (s *Service) UpdateIssue(ctx context.Context, key string, p UpdateParams) error {
	if err := s.requireWritable(); err != nil {
		return err
	}
	key = strings.ToUpper(strings.TrimSpace(key))
	if err := s.checkKeyAllowed(key); err != nil {
		return err
	}

	var fields jira.IssueWriteFields
	changed := false

	if p.Summary != nil {
		summary := strings.TrimSpace(*p.Summary)
		if summary == "" {
			return newUsageError("summary cannot be empty")
		}
		fields.Summary = &summary
		changed = true
	}
	if p.DescriptionMarkdown != nil {
		doc := jira.MarkdownToADF(*p.DescriptionMarkdown)
		fields.Description = &doc
		changed = true
	}
	if p.Labels != nil {
		labels := append([]string(nil), (*p.Labels)...)
		fields.Labels = &labels
		changed = true
	}
	if p.Priority != nil {
		priority := strings.TrimSpace(*p.Priority)
		if priority == "" {
			return newUsageError("priority cannot be empty")
		}
		fields.Priority = &jira.Priority{Name: priority}
		changed = true
	}
	if p.Epic != nil {
		epic := strings.ToUpper(strings.TrimSpace(*p.Epic))
		if epic == "" {
			return newUsageError("epic key cannot be empty")
		}
		fields.Parent = &jira.ParentRef{Key: epic}
		changed = true
	}
	if p.Assignee != nil {
		accountID, err := s.resolveAssignee(ctx, *p.Assignee)
		if err != nil {
			return err
		}
		fields.Assignee = &jira.AssigneeRef{AccountID: accountID}
		changed = true
	}

	if !changed {
		return newUsageError("no fields to update")
	}

	if err := s.Client.EditIssue(ctx, key, fields); err != nil {
		return wrapAPIError(err)
	}
	return nil
}

// ListTransitions returns the transitions available from key's current
// status (read-only; still subject to the project allowlist).
func (s *Service) ListTransitions(ctx context.Context, key string) ([]jira.Transition, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if err := s.checkKeyAllowed(key); err != nil {
		return nil, err
	}
	return s.cachedTransitions(ctx, key)
}

// Transition moves key to target, where target may be either a transition
// name (the workflow action, e.g. "Start Progress") or a destination
// status name (e.g. "In Progress"), matched case-insensitively. On no
// match (or an ambiguous one) the error lists the valid options, e.g.
// `Status "Doing" not found. Valid transitions from "To Do": In Progress, Blocked.`
func (s *Service) Transition(ctx context.Context, key, target string) (jira.Status, error) {
	if err := s.requireWritable(); err != nil {
		return jira.Status{}, err
	}
	key = strings.ToUpper(strings.TrimSpace(key))
	if err := s.checkKeyAllowed(key); err != nil {
		return jira.Status{}, err
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return jira.Status{}, newUsageError("target status or transition name is required")
	}

	transitions, err := s.cachedTransitions(ctx, key)
	if err != nil {
		return jira.Status{}, err
	}

	match, err := matchTransition(transitions, target)
	if err != nil {
		current := s.currentStatusName(ctx, key)
		options := transitionOptionNames(transitions)
		return jira.Status{}, newUsageError(
			"Status %q not found. Valid transitions from %q: %s",
			target, current, strings.Join(options, ", "),
		)
	}

	if err := s.Client.DoTransition(ctx, key, match.ID); err != nil {
		return jira.Status{}, wrapAPIError(err)
	}
	s.invalidateTransitions(key)
	return match.To, nil
}

// matchTransition finds the transition matching target by transition name
// first (exact case-insensitive match), falling back to the destination
// status name. Returns an error if there is no match or more than one.
func matchTransition(transitions []jira.Transition, target string) (jira.Transition, error) {
	var byName, byStatus []jira.Transition
	for _, t := range transitions {
		if strings.EqualFold(t.Name, target) {
			byName = append(byName, t)
		}
		if strings.EqualFold(t.To.Name, target) {
			byStatus = append(byStatus, t)
		}
	}
	switch {
	case len(byName) == 1:
		return byName[0], nil
	case len(byName) == 0 && len(byStatus) == 1:
		return byStatus[0], nil
	default:
		return jira.Transition{}, fmt.Errorf("no unambiguous match for %q", target)
	}
}

func transitionOptionNames(transitions []jira.Transition) []string {
	seen := map[string]bool{}
	var names []string
	for _, t := range transitions {
		if !seen[t.To.Name] {
			seen[t.To.Name] = true
			names = append(names, t.To.Name)
		}
	}
	return names
}

func (s *Service) currentStatusName(ctx context.Context, key string) string {
	issue, err := s.Client.GetIssue(ctx, key, []string{"status"})
	if err != nil || issue.Fields.Status.Name == "" {
		return "current status"
	}
	return issue.Fields.Status.Name
}

// Assign sets key's assignee. who is "me", "unassigned", an email address,
// or a display name; ambiguous name/email matches return a usage error
// listing the candidates.
func (s *Service) Assign(ctx context.Context, key, who string) error {
	if err := s.requireWritable(); err != nil {
		return err
	}
	key = strings.ToUpper(strings.TrimSpace(key))
	if err := s.checkKeyAllowed(key); err != nil {
		return err
	}
	who = strings.TrimSpace(who)
	if who == "" {
		return newUsageError("assignee is required (me, unassigned, email, or name)")
	}

	accountID, err := s.resolveAssignee(ctx, who)
	if err != nil {
		return err
	}
	if err := s.Client.Assign(ctx, key, accountID); err != nil {
		return wrapAPIError(err)
	}
	return nil
}

// resolveAssignee resolves who into an accountID (nil for "unassigned").
func (s *Service) resolveAssignee(ctx context.Context, who string) (*string, error) {
	switch strings.ToLower(strings.TrimSpace(who)) {
	case "unassigned":
		return nil, nil
	case "me":
		me, err := s.Client.Myself(ctx)
		if err != nil {
			return nil, wrapAPIError(err)
		}
		id := me.AccountID
		return &id, nil
	}

	users, err := s.cachedUserSearch(ctx, who)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	match, err := pickUser(users, who)
	if err != nil {
		return nil, err
	}
	id := match.AccountID
	return &id, nil
}

// pickUser resolves a Jira user-search result set to exactly one user,
// preferring active users and an exact case-insensitive email/display-name
// match when the search returns more than one candidate.
func pickUser(users []jira.User, query string) (jira.User, error) {
	candidates := users
	var active []jira.User
	for _, u := range users {
		if u.Active {
			active = append(active, u)
		}
	}
	if len(active) > 0 {
		candidates = active
	}

	if len(candidates) == 0 {
		return jira.User{}, newUsageError("no user found matching %q", query)
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}

	var exact []jira.User
	for _, u := range candidates {
		if strings.EqualFold(u.Email, query) || strings.EqualFold(u.DisplayName, query) {
			exact = append(exact, u)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}

	names := make([]string, len(candidates))
	for i, u := range candidates {
		names[i] = fmt.Sprintf("%s <%s>", u.DisplayName, u.Email)
	}
	return jira.User{}, newUsageError("%q matches multiple users: %s", query, strings.Join(names, "; "))
}

// resolveIssueType resolves name against project's createmeta issue types,
// case-insensitively, returning a usage error listing valid names on no
// match.
func (s *Service) resolveIssueType(ctx context.Context, project, name string) (string, error) {
	types, err := s.cachedCreateMetaIssueTypes(ctx, project)
	if err != nil {
		return "", wrapAPIError(err)
	}
	for _, t := range types {
		if strings.EqualFold(t.Name, name) {
			return t.Name, nil
		}
	}
	valid := make([]string, len(types))
	for i, t := range types {
		valid[i] = t.Name
	}
	return "", newUsageError("issue type %q not found in project %s. Valid types: %s", name, project, strings.Join(valid, ", "))
}

// --- Caching ------------------------------------------------------------
//
// The MCP server keeps one *Service alive for the process lifetime, so
// repeated lookups (transitions per issue, createmeta issue types per
// project, user search per query) are cached for cacheTTL. Caches are
// keyed off the *Service pointer (rather than a field on Service) so this
// file doesn't need to touch service.go's struct definition, which is
// edited concurrently by another agent working on the agile/* files; each
// live *Service still gets its own independent cache.

const cacheTTL = 5 * time.Minute

type cacheEntry[T any] struct {
	value   T
	expires time.Time
}

type writeCache struct {
	mu          sync.Mutex
	transitions map[string]cacheEntry[[]jira.Transition]
	createMeta  map[string]cacheEntry[[]jira.CreateMetaIssueType]
	users       map[string]cacheEntry[[]jira.User]
}

var (
	cachesMu sync.Mutex
	caches   = map[*Service]*writeCache{}
)

func (s *Service) writeCacheFor() *writeCache {
	cachesMu.Lock()
	defer cachesMu.Unlock()
	c, ok := caches[s]
	if !ok {
		c = &writeCache{
			transitions: map[string]cacheEntry[[]jira.Transition]{},
			createMeta:  map[string]cacheEntry[[]jira.CreateMetaIssueType]{},
			users:       map[string]cacheEntry[[]jira.User]{},
		}
		caches[s] = c
	}
	return c
}

func (s *Service) cachedTransitions(ctx context.Context, key string) ([]jira.Transition, error) {
	c := s.writeCacheFor()

	c.mu.Lock()
	if e, ok := c.transitions[key]; ok && time.Now().Before(e.expires) {
		c.mu.Unlock()
		return e.value, nil
	}
	c.mu.Unlock()

	transitions, err := s.Client.ListTransitions(ctx, key)
	if err != nil {
		return nil, wrapAPIError(err)
	}

	c.mu.Lock()
	c.transitions[key] = cacheEntry[[]jira.Transition]{value: transitions, expires: time.Now().Add(cacheTTL)}
	c.mu.Unlock()
	return transitions, nil
}

func (s *Service) invalidateTransitions(key string) {
	c := s.writeCacheFor()
	c.mu.Lock()
	delete(c.transitions, key)
	c.mu.Unlock()
}

func (s *Service) cachedCreateMetaIssueTypes(ctx context.Context, project string) ([]jira.CreateMetaIssueType, error) {
	c := s.writeCacheFor()

	c.mu.Lock()
	if e, ok := c.createMeta[project]; ok && time.Now().Before(e.expires) {
		c.mu.Unlock()
		return e.value, nil
	}
	c.mu.Unlock()

	types, err := s.Client.GetCreateMetaIssueTypes(ctx, project)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.createMeta[project] = cacheEntry[[]jira.CreateMetaIssueType]{value: types, expires: time.Now().Add(cacheTTL)}
	c.mu.Unlock()
	return types, nil
}

func (s *Service) cachedUserSearch(ctx context.Context, query string) ([]jira.User, error) {
	c := s.writeCacheFor()
	normalized := strings.ToLower(strings.TrimSpace(query))

	c.mu.Lock()
	if e, ok := c.users[normalized]; ok && time.Now().Before(e.expires) {
		c.mu.Unlock()
		return e.value, nil
	}
	c.mu.Unlock()

	users, err := s.Client.SearchUsers(ctx, query)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.users[normalized] = cacheEntry[[]jira.User]{value: users, expires: time.Now().Add(cacheTTL)}
	c.mu.Unlock()
	return users, nil
}
