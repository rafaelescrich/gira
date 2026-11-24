package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestAssignSetsAccountID(t *testing.T) {
	var gotBody AssigneeRef
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/rest/api/3/issue/AI-1/assignee" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	})

	id := "712020:abc"
	if err := c.Assign(context.Background(), "AI-1", &id); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if gotBody.AccountID == nil || *gotBody.AccountID != id {
		t.Errorf("accountId = %v, want %q", gotBody.AccountID, id)
	}
}

func TestAssignUnassignSendsNull(t *testing.T) {
	var raw json.RawMessage
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		raw, _ = readAll(r)
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.Assign(context.Background(), "AI-1", nil); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if string(raw) != `{"accountId":null}` {
		t.Errorf("body = %s, want explicit null accountId", raw)
	}
}

func TestSearchUsers(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/user/search" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if q := r.URL.Query().Get("query"); q != "rafael" {
			t.Errorf("query = %q, want rafael", q)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]User{{AccountID: "1", DisplayName: "Rafael", Active: true}})
	})

	users, err := c.SearchUsers(context.Background(), "rafael")
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if len(users) != 1 || users[0].DisplayName != "Rafael" {
		t.Errorf("unexpected users: %+v", users)
	}
}
