package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestBacklogAllPagesUntilTotalReached(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			if got := r.URL.Query().Get("startAt"); got != "0" {
				t.Errorf("call 1 startAt = %q, want 0", got)
			}
			json.NewEncoder(w).Encode(backlogResponse{
				StartAt: 0, Total: 3,
				Issues: []Issue{{Key: "AI-1"}, {Key: "AI-2"}},
			})
		case 2:
			if got := r.URL.Query().Get("startAt"); got != "2" {
				t.Errorf("call 2 startAt = %q, want 2", got)
			}
			json.NewEncoder(w).Encode(backlogResponse{
				StartAt: 2, Total: 3,
				Issues: []Issue{{Key: "AI-3"}},
			})
		default:
			t.Fatalf("unexpected call #%d", calls)
		}
	})

	issues, total, err := c.BacklogAll(context.Background(), 336, 50)
	if err != nil {
		t.Fatalf("BacklogAll: %v", err)
	}
	if total != 3 || len(issues) != 3 {
		t.Fatalf("got %d issues (total %d), want 3", len(issues), total)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestBacklogAllStopsAtLimit(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		// Server always has more than the caller's limit.
		json.NewEncoder(w).Encode(backlogResponse{
			StartAt: 0, Total: 500,
			Issues: []Issue{{Key: "AI-1"}, {Key: "AI-2"}},
		})
	})

	issues, _, err := c.BacklogAll(context.Background(), 336, 2)
	if err != nil {
		t.Fatalf("BacklogAll: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("got %d issues, want 2 (limit)", len(issues))
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestSprintsAllHonorsStateFilterAndIsLast(t *testing.T) {
	var gotState string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotState = r.URL.Query().Get("state")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sprintListResponse{
			Total: 2, IsLast: true,
			Values: []Sprint{
				{ID: 1, Name: "Alpha", State: "active"},
				{ID: 2, Name: "September 2", State: "future"},
			},
		})
	})

	sprints, total, err := c.SprintsAll(context.Background(), 336, []string{"active", "future"}, 50)
	if err != nil {
		t.Fatalf("SprintsAll: %v", err)
	}
	if gotState != "active,future" {
		t.Errorf("state query = %q, want active,future", gotState)
	}
	if total != 2 || len(sprints) != 2 {
		t.Fatalf("got %d sprints (total %d), want 2", len(sprints), total)
	}
}

func TestMoveToSprintChunksAt50(t *testing.T) {
	var callSizes []int
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/sprint/42/issue" {
			t.Errorf("path = %q", r.URL.Path)
		}
		var body moveIssuesRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		callSizes = append(callSizes, len(body.Issues))
		w.WriteHeader(http.StatusNoContent)
	})

	keys := make([]string, 0, 120)
	for i := 0; i < 120; i++ {
		keys = append(keys, fmt.Sprintf("AI-%d", i))
	}

	if err := c.MoveToSprint(context.Background(), 42, keys); err != nil {
		t.Fatalf("MoveToSprint: %v", err)
	}
	if len(callSizes) != 3 {
		t.Fatalf("expected 3 chunked calls, got %d (%v)", len(callSizes), callSizes)
	}
	if callSizes[0] != 50 || callSizes[1] != 50 || callSizes[2] != 20 {
		t.Errorf("chunk sizes = %v, want [50 50 20]", callSizes)
	}
}

func TestMoveToBacklogSendsIssues(t *testing.T) {
	var gotBody moveIssuesRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/backlog/issue" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.MoveToBacklog(context.Background(), []string{"AI-1", "AI-2"}); err != nil {
		t.Fatalf("MoveToBacklog: %v", err)
	}
	if len(gotBody.Issues) != 2 {
		t.Errorf("Issues = %v, want 2 entries", gotBody.Issues)
	}
}

func TestBoardEpicsAllOnTeamManagedBoardErrors(t *testing.T) {
	// Simulates a team-managed ("simple") board where the board epic
	// endpoint isn't supported.
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errorMessages":["The board does not support epics."]}`))
	})

	_, _, err := c.BoardEpicsAll(context.Background(), 336, 50)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("StatusCode = %d, want 400", apiErr.StatusCode)
	}
}

func TestSprintIssuesAllPages(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			json.NewEncoder(w).Encode(backlogResponse{StartAt: 0, Total: 3, Issues: []Issue{{Key: "AI-1"}, {Key: "AI-2"}}})
		case 2:
			json.NewEncoder(w).Encode(backlogResponse{StartAt: 2, Total: 3, Issues: []Issue{{Key: "AI-3"}}})
		default:
			t.Fatalf("unexpected call #%d", calls)
		}
	})

	issues, total, err := c.SprintIssuesAll(context.Background(), 7, 50)
	if err != nil {
		t.Fatalf("SprintIssuesAll: %v", err)
	}
	if total != 3 || len(issues) != 3 {
		t.Fatalf("got %d issues (total %d), want 3", len(issues), total)
	}
}

func TestChunkStrings(t *testing.T) {
	chunks := chunkStrings([]string{"a", "b", "c", "d", "e"}, 2)
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3", len(chunks))
	}
	if len(chunks[0]) != 2 || len(chunks[1]) != 2 || len(chunks[2]) != 1 {
		t.Errorf("chunk sizes = %v", chunks)
	}

	if got := chunkStrings(nil, 2); got != nil {
		t.Errorf("chunkStrings(nil) = %v, want nil", got)
	}
}
