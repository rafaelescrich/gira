package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

// TestPermissionsMergesAcrossAllowedProjects covers Permissions() calling
// GET /rest/api/3/mypermissions once per AllowedProjects entry and
// OR-ing the results together (a permission granted on any allowed
// project counts as granted overall).
func TestPermissionsMergesAcrossAllowedProjects(t *testing.T) {
	var gotProjects []string
	svc := newTestService(t, Options{AllowedProjects: []string{"AI", "OPS"}, DefaultProject: "AI"},
		func(w http.ResponseWriter, r *http.Request) {
			project := r.URL.Query().Get("projectKey")
			gotProjects = append(gotProjects, project)
			w.Header().Set("Content-Type", "application/json")
			perms := map[string]any{}
			switch project {
			case "AI":
				perms["CREATE_ISSUES"] = map[string]any{"havePermission": true}
			case "OPS":
				perms["ADD_COMMENTS"] = map[string]any{"havePermission": true}
			}
			json.NewEncoder(w).Encode(map[string]any{"permissions": perms})
		})

	got, err := svc.Permissions(context.Background())
	if err != nil {
		t.Fatalf("Permissions: %v", err)
	}
	if len(gotProjects) != 2 {
		t.Errorf("expected one mypermissions call per allowed project, got %v", gotProjects)
	}
	if !got["CREATE_ISSUES"] {
		t.Error("CREATE_ISSUES should be granted (from project AI)")
	}
	if !got["ADD_COMMENTS"] {
		t.Error("ADD_COMMENTS should be granted (from project OPS)")
	}
	if got["EDIT_ISSUES"] {
		t.Error("EDIT_ISSUES should not be granted by either project")
	}
	for _, k := range PermissionKeys {
		if _, ok := got[k]; !ok {
			t.Errorf("Permissions() result missing key %q", k)
		}
	}
}

func TestPermissionsRequiresAllowedProjectsOrDefault(t *testing.T) {
	svc := &Service{}
	if _, err := svc.Permissions(context.Background()); err == nil {
		t.Error("expected a usage error with no allowed_projects/default_project configured")
	} else if ExitCode(err) != 1 {
		t.Errorf("ExitCode = %d, want 1 (usage)", ExitCode(err))
	}
}
