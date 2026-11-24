package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rafaelescrich/gira/internal/jira"
)

func newTestService(t *testing.T, opts Options, handler http.HandlerFunc) *Service {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	client := jira.New(srv.URL, "me@example.com", "token")
	return New(client, opts)
}

func defaultOptions() Options {
	return Options{
		AllowedProjects: []string{"AI"},
		DefaultProject:  "AI",
		DefaultBoard:    336,
	}
}

func TestSearchIssuesStructuredFiltersQuoteAndScopeToProject(t *testing.T) {
	var gotJQL string
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			JQL string `json:"jql"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotJQL = req.JQL
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"issues": []any{}, "isLast": true})
	})

	_, err := svc.SearchIssues(context.Background(), SearchParams{
		Status:   `In "Progress"`,
		Assignee: "me",
		Type:     "Bug",
	})
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}

	if !strings.Contains(gotJQL, `project = "AI"`) {
		t.Errorf("JQL should scope to default project, got %q", gotJQL)
	}
	if !strings.Contains(gotJQL, `status = "In \"Progress\""`) {
		t.Errorf("JQL should escape embedded quotes, got %q", gotJQL)
	}
	if !strings.Contains(gotJQL, "assignee = currentUser()") {
		t.Errorf("JQL should translate assignee=me, got %q", gotJQL)
	}
	if !strings.Contains(gotJQL, `issuetype = "Bug"`) {
		t.Errorf("JQL should quote issue type, got %q", gotJQL)
	}
}

func TestSearchIssuesRawJQLWrappedInAllowlistGuard(t *testing.T) {
	var gotJQL string
	svc := newTestService(t, Options{
		AllowedProjects: []string{"AI", "OPS"},
		DefaultProject:  "AI",
	}, func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			JQL string `json:"jql"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotJQL = req.JQL
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"issues": []any{}, "isLast": true})
	})

	_, err := svc.SearchIssues(context.Background(), SearchParams{JQL: "status = Done"})
	if err != nil {
		t.Fatalf("SearchIssues: %v", err)
	}

	want := `project in ("AI", "OPS") AND (status = Done)`
	if gotJQL != want {
		t.Errorf("gotJQL = %q, want %q", gotJQL, want)
	}
}

func TestSearchIssuesRejectsDisallowedProject(t *testing.T) {
	called := false
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	_, err := svc.SearchIssues(context.Background(), SearchParams{Project: "SECRET"})
	if err == nil {
		t.Fatal("expected guardrail error")
	}
	if called {
		t.Error("Jira API should not have been called for a disallowed project")
	}
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4 (guardrail)", ExitCode(err))
	}
}

func TestGetIssueRejectsDisallowedProjectKey(t *testing.T) {
	called := false
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	_, err := svc.GetIssue(context.Background(), "OTHER-1")
	if err == nil {
		t.Fatal("expected guardrail error")
	}
	if called {
		t.Error("Jira API should not have been called for a disallowed project key")
	}
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4 (guardrail)", ExitCode(err))
	}
}

func TestGetIssueAllowsAllowedProjectKey(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jira.Issue{Key: "AI-1"})
	})

	issue, err := svc.GetIssue(context.Background(), "ai-1")
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.Key != "AI-1" {
		t.Errorf("Key = %q", issue.Key)
	}
}

// TestGetIssueRejectsMalformedKeys guards against keys that would otherwise
// slip past the project-prefix allowlist check (which historically just
// looked at everything before the last "-") and get spliced, unescaped,
// into a Jira REST URL by internal/jira — e.g. a key containing a path
// separator could route a request to a completely different endpoint on
// the same authenticated host, bypassing the intent of AllowedProjects.
func TestGetIssueRejectsMalformedKeys(t *testing.T) {
	called := false
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	for _, key := range []string{
		"AI-1/../../rest/api/3/mypermissions",
		"AI-1?maxResults=1",
		"AI-",
		"-12",
		"AI-12-34",
		"",
		"   ",
	} {
		called = false
		_, err := svc.GetIssue(context.Background(), key)
		if err == nil {
			t.Errorf("key %q: expected an error, got none", key)
			continue
		}
		if called {
			t.Errorf("key %q: Jira API should not have been called", key)
		}
	}
}

func TestMe(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/myself" {
			t.Errorf("path = %q, want /rest/api/3/myself", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jira.User{AccountID: "acc-1", DisplayName: "Rafael"})
	})

	me, err := svc.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if me.AccountID != "acc-1" {
		t.Errorf("Me().AccountID = %q", me.AccountID)
	}
}

func TestMeWrapsAPIErrorAsAuth(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errorMessages":["bad token"]}`))
	})
	_, err := svc.Me(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if ExitCode(err) != 3 {
		t.Errorf("ExitCode = %d, want 3 (auth)", ExitCode(err))
	}
}

func TestRequireWritableRefusesWhenReadOnly(t *testing.T) {
	svc := &Service{Options: Options{ReadOnly: true}}
	err := svc.requireWritable()
	if err == nil {
		t.Fatal("expected guardrail error in read-only mode")
	}
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4", ExitCode(err))
	}
}

func TestRequireWritableAllowsWhenNotReadOnly(t *testing.T) {
	svc := &Service{Options: Options{ReadOnly: false}}
	if err := svc.requireWritable(); err != nil {
		t.Fatalf("requireWritable() = %v, want nil", err)
	}
}

func TestExitCodeMapsAuthAndAPI(t *testing.T) {
	authErr := &Error{Kind: KindAuth}
	if ExitCode(authErr) != 3 {
		t.Errorf("auth ExitCode = %d, want 3", ExitCode(authErr))
	}
	apiErr := &Error{Kind: KindAPI}
	if ExitCode(apiErr) != 2 {
		t.Errorf("api ExitCode = %d, want 2", ExitCode(apiErr))
	}
	usageErr := &Error{Kind: KindUsage}
	if ExitCode(usageErr) != 1 {
		t.Errorf("usage ExitCode = %d, want 1", ExitCode(usageErr))
	}
}
