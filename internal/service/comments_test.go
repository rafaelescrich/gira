package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/rafaelescrich/gira/internal/jira"
)

func TestAddCommentRefusesReadOnly(t *testing.T) {
	svc := &Service{Options: Options{ReadOnly: true, AllowedProjects: []string{"AI"}}}
	_, err := svc.AddComment(context.Background(), "AI-1", "hi")
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4", ExitCode(err))
	}
}

func TestAddCommentRejectsDisallowedProjectKey(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call the API for a disallowed project key")
	})
	_, err := svc.AddComment(context.Background(), "OTHER-1", "hi")
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4", ExitCode(err))
	}
}

func TestAddCommentRequiresText(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call the API with empty comment text")
	})
	_, err := svc.AddComment(context.Background(), "AI-1", "   ")
	if ExitCode(err) != 1 {
		t.Errorf("ExitCode = %d, want 1", ExitCode(err))
	}
}

func TestAddCommentSendsADFBody(t *testing.T) {
	var gotBody struct {
		Body jira.ADFDoc `json:"body"`
	}
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/comment") {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jira.Comment{ID: "42"})
	})

	comment, err := svc.AddComment(context.Background(), "ai-1", "hello **world**")
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if comment.ID != "42" {
		t.Errorf("ID = %q", comment.ID)
	}
	if gotBody.Body.Type != "doc" {
		t.Errorf("expected ADF doc body, got %+v", gotBody.Body)
	}
}

func TestListCommentsRejectsDisallowedProjectKey(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call the API for a disallowed project key")
	})
	_, err := svc.ListComments(context.Background(), "OTHER-1")
	if ExitCode(err) != 4 {
		t.Errorf("ExitCode = %d, want 4", ExitCode(err))
	}
}

func TestListComments(t *testing.T) {
	svc := newTestService(t, defaultOptions(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"comments": []map[string]any{{"id": "1"}, {"id": "2"}},
		})
	})

	comments, err := svc.ListComments(context.Background(), "AI-1")
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(comments))
	}
}
