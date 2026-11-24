package jira

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func strPtr(s string) *string { return &s }

func TestCreateIssueSendsFieldsAndDecodesRef(t *testing.T) {
	var gotBody issueFieldsRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/rest/api/3/issue" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(IssueRef{ID: "100", Key: "AI-1", Self: "https://x/issue/100"})
	})

	labels := []string{"gira-test"}
	doc := MarkdownToADF("hello")
	fields := IssueWriteFields{
		Project:     &Project{Key: "AI"},
		IssueType:   &IssueTypeRef{Name: "Task"},
		Summary:     strPtr("Do the thing"),
		Description: &doc,
		Labels:      &labels,
		Priority:    &Priority{Name: "High"},
		Parent:      &ParentRef{Key: "AI-3"},
	}

	ref, err := c.CreateIssue(context.Background(), fields)
	if err != nil {
		t.Fatalf("CreateIssue: %v", err)
	}
	if ref.Key != "AI-1" {
		t.Errorf("Key = %q, want AI-1", ref.Key)
	}
	if gotBody.Fields.Summary == nil || *gotBody.Fields.Summary != "Do the thing" {
		t.Errorf("summary not sent correctly: %+v", gotBody.Fields.Summary)
	}
	if gotBody.Fields.Project == nil || gotBody.Fields.Project.Key != "AI" {
		t.Errorf("project not sent correctly: %+v", gotBody.Fields.Project)
	}
	if gotBody.Fields.IssueType == nil || gotBody.Fields.IssueType.Name != "Task" {
		t.Errorf("issuetype not sent correctly: %+v", gotBody.Fields.IssueType)
	}
	if gotBody.Fields.Parent == nil || gotBody.Fields.Parent.Key != "AI-3" {
		t.Errorf("parent (epic link) not sent correctly: %+v", gotBody.Fields.Parent)
	}
}

func TestCreateIssueRequiresProjectTypeAndSummary(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not call the API when required fields are missing")
	})
	_, err := c.CreateIssue(context.Background(), IssueWriteFields{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEditIssueSendsPartialFieldsOnly(t *testing.T) {
	var raw json.RawMessage
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/rest/api/3/issue/AI-1" {
			t.Errorf("path = %s", r.URL.Path)
		}
		buf, _ := readAll(r)
		raw = buf
		w.WriteHeader(http.StatusNoContent)
	})

	summary := "New summary"
	err := c.EditIssue(context.Background(), "AI-1", IssueWriteFields{Summary: &summary})
	if err != nil {
		t.Fatalf("EditIssue: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode sent body: %v", err)
	}
	fields, ok := decoded["fields"].(map[string]any)
	if !ok {
		t.Fatalf("expected fields object, got %#v", decoded)
	}
	if len(fields) != 1 {
		t.Errorf("expected only 'summary' field sent, got %#v", fields)
	}
	if fields["summary"] != "New summary" {
		t.Errorf("summary = %v", fields["summary"])
	}
}

func TestAssigneeRefMarshalsExplicitNullForUnassign(t *testing.T) {
	b, err := json.Marshal(AssigneeRef{AccountID: nil})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(b) != `{"accountId":null}` {
		t.Errorf("got %s, want explicit null accountId", b)
	}
}

func TestGetCreateMetaIssueTypes(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/createmeta/AI/issuetypes" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(createMetaIssueTypesResponse{
			IssueTypes: []CreateMetaIssueType{{ID: "10001", Name: "Task"}, {ID: "10002", Name: "Bug"}},
		})
	})

	types, err := c.GetCreateMetaIssueTypes(context.Background(), "AI")
	if err != nil {
		t.Fatalf("GetCreateMetaIssueTypes: %v", err)
	}
	if len(types) != 2 || types[0].Name != "Task" {
		t.Errorf("unexpected types: %+v", types)
	}
}

func TestGetCreateMetaFieldsPages(t *testing.T) {
	calls := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		switch calls {
		case 1:
			json.NewEncoder(w).Encode(createMetaFieldsResponse{
				Values: []CreateMetaField{{FieldID: "summary", Name: "Summary", Required: true}},
				IsLast: false,
			})
		case 2:
			json.NewEncoder(w).Encode(createMetaFieldsResponse{
				Values: []CreateMetaField{{FieldID: "description", Name: "Description", Required: false}},
				IsLast: true,
			})
		default:
			t.Fatalf("unexpected call #%d", calls)
		}
	})

	fields, err := c.GetCreateMetaFields(context.Background(), "AI", "10001")
	if err != nil {
		t.Fatalf("GetCreateMetaFields: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("expected 2 fields across pages, got %d", len(fields))
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

// readAll is a tiny helper to read the full request body in tests.
func readAll(r *http.Request) ([]byte, error) {
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 1024)
	for {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}
