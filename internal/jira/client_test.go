package jira

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient builds a Client pointed at an httptest.Server, with sleep
// stubbed out so retry tests run instantly.
func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := New(srv.URL, "me@example.com", "secret-token")
	c.sleep = func(_ time.Duration) {}
	return c
}

func TestAuthHeaderPresent(t *testing.T) {
	var gotAuth string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if got := r.Header.Get("User-Agent"); got == "" {
			t.Errorf("expected User-Agent header, got none")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(User{AccountID: "123", DisplayName: "Me"})
	})

	u, err := c.Myself(context.Background())
	if err != nil {
		t.Fatalf("Myself: %v", err)
	}
	if u.AccountID != "123" {
		t.Errorf("AccountID = %q, want 123", u.AccountID)
	}

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("me@example.com:secret-token"))
	if gotAuth != wantAuth {
		t.Errorf("Authorization header = %q, want %q", gotAuth, wantAuth)
	}
}

func TestSearchJQLPaging(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req searchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			if req.NextPageToken != "" {
				t.Errorf("first call should have no page token, got %q", req.NextPageToken)
			}
			json.NewEncoder(w).Encode(searchResponse{
				Issues:        []Issue{{Key: "AI-1"}, {Key: "AI-2"}},
				NextPageToken: "page-2",
				IsLast:        false,
			})
		case 2:
			if req.NextPageToken != "page-2" {
				t.Errorf("second call page token = %q, want page-2", req.NextPageToken)
			}
			json.NewEncoder(w).Encode(searchResponse{
				Issues: []Issue{{Key: "AI-3"}},
				IsLast: true,
			})
		default:
			t.Fatalf("unexpected call #%d", calls)
		}
	})

	issues, err := c.SearchAll(context.Background(), "project = AI", nil, 50)
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 requests, got %d", calls)
	}
	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %d", len(issues))
	}
	if issues[0].Key != "AI-1" || issues[2].Key != "AI-3" {
		t.Errorf("unexpected issues: %+v", issues)
	}
}

// TestGetIssueEscapesKeyInPath is a defense-in-depth check: even though
// internal/service now rejects malformed issue keys before they ever reach
// this package, the low-level client itself must not let a key containing
// "/" or ".." route a request to a different URL path than the one the
// caller asked for.
func TestGetIssueEscapesKeyInPath(t *testing.T) {
	var gotRequestURI string
	var gotSegments int
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.RequestURI
		// r.URL.Path is unescaped by net/http (%2F decodes back to "/"),
		// so what matters for "did this stay one path segment" is how many
		// segments the *server's router* would see, i.e. r.URL.EscapedPath().
		gotSegments = strings.Count(r.URL.EscapedPath(), "/")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Issue{Key: "AI-1"})
	})

	key := "AI-1/../../rest/api/3/mypermissions"
	if _, err := c.GetIssue(context.Background(), key, nil); err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	// Unescaped, "/rest/api/3/issue/"+key would contain many "/" segments
	// and could route to a different endpoint. Escaped, the key's slashes
	// become literal "%2F" and the request stays under /rest/api/3/issue/.
	if !strings.Contains(gotRequestURI, "%2F") {
		t.Errorf("request URI = %q; key's \"/\" should have been percent-escaped", gotRequestURI)
	}
	wantSegments := strings.Count("/rest/api/3/issue/", "/") // just the fixed prefix
	if gotSegments != wantSegments {
		t.Errorf("escaped path has %d '/' segments, want %d (key must stay a single path segment)", gotSegments, wantSegments)
	}
}

// TestSearchAllStopsOnEmptyPage guards against a misbehaving server that
// reports isLast=false with a (possibly repeating) nextPageToken but no
// issues on the page: SearchAll must stop rather than loop forever.
func TestSearchAllStopsOnEmptyPage(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searchResponse{
			Issues:        nil,
			NextPageToken: "same-token",
			IsLast:        false,
		})
	})

	issues, err := c.SearchAll(context.Background(), "project = AI", nil, 50)
	if err != nil {
		t.Fatalf("SearchAll: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
	if calls != 1 {
		t.Errorf("expected SearchAll to stop after 1 call on an empty page, got %d calls", calls)
	}
}

func TestRetryOn429HonorsRetryAfter(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"errorMessages":["rate limited"]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(User{AccountID: "1"})
	})

	_, err := c.Myself(context.Background())
	if err != nil {
		t.Fatalf("Myself: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls (1 retry), got %d", calls)
	}
}

func TestRetryOn429ExhaustsAndReturnsError(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"errorMessages":["still limited"]}`))
	})

	_, err := c.Myself(context.Background())
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if calls != maxRetries {
		t.Errorf("expected %d calls, got %d", maxRetries, calls)
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d, want 429", apiErr.StatusCode)
	}
}

func TestErrorParsing(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"errorMessages":["Bad request"],"errors":{"summary":"is required"}}`))
	})

	_, err := c.Myself(context.Background())
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
	if len(apiErr.ErrorMessages) != 1 || apiErr.ErrorMessages[0] != "Bad request" {
		t.Errorf("ErrorMessages = %v", apiErr.ErrorMessages)
	}
	if apiErr.Errors["summary"] != "is required" {
		t.Errorf("Errors[summary] = %q", apiErr.Errors["summary"])
	}
}

func TestUnauthorizedMessage(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"errorMessages":["AUTHENTICATED_FAILED"]}`))
	})

	_, err := c.Myself(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if !apiErr.IsUnauthorized() {
		t.Errorf("expected IsUnauthorized() true")
	}
	if got := apiErr.Error(); got == "" {
		t.Errorf("expected non-empty message")
	}
}

func TestGetIssueSendsExplicitFields(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/AI-1" {
			t.Errorf("path = %q", r.URL.Path)
		}
		fields := r.URL.Query().Get("fields")
		if fields == "" {
			t.Error("expected non-empty fields query param")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(Issue{Key: "AI-1", Fields: IssueFields{Summary: "hi"}})
	})

	issue, err := c.GetIssue(context.Background(), "AI-1", nil)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.Key != "AI-1" || issue.Fields.Summary != "hi" {
		t.Errorf("unexpected issue: %+v", issue)
	}
}

func TestRetries5xxOnGETOnly(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"errorMessages":["down"]}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(User{AccountID: "ok"})
	})
	_, err := c.Myself(context.Background())
	if err != nil {
		t.Fatalf("Myself (GET, should retry 5xx): %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestResponseSizeCapped(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		chunk := strings.Repeat("a", 1<<20)
		for i := 0; i < 11; i++ {
			_, _ = w.Write([]byte(chunk))
		}
	})
	var out map[string]any
	err := c.get(context.Background(), "/x", &out)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size cap error, got %v", err)
	}
}
