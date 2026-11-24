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

func TestBacklogUsesDefaultBoardWhenZero(t *testing.T) {
	var gotPath string
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"startAt": 0, "total": 0, "issues": []any{}})
	})

	if _, _, err := svc.Backlog(context.Background(), 0, 0); err != nil {
		t.Fatalf("Backlog: %v", err)
	}
	if gotPath != "/rest/agile/1.0/board/336/backlog" {
		t.Errorf("path = %q, want board 336 (default_board)", gotPath)
	}
}

func TestBacklogNoBoardConfiguredErrors(t *testing.T) {
	svc := newTestService(t, Options{AllowedProjects: []string{"AI"}, DefaultProject: "AI"}, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not call Jira when no board is configured")
	})
	_, _, err := svc.Backlog(context.Background(), 0, 0)
	if err == nil {
		t.Fatal("expected usage error")
	}
	if ExitCode(err) != 1 {
		t.Errorf("ExitCode = %d, want 1 (usage)", ExitCode(err))
	}
}

func sprintFixtureHandler(t *testing.T, sprints []jira.Sprint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"startAt": 0, "total": len(sprints), "isLast": true, "values": sprints,
		})
	}
}

func TestSprintsDefaultsToActiveAndFuture(t *testing.T) {
	var gotQuery string
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("state")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"total": 0, "isLast": true, "values": []any{}})
	})
	if _, err := svc.Sprints(context.Background(), 0, false); err != nil {
		t.Fatalf("Sprints: %v", err)
	}
	if gotQuery != "active,future" {
		t.Errorf("state query = %q, want active,future", gotQuery)
	}
}

func TestSprintsAllIncludesClosedWhenRequested(t *testing.T) {
	var gotQuery string
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("state")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"total": 0, "isLast": true, "values": []any{}})
	})
	if _, err := svc.Sprints(context.Background(), 0, true); err != nil {
		t.Fatalf("Sprints: %v", err)
	}
	if gotQuery != "active,future,closed" {
		t.Errorf("state query = %q, want active,future,closed", gotQuery)
	}
}

func TestResolveSprintCurrentNoActiveSprintIsUsageError(t *testing.T) {
	svc := newTestService(t, defaultOptions(), sprintFixtureHandler(t, []jira.Sprint{
		{ID: 2, Name: "September 2", State: "future"},
	}))

	_, err := svc.ResolveSprint(context.Background(), 0, "current")
	if err == nil {
		t.Fatal("expected usage error for no active sprint")
	}
	if ExitCode(err) != 1 {
		t.Errorf("ExitCode = %d, want 1 (usage)", ExitCode(err))
	}
}

func TestResolveSprintCurrentReturnsSoleActiveSprint(t *testing.T) {
	svc := newTestService(t, defaultOptions(), sprintFixtureHandler(t, []jira.Sprint{
		{ID: 1, Name: "Alpha", State: "active"},
		{ID: 2, Name: "September 2", State: "future"},
	}))

	sp, err := svc.ResolveSprint(context.Background(), 0, "current")
	if err != nil {
		t.Fatalf("ResolveSprint: %v", err)
	}
	if sp.ID != 1 {
		t.Errorf("ID = %d, want 1", sp.ID)
	}
}

func TestResolveSprintNextPicksEarliestFuture(t *testing.T) {
	svc := newTestService(t, defaultOptions(), sprintFixtureHandler(t, []jira.Sprint{
		{ID: 3, Name: "October", State: "future", StartDate: "2026-10-01T00:00:00.000Z"},
		{ID: 2, Name: "September 2", State: "future", StartDate: "2026-09-15T00:00:00.000Z"},
	}))

	sp, err := svc.ResolveSprint(context.Background(), 0, "next")
	if err != nil {
		t.Fatalf("ResolveSprint: %v", err)
	}
	if sp.ID != 2 {
		t.Errorf("ID = %d, want 2 (earliest start date)", sp.ID)
	}
}

func TestResolveSprintByNumericID(t *testing.T) {
	svc := newTestService(t, defaultOptions(), sprintFixtureHandler(t, []jira.Sprint{
		{ID: 1, Name: "Alpha", State: "closed"},
		{ID: 2, Name: "September 2", State: "future"},
	}))

	sp, err := svc.ResolveSprint(context.Background(), 0, "1")
	if err != nil {
		t.Fatalf("ResolveSprint: %v", err)
	}
	if sp.Name != "Alpha" {
		t.Errorf("Name = %q, want Alpha", sp.Name)
	}
}

func TestResolveSprintByName(t *testing.T) {
	svc := newTestService(t, defaultOptions(), sprintFixtureHandler(t, []jira.Sprint{
		{ID: 2, Name: "September 2", State: "future"},
	}))

	sp, err := svc.ResolveSprint(context.Background(), 0, "september 2")
	if err != nil {
		t.Fatalf("ResolveSprint: %v", err)
	}
	if sp.ID != 2 {
		t.Errorf("ID = %d, want 2", sp.ID)
	}
}

// TestSprintIssuesResolvesSprintThenListsIts covers the thin
// Service.SprintIssues wrapper: it must resolve sprintRef via
// ResolveSprint (hitting the board's sprint list) and then fetch that
// sprint's issues, returning both.
func TestSprintIssuesResolvesSprintThenListsIssues(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/agile/1.0/board/{id}/sprint", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"total": 1, "isLast": true,
			"values": []jira.Sprint{{ID: 832, Name: "September 2", State: "future"}},
		})
	})
	mux.HandleFunc("GET /rest/agile/1.0/sprint/{id}/issue", func(w http.ResponseWriter, r *http.Request) {
		if got := r.PathValue("id"); got != "832" {
			t.Errorf("sprint id in path = %q, want 832", got)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"total": 1, "issues": []jira.Issue{{Key: "AI-96"}},
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	svc := New(jira.New(srv.URL, "me@example.com", "token"), defaultOptions())

	issues, sprint, err := svc.SprintIssues(context.Background(), 0, "832", 10)
	if err != nil {
		t.Fatalf("SprintIssues: %v", err)
	}
	if sprint.ID != 832 || sprint.Name != "September 2" {
		t.Errorf("resolved sprint = %+v", sprint)
	}
	if len(issues) != 1 || issues[0].Key != "AI-96" {
		t.Errorf("issues = %+v", issues)
	}
}

func TestMoveToSprintRefusesReadOnly(t *testing.T) {
	called := false
	opts := defaultOptions()
	opts.ReadOnly = true
	svc := newTestService(t, opts, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	_, err := svc.MoveToSprint(context.Background(), 0, "current", []string{"AI-1"})
	if err == nil {
		t.Fatal("expected guardrail error in read-only mode")
	}
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4 (guardrail)", ExitCode(err))
	}
	if called {
		t.Error("Jira API should not have been called in read-only mode")
	}
}

func TestMoveToSprintRefusesDisallowedProjectKey(t *testing.T) {
	called := false
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	_, err := svc.MoveToSprint(context.Background(), 0, "current", []string{"OTHER-1"})
	if err == nil {
		t.Fatal("expected guardrail error for disallowed project")
	}
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4 (guardrail)", ExitCode(err))
	}
	if called {
		t.Error("Jira API should not have been called for a disallowed key (checked before sprint resolution)")
	}
}

func TestMoveToSprintRefusesOverBatchCap(t *testing.T) {
	called := false
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	keys := make([]string, MaxAgileMoveKeys+1)
	for i := range keys {
		keys[i] = "AI-1"
	}
	_, err := svc.MoveToSprint(context.Background(), 0, "current", keys)
	if err == nil {
		t.Fatal("expected usage error over batch cap")
	}
	if ExitCode(err) != 1 {
		t.Errorf("ExitCode = %d, want 1 (usage)", ExitCode(err))
	}
	if called {
		t.Error("Jira API should not have been called over the batch cap")
	}
}

func TestMoveToBacklogRefusesReadOnly(t *testing.T) {
	opts := defaultOptions()
	opts.ReadOnly = true
	svc := newTestService(t, opts, func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not call Jira in read-only mode")
	})

	err := svc.MoveToBacklog(context.Background(), []string{"AI-1"})
	if err == nil {
		t.Fatal("expected guardrail error in read-only mode")
	}
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4 (guardrail)", ExitCode(err))
	}
}

func TestMoveToBacklogHappyPath(t *testing.T) {
	var gotIssues []string
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Issues []string `json:"issues"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotIssues = body.Issues
		w.WriteHeader(http.StatusNoContent)
	})

	if err := svc.MoveToBacklog(context.Background(), []string{"ai-1", "AI-2", "ai-1"}); err != nil {
		t.Fatalf("MoveToBacklog: %v", err)
	}
	if len(gotIssues) != 2 {
		t.Fatalf("Issues = %v, want 2 (deduped, upper-cased)", gotIssues)
	}
}

func TestEpicsFallsBackToJQLWhenBoardEndpointErrors(t *testing.T) {
	calls := 0
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/rest/agile/1.0/board/336/epic":
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"errorMessages":["The board does not support epics."]}`))
		case r.URL.Path == "/rest/api/3/search/jql":
			var req struct {
				JQL string `json:"jql"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if want := `issuetype = "Epic"`; !strings.Contains(req.JQL, want) {
				t.Errorf("fallback JQL = %q, want it to contain %q", req.JQL, want)
			}
			json.NewEncoder(w).Encode(map[string]any{
				"issues": []jira.Issue{
					{ID: "10001", Key: "AI-3", Fields: jira.IssueFields{Summary: "Epic one"}},
				},
				"isLast": true,
			})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	})

	epics, err := svc.Epics(context.Background(), 0)
	if err != nil {
		t.Fatalf("Epics: %v", err)
	}
	if len(epics) != 1 || epics[0].Key != "AI-3" {
		t.Fatalf("epics = %+v, want one epic AI-3 (via JQL fallback)", epics)
	}
}
