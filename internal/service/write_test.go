package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/rafaelescrich/gira/internal/jira"
)

func TestCreateIssueRefusesReadOnly(t *testing.T) {
	svc := newTestService(t, Options{AllowedProjects: []string{"AI"}, DefaultProject: "AI", ReadOnly: true},
		func(w http.ResponseWriter, r *http.Request) {
			t.Error("should not call the API in read-only mode")
		})

	_, err := svc.CreateIssue(context.Background(), CreateParams{Type: "Task", Summary: "x"})
	if err == nil {
		t.Fatal("expected guardrail error")
	}
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4", ExitCode(err))
	}
}

func TestCreateIssueRejectsDisallowedProject(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call the API for a disallowed project")
	})

	_, err := svc.CreateIssue(context.Background(), CreateParams{Project: "SECRET", Type: "Task", Summary: "x"})
	if err == nil {
		t.Fatal("expected guardrail error")
	}
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4", ExitCode(err))
	}
}

func TestCreateIssueRequiresSummaryAndType(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call the API when required fields are missing")
	})

	if _, err := svc.CreateIssue(context.Background(), CreateParams{Type: "Task"}); err == nil {
		t.Error("expected usage error for missing summary")
	}
	if _, err := svc.CreateIssue(context.Background(), CreateParams{Summary: "x"}); err == nil {
		t.Error("expected usage error for missing type")
	}
}

func TestCreateIssueValidatesIssueTypeAgainstCreateMeta(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "createmeta") {
			json.NewEncoder(w).Encode(map[string]any{
				"issueTypes": []map[string]any{{"id": "1", "name": "Task"}, {"id": "2", "name": "Bug"}},
			})
			return
		}
		t.Errorf("unexpected call to %s", r.URL.Path)
	})

	_, err := svc.CreateIssue(context.Background(), CreateParams{Type: "Doing", Summary: "x"})
	if err == nil {
		t.Fatal("expected usage error for unknown issue type")
	}
	if !strings.Contains(err.Error(), "Task") || !strings.Contains(err.Error(), "Bug") {
		t.Errorf("error should list valid types, got: %v", err)
	}
	if ExitCode(err) != 1 {
		t.Errorf("ExitCode = %d, want 1 (usage)", ExitCode(err))
	}
}

func TestCreateIssueBuildsFieldsAndLinksEpic(t *testing.T) {
	var gotFields jira.IssueWriteFields
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "createmeta") {
			json.NewEncoder(w).Encode(map[string]any{
				"issueTypes": []map[string]any{{"id": "1", "name": "Task"}},
			})
			return
		}
		var req struct {
			Fields jira.IssueWriteFields `json:"fields"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotFields = req.Fields
		json.NewEncoder(w).Encode(jira.IssueRef{Key: "AI-99"})
	})

	ref, err := svc.CreateIssue(context.Background(), CreateParams{
		Type:                "task", // case-insensitive resolution
		Summary:             "Do the thing",
		DescriptionMarkdown: "some **body**",
		Epic:                "ai-3",
		Labels:              []string{"x", "y"},
		Priority:            "High",
	})
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if ref.Key != "AI-99" {
		t.Errorf("Key = %q", ref.Key)
	}
	if gotFields.IssueType == nil || gotFields.IssueType.Name != "Task" {
		t.Errorf("issue type not resolved to canonical name: %+v", gotFields.IssueType)
	}
	if gotFields.Parent == nil || gotFields.Parent.Key != "AI-3" {
		t.Errorf("epic parent not set: %+v", gotFields.Parent)
	}
	if gotFields.Description == nil {
		t.Error("expected description ADF to be set")
	}
	if gotFields.Labels == nil || len(*gotFields.Labels) != 2 {
		t.Errorf("labels = %+v", gotFields.Labels)
	}
	if gotFields.Priority == nil || gotFields.Priority.Name != "High" {
		t.Errorf("priority = %+v", gotFields.Priority)
	}
}

func TestUpdateIssueRefusesReadOnly(t *testing.T) {
	svc := &Service{Options: Options{ReadOnly: true, AllowedProjects: []string{"AI"}}}
	summary := "x"
	err := svc.UpdateIssue(context.Background(), "AI-1", UpdateParams{Summary: &summary})
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4", ExitCode(err))
	}
}

func TestUpdateIssueRequiresAtLeastOneField(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call the API with nothing to update")
	})
	err := svc.UpdateIssue(context.Background(), "AI-1", UpdateParams{})
	if err == nil {
		t.Fatal("expected usage error")
	}
	if ExitCode(err) != 1 {
		t.Errorf("ExitCode = %d, want 1", ExitCode(err))
	}
}

func TestUpdateIssueSendsOnlyChangedFields(t *testing.T) {
	var raw json.RawMessage
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		raw = b
		w.WriteHeader(http.StatusNoContent)
	})

	summary := "New summary"
	err := svc.UpdateIssue(context.Background(), "ai-1", UpdateParams{Summary: &summary})
	if err != nil {
		t.Fatalf("UpdateIssue: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}
	fields := decoded["fields"].(map[string]any)
	if len(fields) != 1 || fields["summary"] != "New summary" {
		t.Errorf("fields = %#v", fields)
	}
}

func TestTransitionResolvesByStatusNameCaseInsensitive(t *testing.T) {
	var gotTransitionID string
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/transitions") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"transitions": []map[string]any{
					{"id": "11", "name": "Start Progress", "to": map[string]any{"name": "In Progress"}},
					{"id": "21", "name": "Block", "to": map[string]any{"name": "Blocked"}},
				},
			})
		case strings.Contains(r.URL.Path, "/transitions") && r.Method == http.MethodPost:
			var body struct {
				Transition struct {
					ID string `json:"id"`
				} `json:"transition"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotTransitionID = body.Transition.ID
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})

	status, err := svc.Transition(context.Background(), "AI-1", "in progress")
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if status.Name != "In Progress" {
		t.Errorf("status = %+v", status)
	}
	if gotTransitionID != "11" {
		t.Errorf("transition id = %q, want 11", gotTransitionID)
	}
}

func TestTransitionResolvesByTransitionName(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/transitions") && r.Method == http.MethodGet:
			json.NewEncoder(w).Encode(map[string]any{
				"transitions": []map[string]any{
					{"id": "11", "name": "Start Progress", "to": map[string]any{"name": "In Progress"}},
				},
			})
		case strings.Contains(r.URL.Path, "/transitions") && r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		}
	})

	status, err := svc.Transition(context.Background(), "AI-1", "start progress")
	if err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if status.Name != "In Progress" {
		t.Errorf("status = %+v", status)
	}
}

func TestTransitionUnknownStatusListsValidOptions(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/transitions"):
			json.NewEncoder(w).Encode(map[string]any{
				"transitions": []map[string]any{
					{"id": "11", "name": "Start Progress", "to": map[string]any{"name": "In Progress"}},
					{"id": "21", "name": "Block", "to": map[string]any{"name": "Blocked"}},
				},
			})
		case strings.HasSuffix(r.URL.Path, "/issue/AI-1"):
			json.NewEncoder(w).Encode(map[string]any{
				"fields": map[string]any{"status": map[string]any{"name": "To Do"}},
			})
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	})

	_, err := svc.Transition(context.Background(), "AI-1", "Doing")
	if err == nil {
		t.Fatal("expected usage error")
	}
	msg := err.Error()
	if !strings.Contains(msg, `"Doing"`) || !strings.Contains(msg, `"To Do"`) ||
		!strings.Contains(msg, "In Progress") || !strings.Contains(msg, "Blocked") {
		t.Errorf("unexpected error message: %s", msg)
	}
	if ExitCode(err) != 1 {
		t.Errorf("ExitCode = %d, want 1", ExitCode(err))
	}
}

func TestTransitionRefusesReadOnly(t *testing.T) {
	svc := &Service{Options: Options{ReadOnly: true, AllowedProjects: []string{"AI"}}}
	_, err := svc.Transition(context.Background(), "AI-1", "Done")
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4", ExitCode(err))
	}
}

func TestAssignMe(t *testing.T) {
	var gotAccountID *string
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/myself"):
			json.NewEncoder(w).Encode(jira.User{AccountID: "me-id"})
		case strings.HasSuffix(r.URL.Path, "/assignee"):
			var body jira.AssigneeRef
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotAccountID = body.AccountID
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	})

	if err := svc.Assign(context.Background(), "AI-1", "me"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if gotAccountID == nil || *gotAccountID != "me-id" {
		t.Errorf("accountID = %v, want me-id", gotAccountID)
	}
}

func TestAssignUnassigned(t *testing.T) {
	var gotAccountID = new(string) // non-nil sentinel to verify it gets set to nil
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		var body jira.AssigneeRef
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotAccountID = body.AccountID
		w.WriteHeader(http.StatusNoContent)
	})

	if err := svc.Assign(context.Background(), "AI-1", "unassigned"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if gotAccountID != nil {
		t.Errorf("accountID = %v, want nil", *gotAccountID)
	}
}

func TestAssignAmbiguousNameListsCandidates(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/user/search") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]jira.User{
				{AccountID: "1", DisplayName: "Alex Smith", Email: "alex.smith@x.com", Active: true},
				{AccountID: "2", DisplayName: "Alex Jones", Email: "alex.jones@x.com", Active: true},
			})
			return
		}
		t.Errorf("should not reach assignee endpoint on ambiguous match: %s", r.URL.Path)
	})

	err := svc.Assign(context.Background(), "AI-1", "Alex")
	if err == nil {
		t.Fatal("expected ambiguous-match usage error")
	}
	if !strings.Contains(err.Error(), "Alex Smith") || !strings.Contains(err.Error(), "Alex Jones") {
		t.Errorf("error should list candidates, got: %v", err)
	}
	if ExitCode(err) != 1 {
		t.Errorf("ExitCode = %d, want 1", ExitCode(err))
	}
}

func TestAssignExactMatchAmongMultipleResults(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/user/search") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]jira.User{
				{AccountID: "1", DisplayName: "Alex Smith", Email: "alex@x.com", Active: true},
				{AccountID: "2", DisplayName: "Alex Smithers", Email: "alexs@x.com", Active: true},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/assignee") {
			var body jira.AssigneeRef
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.AccountID == nil || *body.AccountID != "1" {
				t.Errorf("expected exact match accountID 1, got %v", body.AccountID)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		t.Errorf("unexpected request %s", r.URL.Path)
	})

	if err := svc.Assign(context.Background(), "AI-1", "Alex Smith"); err != nil {
		t.Fatalf("Assign: %v", err)
	}
}

func TestAssignRejectsDisallowedProjectKey(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call the API for a disallowed project key")
	})
	err := svc.Assign(context.Background(), "OTHER-1", "me")
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4", ExitCode(err))
	}
}

func TestListTransitionsCachesWithinTTL(t *testing.T) {
	calls := 0
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"transitions": []map[string]any{{"id": "1", "name": "Done", "to": map[string]any{"name": "Done"}}},
		})
	})

	if _, err := svc.ListTransitions(context.Background(), "AI-1"); err != nil {
		t.Fatalf("ListTransitions: %v", err)
	}
	if _, err := svc.ListTransitions(context.Background(), "AI-1"); err != nil {
		t.Fatalf("ListTransitions: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call (second served from cache), got %d", calls)
	}
}
