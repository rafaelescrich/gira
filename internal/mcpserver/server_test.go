package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	gosdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rafaelescrich/gira/internal/config"
	"github.com/rafaelescrich/gira/internal/jira"
	"github.com/rafaelescrich/gira/internal/service"
)

// --- Fake Jira server ----------------------------------------------------

// newFakeJira builds an httptest server covering every endpoint the MCP
// tools under test may call, with fixed, minimal fixtures. permOverride
// selectively overrides GET /rest/api/3/mypermissions' "havePermission"
// values (unset keys default to true), letting tests simulate a
// permission-limited account.
func newFakeJira(t *testing.T, permOverride map[string]bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	writeJSON := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}

	mux.HandleFunc("GET /rest/api/3/myself", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, jira.User{AccountID: "acc-1", DisplayName: "Rafael Test", Email: "rafael@example.com", Active: true})
	})

	mux.HandleFunc("POST /rest/api/3/search/jql", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"issues": []jira.Issue{
				{Key: "AI-1", Fields: jira.IssueFields{Summary: "First issue", Status: jira.Status{Name: "To Do"}, IssueType: jira.IssueType{Name: "Task"}}},
			},
			"isLast": true,
		})
	})

	mux.HandleFunc("GET /rest/api/3/issue/createmeta/{project}/issuetypes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"issueTypes": []jira.CreateMetaIssueType{
				{ID: "1", Name: "Task"}, {ID: "2", Name: "Bug"}, {ID: "3", Name: "Epic"}, {ID: "4", Name: "Story"},
			},
		})
	})

	mux.HandleFunc("POST /rest/api/3/issue", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, jira.IssueRef{ID: "999", Key: "AI-99", Self: "https://example.test/issue/999"})
	})

	mux.HandleFunc("GET /rest/api/3/user/search", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, []jira.User{{AccountID: "acc-2", DisplayName: "Bob Bobson", Email: "bob@example.com", Active: true}})
	})

	mux.HandleFunc("GET /rest/api/3/issue/{key}", func(w http.ResponseWriter, r *http.Request) {
		key := r.PathValue("key")
		writeJSON(w, jira.Issue{
			Key: key,
			Fields: jira.IssueFields{
				Summary:   "Sample issue " + key,
				Status:    jira.Status{Name: "In Progress"},
				IssueType: jira.IssueType{Name: "Task"},
				Assignee:  &jira.User{DisplayName: "Rafael Test"},
				Priority:  &jira.Priority{Name: "High"},
			},
		})
	})

	mux.HandleFunc("PUT /rest/api/3/issue/{key}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /rest/api/3/issue/{key}/transitions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"transitions": []jira.Transition{
				{ID: "11", Name: "Start Progress", To: jira.Status{ID: "3", Name: "In Progress"}},
				{ID: "21", Name: "Done", To: jira.Status{ID: "4", Name: "Done"}},
			},
		})
	})
	mux.HandleFunc("POST /rest/api/3/issue/{key}/transitions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("PUT /rest/api/3/issue/{key}/assignee", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /rest/api/3/issue/{key}/comment", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"comments": []jira.Comment{}})
	})
	mux.HandleFunc("POST /rest/api/3/issue/{key}/comment", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, jira.Comment{ID: "5001"})
	})

	mux.HandleFunc("GET /rest/agile/1.0/board/{id}/backlog", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"startAt": 0, "total": 1,
			"issues": []jira.Issue{{Key: "AI-2", Fields: jira.IssueFields{Summary: "Backlog issue", Status: jira.Status{Name: "To Do"}}}},
		})
	})
	mux.HandleFunc("GET /rest/agile/1.0/board/{id}/sprint", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"startAt": 0, "total": 1, "isLast": true,
			"values": []jira.Sprint{{ID: 42, Name: "September 2", State: "active"}},
		})
	})
	mux.HandleFunc("GET /rest/agile/1.0/board/{id}/epic", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"startAt": 0, "total": 1, "isLast": true,
			"values": []jira.Epic{{ID: 7, Key: "AI-3", Name: "Big epic", Summary: "Big epic"}},
		})
	})
	mux.HandleFunc("POST /rest/agile/1.0/sprint/{id}/issue", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /rest/agile/1.0/backlog/issue", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /rest/api/3/mypermissions", func(w http.ResponseWriter, r *http.Request) {
		requested := strings.Split(r.URL.Query().Get("permissions"), ",")
		perms := map[string]any{}
		for _, key := range requested {
			granted := true
			if permOverride != nil {
				if v, ok := permOverride[key]; ok {
					granted = v
				}
			}
			perms[key] = map[string]any{"id": key, "key": key, "havePermission": granted}
		}
		writeJSON(w, map[string]any{"permissions": perms})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testOptions() service.Options {
	return service.Options{
		AllowedProjects: []string{"AI"},
		DefaultProject:  "AI",
		DefaultBoard:    336,
	}
}

func testConfig() config.Config {
	return config.Config{
		Site:            "https://example.atlassian.net",
		DefaultProject:  "AI",
		DefaultBoard:    336,
		AllowedProjects: []string{"AI"},
	}
}

// connect starts server (built with mcpserver.New) and an in-memory client
// session against it, registering cleanup on t.
func connect(t *testing.T, server *gosdkmcp.Server) *gosdkmcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	t1, t2 := gosdkmcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	client := gosdkmcp.NewClient(&gosdkmcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// buildServer wires a fake Jira server + Service into a new MCP server
// with opts, returning the connected client session.
func buildServer(t *testing.T, opts Options, permOverride map[string]bool) *gosdkmcp.ClientSession {
	t.Helper()
	fake := newFakeJira(t, permOverride)
	client := jira.New(fake.URL, "me@example.com", "token")
	svc := service.New(client, testOptions())
	server := New(context.Background(), svc, testConfig(), opts)
	return connect(t, server)
}

func toolNames(t *testing.T, session *gosdkmcp.ClientSession) []string {
	t.Helper()
	var names []string
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Tools: %v", err)
		}
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

var allToolNames = []string{
	"jira_add_comment",
	"jira_assign_issue",
	"jira_create_issue",
	"jira_get_backlog",
	"jira_get_issue",
	"jira_list_epics",
	"jira_list_sprints",
	"jira_list_transitions",
	"jira_move_to_backlog",
	"jira_move_to_sprint",
	"jira_search_issues",
	"jira_transition_issue",
	"jira_update_issue",
	"jira_whoami",
}

var readOnlyToolNames = []string{
	"jira_get_backlog",
	"jira_get_issue",
	"jira_list_epics",
	"jira_list_sprints",
	"jira_list_transitions",
	"jira_search_issues",
	"jira_whoami",
}

// --- Tool list per mode ---------------------------------------------------

func TestToolList_AllToolsMode(t *testing.T) {
	session := buildServer(t, Options{AllTools: true}, nil)
	got := toolNames(t, session)
	assertNamesEqual(t, got, allToolNames)
}

func TestToolList_ReadOnlyMode(t *testing.T) {
	session := buildServer(t, Options{ReadOnly: true}, nil)
	got := toolNames(t, session)
	assertNamesEqual(t, got, readOnlyToolNames)
}

// TestToolList_PermissionLimited mirrors BLUEPRINT.md §11's live
// permissions (only ADD_COMMENTS granted): only jira_add_comment should
// join the read-only tools.
func TestToolList_PermissionLimited(t *testing.T) {
	perms := map[string]bool{
		"CREATE_ISSUES":     false,
		"EDIT_ISSUES":       false,
		"TRANSITION_ISSUES": false,
		"ASSIGN_ISSUES":     false,
		"ADD_COMMENTS":      true,
		"SCHEDULE_ISSUES":   false,
	}
	session := buildServer(t, Options{}, perms)
	got := toolNames(t, session)
	want := append(append([]string{}, readOnlyToolNames...), "jira_add_comment")
	sort.Strings(want)
	assertNamesEqual(t, got, want)
}

// TestToolList_PermissionCheckFailsOpen simulates the mypermissions call
// itself failing (by pointing the service at a project outside anything
// the fake server understands isn't feasible here, so instead we break the
// call by requesting an unconfigured project) — registering all write
// tools rather than silently hiding them.
func TestToolList_PermissionCheckFailsOpen(t *testing.T) {
	fake := newFakeJira(t, nil)
	// A client whose base URL 404s the mypermissions endpoint specifically
	// simulates a failed check without touching the other fixtures.
	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/api/3/mypermissions" {
			http.Error(w, `{"errorMessages":["boom"]}`, http.StatusInternalServerError)
			return
		}
		fake.Config.Handler.ServeHTTP(w, r)
	}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := jira.New(srv.URL, "me@example.com", "token")
	svc := service.New(client, testOptions())
	server := New(context.Background(), svc, testConfig(), Options{})
	session := connect(t, server)

	got := toolNames(t, session)
	assertNamesEqual(t, got, allToolNames)
}

func assertNamesEqual(t *testing.T, got, want []string) {
	t.Helper()
	sort.Strings(got)
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("tool count = %d, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("tool[%d] = %q, want %q\ngot:  %v\nwant: %v", i, got[i], want[i], got, want)
		}
	}
}

// --- Schemas ---------------------------------------------------------------

func TestToolSchemasPresent(t *testing.T) {
	session := buildServer(t, Options{AllTools: true}, nil)
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Tools: %v", err)
		}
		if tool.InputSchema == nil {
			t.Errorf("%s: nil InputSchema", tool.Name)
		}
		if tool.Annotations == nil {
			t.Errorf("%s: nil Annotations", tool.Name)
		}
	}
}

func TestSearchIssuesSchemaHasFilters(t *testing.T) {
	session := buildServer(t, Options{AllTools: true}, nil)
	schema := schemaOf(t, session, "jira_search_issues")
	for _, prop := range []string{"status", "assignee", "type", "epic", "sprint", "text", "jql", "limit", "updated_since", "created_since", "sort", "sort_asc"} {
		if _, ok := schema.properties[prop]; !ok {
			t.Errorf("jira_search_issues schema missing property %q", prop)
		}
	}
	if len(schema.required) != 0 {
		t.Errorf("jira_search_issues should have no required fields, got %v", schema.required)
	}
}

func TestCreateIssueSchemaRequiresTypeAndSummary(t *testing.T) {
	session := buildServer(t, Options{AllTools: true}, nil)
	schema := schemaOf(t, session, "jira_create_issue")
	want := map[string]bool{"type": true, "summary": true}
	got := map[string]bool{}
	for _, r := range schema.required {
		got[r] = true
	}
	for k := range want {
		if !got[k] {
			t.Errorf("jira_create_issue required = %v, want to include %q", schema.required, k)
		}
	}
	if got["project"] {
		t.Errorf("jira_create_issue: project should be optional")
	}
}

type toolSchema struct {
	properties map[string]any
	required   []string
}

func schemaOf(t *testing.T, session *gosdkmcp.ClientSession, name string) toolSchema {
	t.Helper()
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("Tools: %v", err)
		}
		if tool.Name != name {
			continue
		}
		b, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal schema: %v", err)
		}
		var raw struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		}
		if err := json.Unmarshal(b, &raw); err != nil {
			t.Fatalf("unmarshal schema: %v", err)
		}
		return toolSchema{properties: raw.Properties, required: raw.Required}
	}
	t.Fatalf("tool %q not found", name)
	return toolSchema{}
}

// --- Happy paths, one per tool ---------------------------------------------

func TestHappyPaths(t *testing.T) {
	session := buildServer(t, Options{AllTools: true}, nil)
	ctx := context.Background()

	call := func(t *testing.T, name string, args map[string]any) *gosdkmcp.CallToolResult {
		t.Helper()
		res, err := session.CallTool(ctx, &gosdkmcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: protocol error: %v", name, err)
		}
		if res.IsError {
			t.Fatalf("%s: unexpected tool error: %v", name, textOf(res))
		}
		return res
	}

	t.Run("whoami", func(t *testing.T) {
		res := call(t, "jira_whoami", nil)
		if !strings.Contains(textOf(res), "Rafael Test") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
	t.Run("search_issues", func(t *testing.T) {
		res := call(t, "jira_search_issues", map[string]any{"limit": 3})
		if !strings.Contains(textOf(res), "issue(s) found") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
	t.Run("get_issue", func(t *testing.T) {
		res := call(t, "jira_get_issue", map[string]any{"key": "AI-1"})
		if !strings.Contains(textOf(res), "AI-1") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
	t.Run("list_transitions", func(t *testing.T) {
		res := call(t, "jira_list_transitions", map[string]any{"key": "AI-1"})
		if !strings.Contains(textOf(res), "In Progress") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
	t.Run("get_backlog", func(t *testing.T) {
		res := call(t, "jira_get_backlog", nil)
		if !strings.Contains(textOf(res), "backlog issue") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
	t.Run("list_sprints", func(t *testing.T) {
		res := call(t, "jira_list_sprints", nil)
		if !strings.Contains(textOf(res), "sprint(s)") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
	t.Run("list_epics", func(t *testing.T) {
		res := call(t, "jira_list_epics", nil)
		if !strings.Contains(textOf(res), "epic(s)") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
	t.Run("create_issue", func(t *testing.T) {
		res := call(t, "jira_create_issue", map[string]any{"type": "Task", "summary": "New thing"})
		if !strings.Contains(textOf(res), "AI-99") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
	t.Run("update_issue", func(t *testing.T) {
		res := call(t, "jira_update_issue", map[string]any{"key": "AI-1", "summary": "Updated summary"})
		if !strings.Contains(textOf(res), "Updated AI-1") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
	t.Run("transition_issue", func(t *testing.T) {
		res := call(t, "jira_transition_issue", map[string]any{"key": "AI-1", "to_status": "In Progress"})
		if !strings.Contains(textOf(res), "In Progress") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
	t.Run("assign_issue", func(t *testing.T) {
		res := call(t, "jira_assign_issue", map[string]any{"key": "AI-1", "assignee": "unassigned"})
		if !strings.Contains(textOf(res), "AI-1") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
	t.Run("add_comment", func(t *testing.T) {
		res := call(t, "jira_add_comment", map[string]any{"key": "AI-1", "body_markdown": "hello"})
		if !strings.Contains(textOf(res), "AI-1") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
	t.Run("move_to_sprint", func(t *testing.T) {
		res := call(t, "jira_move_to_sprint", map[string]any{"sprint": "current", "keys": []string{"AI-1"}})
		if !strings.Contains(textOf(res), "September 2") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
	t.Run("move_to_backlog", func(t *testing.T) {
		res := call(t, "jira_move_to_backlog", map[string]any{"keys": []string{"AI-1"}})
		if !strings.Contains(textOf(res), "backlog") {
			t.Errorf("summary = %q", textOf(res))
		}
	})
}

// --- Errors -> IsError ------------------------------------------------------

func TestGuardrailErrorSetsIsError(t *testing.T) {
	session := buildServer(t, Options{AllTools: true}, nil)
	res, err := session.CallTool(context.Background(), &gosdkmcp.CallToolParams{
		Name:      "jira_get_issue",
		Arguments: map[string]any{"key": "OTHER-1"},
	})
	if err != nil {
		t.Fatalf("protocol error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError=true for a disallowed project key")
	}
	if !strings.Contains(textOf(res), "not in the allowed list") {
		t.Errorf("error text = %q, want a mention of the allowlist", textOf(res))
	}
}

func TestReadOnlyModeRefusesWrites(t *testing.T) {
	// Read-only mode doesn't register write tools at all; calling one by
	// name should fail as an unknown tool (a protocol-level error), not a
	// silently-succeeding write.
	session := buildServer(t, Options{ReadOnly: true}, nil)
	_, err := session.CallTool(context.Background(), &gosdkmcp.CallToolParams{
		Name:      "jira_add_comment",
		Arguments: map[string]any{"key": "AI-1", "body_markdown": "hi"},
	})
	if err == nil {
		t.Fatal("expected an error calling an unregistered write tool in read-only mode")
	}
}

// TestIssueSummaryIncludesCreated guards against IssueSummary silently
// dropping the created timestamp that jira_search_issues/get_backlog/etc.
// rows are built from.
func TestIssueSummaryIncludesCreated(t *testing.T) {
	iss := jira.Issue{
		Key: "AI-1",
		Fields: jira.IssueFields{
			Summary: "Sample",
			Created: "2026-09-01T00:00:00.000-0500",
			Updated: "2026-09-24T11:53:37.056-0500",
		},
	}
	got := issueSummary(iss)
	if got.Created != iss.Fields.Created {
		t.Errorf("issueSummary(...).Created = %q, want %q", got.Created, iss.Fields.Created)
	}
	if got.Updated != iss.Fields.Updated {
		t.Errorf("issueSummary(...).Updated = %q, want %q", got.Updated, iss.Fields.Updated)
	}
}

func textOf(res *gosdkmcp.CallToolResult) string {
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*gosdkmcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String()
}
