package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestListComments(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/AI-1/comment" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(commentsResponse{
			Comments: []Comment{{ID: "1", Author: &User{DisplayName: "Rafael"}}},
			Total:    1,
		})
	})

	comments, err := c.ListComments(context.Background(), "AI-1")
	if err != nil {
		t.Fatalf("ListComments: %v", err)
	}
	if len(comments) != 1 || comments[0].ID != "1" {
		t.Errorf("unexpected comments: %+v", comments)
	}
}

func TestAddCommentSendsADFBody(t *testing.T) {
	var gotBody addCommentRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/rest/api/3/issue/AI-1/comment" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Comment{ID: "999"})
	})

	doc := MarkdownToADF("hello **world**")
	comment, err := c.AddComment(context.Background(), "AI-1", doc)
	if err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if comment.ID != "999" {
		t.Errorf("ID = %q, want 999", comment.ID)
	}
	if gotBody.Body.Type != "doc" {
		t.Errorf("body type = %q, want doc", gotBody.Body.Type)
	}
	if len(gotBody.Body.Content) == 0 {
		t.Errorf("expected non-empty ADF content")
	}
}
