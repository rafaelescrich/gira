package service

import "testing"

// TestBuildJQLRawOrderBy covers the raw-JQL guardrail wrapping in
// buildJQL, in particular splitting off a trailing ORDER BY clause so it
// lands after the project-allowlist AND(...) guard rather than inside it
// (Jira rejects "AND (... ORDER BY ...)" with a 400).
func TestBuildJQLRawOrderBy(t *testing.T) {
	svc := &Service{Options: Options{AllowedProjects: []string{"AI"}}}

	tests := []struct {
		name    string
		jql     string
		want    string
		wantErr bool
	}{
		{
			name: "condition with trailing order by",
			jql:  "updated >= -7d ORDER BY updated DESC",
			want: `project in ("AI") AND (updated >= -7d) ORDER BY updated DESC`,
		},
		{
			name: "lowercase order by",
			jql:  "updated >= -7d order by updated desc",
			want: `project in ("AI") AND (updated >= -7d) order by updated desc`,
		},
		{
			name: "no order by at all",
			jql:  "status = Done",
			want: `project in ("AI") AND (status = Done)`,
		},
		{
			name: "order by only, no condition",
			jql:  "ORDER BY created DESC",
			want: `project in ("AI") ORDER BY created DESC`,
		},
		{
			name: "order by only with leading/trailing space",
			jql:  "   ORDER BY created DESC   ",
			want: `project in ("AI") ORDER BY created DESC`,
		},
		{
			name: "multiple order by fields",
			jql:  "status = Done ORDER BY priority DESC, updated ASC",
			want: `project in ("AI") AND (status = Done) ORDER BY priority DESC, updated ASC`,
		},
		{
			name: "order by text inside a quoted string is not a split point",
			jql:  `text ~ "please order by tomorrow"`,
			want: `project in ("AI") AND (text ~ "please order by tomorrow")`,
		},
		{
			name: "order by inside parens is not top-level",
			jql:  `(status = Done) AND updated >= -7d ORDER BY updated DESC`,
			want: `project in ("AI") AND ((status = Done) AND updated >= -7d) ORDER BY updated DESC`,
		},
		{
			name: "orderbyfield identifier is not mistaken for the keyword",
			jql:  `orderby_field = 1`,
			want: `project in ("AI") AND (orderby_field = 1)`,
		},
		{
			name:    "order by clause smuggling AND is rejected",
			jql:     `status = Done ORDER BY updated) OR project = "OTHER" OR (true`,
			wantErr: true,
		},
		{
			name:    "order by clause smuggling a paren is rejected",
			jql:     `status = Done ORDER BY updated ASC) OR (1=1`,
			wantErr: true,
		},
		{
			name:    "order by clause smuggling OR is rejected",
			jql:     `status = Done ORDER BY updated OR true`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.buildJQL(SearchParams{JQL: tt.jql})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildJQL(%q) = %q, nil; want an error", tt.jql, got)
				}
				if ExitCode(err) != 1 {
					t.Errorf("ExitCode = %d, want 1 (usage error)", ExitCode(err))
				}
				return
			}
			if err != nil {
				t.Fatalf("buildJQL(%q): unexpected error: %v", tt.jql, err)
			}
			if got != tt.want {
				t.Errorf("buildJQL(%q) = %q, want %q", tt.jql, got, tt.want)
			}
		})
	}
}

// TestBuildJQLStructuredSortAndCreatedSince covers the --sort/--asc and
// --created-since additions to the structured-filter path.
func TestBuildJQLStructuredSortAndCreatedSince(t *testing.T) {
	svc := &Service{Options: Options{AllowedProjects: []string{"AI"}, DefaultProject: "AI"}}

	tests := []struct {
		name    string
		params  SearchParams
		want    string
		wantErr bool
	}{
		{
			name:   "default sort is updated desc",
			params: SearchParams{Status: "Done"},
			want:   `project = "AI" AND status = "Done" ORDER BY updated DESC`,
		},
		{
			name:   "sort created",
			params: SearchParams{Status: "Done", Sort: "created"},
			want:   `project = "AI" AND status = "Done" ORDER BY created DESC`,
		},
		{
			name:   "sort case-insensitive plus asc",
			params: SearchParams{Status: "Done", Sort: "CREATED", SortAsc: true},
			want:   `project = "AI" AND status = "Done" ORDER BY created ASC`,
		},
		{
			name:    "invalid sort is a usage error",
			params:  SearchParams{Sort: "priority"},
			wantErr: true,
		},
		{
			name:   "created_since adds a clause",
			params: SearchParams{CreatedSince: "-7d"},
			want:   `project = "AI" AND created >= "-7d" ORDER BY updated DESC`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.buildJQL(tt.params)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildJQL(%+v) = %q, nil; want an error", tt.params, got)
				}
				if ExitCode(err) != 1 {
					t.Errorf("ExitCode = %d, want 1 (usage error)", ExitCode(err))
				}
				return
			}
			if err != nil {
				t.Fatalf("buildJQL(%+v): unexpected error: %v", tt.params, err)
			}
			if got != tt.want {
				t.Errorf("buildJQL(%+v) = %q, want %q", tt.params, got, tt.want)
			}
		})
	}
}

// TestSplitTrailingOrderBy exercises the split helper directly for a few
// edge cases that are easiest to assert on without going through the full
// guard-wrapping in buildJQL.
func TestSplitTrailingOrderBy(t *testing.T) {
	tests := []struct {
		name          string
		jql           string
		wantCondition string
		wantOrderBy   string
		wantErr       bool
	}{
		{
			name:          "no order by",
			jql:           "status = Done",
			wantCondition: "status = Done",
			wantOrderBy:   "",
		},
		{
			name:          "simple split",
			jql:           "status = Done ORDER BY updated DESC",
			wantCondition: "status = Done",
			wantOrderBy:   "ORDER BY updated DESC",
		},
		{
			name:          "order-by-only",
			jql:           "ORDER BY updated DESC",
			wantCondition: "",
			wantOrderBy:   "ORDER BY updated DESC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cond, orderBy, err := splitTrailingOrderBy(tt.jql)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got condition=%q orderBy=%q", cond, orderBy)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cond != tt.wantCondition {
				t.Errorf("condition = %q, want %q", cond, tt.wantCondition)
			}
			if orderBy != tt.wantOrderBy {
				t.Errorf("orderBy = %q, want %q", orderBy, tt.wantOrderBy)
			}
		})
	}
}

func TestBuildJQLRejectsGuardEscape(t *testing.T) {
	s := &Service{Options: Options{AllowedProjects: []string{"AI"}, DefaultProject: "AI"}}
	for _, jql := range []string{
		`status=Done) OR (project = "OTHER"`,
		`a=1) OR project=OTHER OR (b=2`,
		`(a=1`,
		`a=1)`,
		`)(`,
	} {
		if q, err := s.buildJQL(SearchParams{JQL: jql}); err == nil {
			t.Errorf("buildJQL(%q) = %q, want error", jql, q)
		}
	}
	// Balanced parens and parens inside quotes remain fine.
	for _, jql := range []string{`(a=1 OR b=2) AND c=3`, `summary ~ "foo) OR ("`} {
		if _, err := s.buildJQL(SearchParams{JQL: jql}); err != nil {
			t.Errorf("buildJQL(%q) unexpected error: %v", jql, err)
		}
	}
}
