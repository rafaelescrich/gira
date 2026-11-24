package service

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/rafaelescrich/gira/internal/jira"
)

// SearchParams describes a search either via structured filters (safely
// turned into quoted JQL) or via raw JQL, which gets wrapped in a project
// guard so it can never escape AllowedProjects.
type SearchParams struct {
	// Project restricts a structured search to one project; defaults to
	// DefaultProject when empty. Ignored when JQL is set (raw JQL is
	// wrapped against the *full* AllowedProjects list instead).
	Project string

	// Structured filters.
	Status       string
	Assignee     string // "me", "unassigned", an email, or a display name
	Type         string
	Epic         string // epic issue key
	Sprint       string // "current" or a numeric sprint ID
	Text         string
	UpdatedSince string // JQL-compatible duration ("-7d") or date
	CreatedSince string // JQL-compatible duration ("-7d") or date

	// Sort picks the ORDER BY field for structured searches: "created" or
	// "updated" (case-insensitive; empty defaults to "updated"). Ignored
	// when JQL is set, since raw JQL controls its own ordering (including
	// via a trailing ORDER BY clause, see splitTrailingOrderBy).
	Sort string
	// SortAsc reverses the default descending sort order to ascending.
	SortAsc bool

	// JQL, if set, is raw JQL supplied by the caller. It is wrapped as
	// `project in (<allowed>) AND (<jql>)` rather than used verbatim, and
	// takes precedence over the structured filters above.
	JQL string

	Limit     int
	PageToken string
}

// SearchIssues resolves params into JQL (quoting/escaping structured
// filters, or guarding raw JQL against AllowedProjects) and runs a single
// search page.
func (s *Service) SearchIssues(ctx context.Context, p SearchParams) (jira.IssuePage, error) {
	jql, err := s.buildJQL(p)
	if err != nil {
		return jira.IssuePage{}, err
	}

	limit := p.Limit
	if limit <= 0 {
		limit = jira.DefaultSearchLimit
	}
	if limit > jira.MaxSearchResults {
		limit = jira.MaxSearchResults
	}

	page, err := s.Client.SearchJQL(ctx, jql, nil, limit, p.PageToken)
	if err != nil {
		return jira.IssuePage{}, wrapAPIError(err)
	}
	return page, nil
}

// buildJQL turns SearchParams into a JQL string, enforcing the project
// allowlist guardrail in both the raw-JQL and structured-filter paths.
func (s *Service) buildJQL(p SearchParams) (string, error) {
	if raw := strings.TrimSpace(p.JQL); raw != "" {
		condition, orderBy, err := splitTrailingOrderBy(raw)
		if err != nil {
			return "", newUsageError("%s", err.Error())
		}

		if err := checkBalancedParens(condition); err != nil {
			return "", err
		}

		guard := "project in (" + s.allowedProjectsJQL() + ")"
		switch {
		case condition == "" && orderBy == "":
			// Unreachable in practice (raw is non-empty here, and an
			// all-whitespace-after-ORDER-BY case still leaves orderBy
			// non-empty), but handled for completeness.
			return guard, nil
		case condition == "":
			return guard + " " + orderBy, nil
		case orderBy == "":
			return guard + " AND (" + condition + ")", nil
		default:
			return guard + " AND (" + condition + ") " + orderBy, nil
		}
	}

	project := strings.TrimSpace(p.Project)
	if project == "" {
		project = s.DefaultProject
	}
	if err := s.checkProjectAllowed(project); err != nil {
		return "", err
	}

	clauses := []string{"project = " + jqlQuote(project)}

	if p.Status != "" {
		clauses = append(clauses, "status = "+jqlQuote(p.Status))
	}
	if p.Assignee != "" {
		switch strings.ToLower(p.Assignee) {
		case "me":
			clauses = append(clauses, "assignee = currentUser()")
		case "unassigned":
			clauses = append(clauses, "assignee is EMPTY")
		default:
			clauses = append(clauses, "assignee = "+jqlQuote(p.Assignee))
		}
	}
	if p.Type != "" {
		clauses = append(clauses, "issuetype = "+jqlQuote(p.Type))
	}
	if p.Epic != "" {
		clauses = append(clauses, "parent = "+jqlQuote(p.Epic))
	}
	if p.Sprint != "" {
		if strings.EqualFold(p.Sprint, "current") {
			clauses = append(clauses, "sprint in openSprints()")
		} else {
			// Sprint IDs are numeric literals in JQL, not quoted strings.
			clauses = append(clauses, "sprint = "+jqlNumberOrQuote(p.Sprint))
		}
	}
	if p.Text != "" {
		clauses = append(clauses, "text ~ "+jqlQuote(p.Text))
	}
	if p.UpdatedSince != "" {
		clauses = append(clauses, "updated >= "+jqlQuote(p.UpdatedSince))
	}
	if p.CreatedSince != "" {
		clauses = append(clauses, "created >= "+jqlQuote(p.CreatedSince))
	}

	sortField, err := sortFieldFor(p.Sort)
	if err != nil {
		return "", err
	}
	direction := "DESC"
	if p.SortAsc {
		direction = "ASC"
	}

	return strings.Join(clauses, " AND ") + " ORDER BY " + sortField + " " + direction, nil
}

// sortFieldFor validates and normalizes a --sort value for a structured
// search: "created" or "updated" (case-insensitive), defaulting to
// "updated" when empty.
func sortFieldFor(sort string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(sort)) {
	case "", "updated":
		return "updated", nil
	case "created":
		return "created", nil
	default:
		return "", newUsageError(`invalid sort %q; must be "created" or "updated"`, sort)
	}
}

// allowedProjectsJQL renders AllowedProjects as a quoted, comma-separated
// list suitable for `project in (...)`.
func (s *Service) allowedProjectsJQL() string {
	quoted := make([]string, len(s.AllowedProjects))
	for i, p := range s.AllowedProjects {
		quoted[i] = jqlQuote(p)
	}
	return strings.Join(quoted, ", ")
}

// jqlQuote safely quotes a string literal for JQL, escaping backslashes
// and double quotes.
func jqlQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// jqlNumberOrQuote renders s unquoted if it looks like an integer (as Jira
// sprint/board IDs are), otherwise falls back to a quoted string literal.
func jqlNumberOrQuote(s string) string {
	for _, r := range s {
		if r < '0' || r > '9' {
			return jqlQuote(s)
		}
	}
	if s == "" {
		return jqlQuote(s)
	}
	return s
}

// orderByRe matches a top-level "ORDER BY" keyword pair, case-insensitive
// and word-bounded, in a string that has already had its quoted regions
// masked out (see maskQuotedRegions). It's only ever run against the
// masked copy, never the original text.
var orderByRe = regexp.MustCompile(`(?i)\border\s+by\b`)

// andOrParenRe matches AND, OR, or a parenthesis as a standalone token, run
// against a masked copy of the ORDER BY clause to make sure it can't
// smuggle a boolean condition or escape the guard it gets spliced after.
var andOrParenRe = regexp.MustCompile(`(?i)\b(and|or)\b|[()]`)

// maskQuotedRegions returns a same-length copy of s (as bytes, so offsets
// line up with regexp's byte-indexed match positions even when s contains
// multi-byte UTF-8 text) with the contents of every quoted string literal
// (single- or double-quoted, JQL-style backslash escapes honored) replaced
// by 'x'. This lets keyword/paren scans run over it without being confused
// by "ORDER BY", "AND", "(" etc. appearing inside a string value. Every
// byte inside a masked region becomes the single ASCII byte 'x', so the
// result stays valid UTF-8 even when it cuts across a multi-byte rune.
func maskQuotedRegions(s string) []byte {
	b := []byte(s)
	masked := make([]byte, len(b))
	copy(masked, b)

	var quote byte
	for i := 0; i < len(b); i++ {
		c := b[i]
		if quote != 0 {
			masked[i] = 'x'
			switch {
			case c == '\\' && i+1 < len(b):
				i++
				masked[i] = 'x'
			case c == quote:
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			masked[i] = 'x'
		}
	}
	return masked
}

// splitTrailingOrderBy splits raw JQL into its boolean condition and a
// trailing "ORDER BY ..." clause, if one is present at the top level
// (outside quotes and outside any parenthesized group). Jira requires
// ORDER BY to follow any AND(...) wrapper rather than sit inside it, so
// buildJQL needs the two parts separately to build a valid guarded query.
//
// The split point is the first top-level "ORDER BY" found scanning
// left-to-right. The returned orderBy clause (verbatim from "ORDER"/"order"
// to the end of the string, case preserved) is validated to contain no
// AND/OR/parentheses outside quotes, since it is spliced back in after the
// allowlist guard rather than inside it — otherwise a caller could hide a
// second condition (or an unbalanced paren) in what's supposed to be just
// a sort order.
//
// When no top-level ORDER BY is found, condition is jql, unchanged, and
// orderBy is "".
func splitTrailingOrderBy(jql string) (condition, orderBy string, err error) {
	masked := maskQuotedRegions(jql)

	depth := 0
	depthAt := make([]int, len(masked)+1)
	for i, c := range masked {
		depthAt[i] = depth
		switch c {
		case '(':
			depth++
		case ')':
			depth--
		}
	}
	depthAt[len(masked)] = depth

	maskedStr := string(masked)
	idx := -1
	for _, loc := range orderByRe.FindAllStringIndex(maskedStr, -1) {
		if depthAt[loc[0]] == 0 {
			idx = loc[0]
			break
		}
	}
	if idx < 0 {
		return jql, "", nil
	}

	condition = strings.TrimSpace(jql[:idx])
	orderBy = strings.TrimSpace(jql[idx:])

	if loc := andOrParenRe.FindIndex(maskQuotedRegions(orderBy)); loc != nil {
		return "", "", errBadOrderBy
	}

	return condition, orderBy, nil
}

var errBadOrderBy = errors.New(`raw JQL's ORDER BY clause may not contain "AND", "OR", or parentheses`)

var errUnbalancedParens = errors.New("raw JQL has unbalanced parentheses")

// checkBalancedParens rejects JQL whose parentheses (outside quoted strings)
// are not balanced. Without this, a condition such as `a=1) OR (project=X`
// would close the guard's wrapping group early and escape the project
// allowlist, because the condition is spliced in as `guard AND (<cond>)`.
func checkBalancedParens(jql string) error {
	depth := 0
	for _, c := range maskQuotedRegions(jql) {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return newUsageError("%s", errUnbalancedParens.Error())
			}
		}
	}
	if depth != 0 {
		return newUsageError("%s", errUnbalancedParens.Error())
	}
	return nil
}
