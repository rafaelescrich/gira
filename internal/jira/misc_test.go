package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- APIError helpers ----------------------------------------------------

func TestAPIErrorHelpers(t *testing.T) {
	notFound := parseAPIError(http.StatusNotFound, []byte(`{"errorMessages":["Issue does not exist"]}`))
	if !notFound.IsNotFound() {
		t.Error("IsNotFound() = false, want true for 404")
	}
	if notFound.IsForbidden() || notFound.IsUnauthorized() {
		t.Error("a 404 should not report IsForbidden/IsUnauthorized")
	}
	if got := notFound.Error(); got == "" {
		t.Error("Error() returned empty string")
	}

	forbidden := parseAPIError(http.StatusForbidden, nil)
	if !forbidden.IsForbidden() {
		t.Error("IsForbidden() = false, want true for 403")
	}

	unauthorized := parseAPIError(http.StatusUnauthorized, []byte(`irrelevant body`))
	if !unauthorized.IsUnauthorized() {
		t.Error("IsUnauthorized() = false, want true for 401")
	}
	if got := unauthorized.Error(); got != "jira: authentication failed (401) — token expired or revoked; run `gira auth login`" {
		t.Errorf("401 Error() = %q", got)
	}

	// A body that isn't valid JSON falls back to the raw text so the
	// caller/message still has something useful to show.
	unparsable := parseAPIError(http.StatusInternalServerError, []byte("<html>server error</html>"))
	if got := unparsable.RawBody(); got != "<html>server error</html>" {
		t.Errorf("RawBody() = %q", got)
	}
	if got := unparsable.Error(); got == "jira: request failed (500)" {
		t.Errorf("Error() should include the raw body when nothing structured was found, got %q", got)
	}

	// Field-level errors are surfaced verbatim.
	fieldErr := parseAPIError(http.StatusBadRequest, []byte(`{"errors":{"summary":"is required"}}`))
	if got := fieldErr.Error(); got != "jira: request failed (400): summary: is required" {
		t.Errorf("Error() = %q", got)
	}
}

// --- MyPermissions ---------------------------------------------------------

func TestMyPermissions(t *testing.T) {
	var gotProject, gotPerms string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotProject = r.URL.Query().Get("projectKey")
		gotPerms = r.URL.Query().Get("permissions")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"permissions": map[string]any{
				"CREATE_ISSUES": map[string]any{"havePermission": true},
				"EDIT_ISSUES":   map[string]any{"havePermission": false},
			},
		})
	})

	got, err := c.MyPermissions(context.Background(), "AI", []string{"CREATE_ISSUES", "EDIT_ISSUES"})
	if err != nil {
		t.Fatalf("MyPermissions: %v", err)
	}
	if gotProject != "AI" {
		t.Errorf("projectKey query param = %q, want AI", gotProject)
	}
	if gotPerms != "CREATE_ISSUES,EDIT_ISSUES" {
		t.Errorf("permissions query param = %q", gotPerms)
	}
	if !got["CREATE_ISSUES"] || got["EDIT_ISSUES"] {
		t.Errorf("MyPermissions result = %+v", got)
	}
}

func TestMyPermissionsRequiresProjectAndPermissions(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not make an HTTP request with invalid arguments")
	})
	if _, err := c.MyPermissions(context.Background(), "", []string{"X"}); err == nil {
		t.Error("expected an error for an empty project key")
	}
	if _, err := c.MyPermissions(context.Background(), "AI", nil); err == nil {
		t.Error("expected an error for an empty permissions list")
	}
}

// --- Board -----------------------------------------------------------------

func TestGetBoardAndConfiguration(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/rest/agile/1.0/board/336":
			json.NewEncoder(w).Encode(Board{ID: 336, Name: "Test board", Type: "simple"})
		case "/rest/agile/1.0/board/336/configuration":
			json.NewEncoder(w).Encode(BoardConfiguration{ID: 336, Name: "Test board"})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	})

	board, err := c.GetBoard(context.Background(), 336)
	if err != nil {
		t.Fatalf("GetBoard: %v", err)
	}
	if board.Name != "Test board" {
		t.Errorf("GetBoard().Name = %q", board.Name)
	}

	cfg, err := c.GetBoardConfiguration(context.Background(), 336)
	if err != nil {
		t.Fatalf("GetBoardConfiguration: %v", err)
	}
	if cfg.Name != "Test board" {
		t.Errorf("GetBoardConfiguration().Name = %q", cfg.Name)
	}
}

// --- SetHTTPClient -----------------------------------------------------------

func TestSetHTTPClient(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(User{AccountID: "1"})
	}))
	defer srv.Close()

	// Build a client pointed elsewhere, then retarget it via
	// SetHTTPClient's sibling (the base URL is fixed at construction, so
	// exercise SetHTTPClient itself by swapping in a client with a custom
	// Transport and confirming requests still reach the real server).
	c := New(srv.URL, "me@example.com", "tok")
	c.SetHTTPClient(&http.Client{Transport: http.DefaultTransport})

	if _, err := c.Myself(context.Background()); err != nil {
		t.Fatalf("Myself: %v", err)
	}
	if !called {
		t.Error("request never reached the test server after SetHTTPClient")
	}
}
