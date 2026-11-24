package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestListTransitions(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/AI-1/transitions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(transitionsResponse{
			Transitions: []Transition{
				{ID: "11", Name: "Start Progress", To: Status{Name: "In Progress"}},
				{ID: "21", Name: "Block", To: Status{Name: "Blocked"}},
			},
		})
	})

	transitions, err := c.ListTransitions(context.Background(), "AI-1")
	if err != nil {
		t.Fatalf("ListTransitions: %v", err)
	}
	if len(transitions) != 2 || transitions[0].To.Name != "In Progress" {
		t.Errorf("unexpected transitions: %+v", transitions)
	}
}

func TestDoTransitionSendsTransitionID(t *testing.T) {
	var gotBody doTransitionRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/rest/api/3/issue/AI-1/transitions" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.DoTransition(context.Background(), "AI-1", "11"); err != nil {
		t.Fatalf("DoTransition: %v", err)
	}
	if gotBody.Transition.ID != "11" {
		t.Errorf("transition id = %q, want 11", gotBody.Transition.ID)
	}
}
