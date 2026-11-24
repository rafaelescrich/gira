package service

import (
	"context"
)

// PermissionKeys lists the Jira global/project permissions gated behind
// gira's write tools (blueprint §7 M4 item 6). Order matches BLUEPRINT.md
// §11's mypermissions table.
var PermissionKeys = []string{
	"CREATE_ISSUES",
	"EDIT_ISSUES",
	"TRANSITION_ISSUES",
	"ASSIGN_ISSUES",
	"ADD_COMMENTS",
	"SCHEDULE_ISSUES",
}

// Permissions reports, for each of PermissionKeys, whether the
// authenticated user holds it on at least one of AllowedProjects (falling
// back to DefaultProject if AllowedProjects is empty). It calls
// GET /rest/api/3/mypermissions once per allowed project and merges the
// results, since a token may be granted a permission on one allowed
// project but not another.
//
// This is a read-only call; it is not subject to the read-only guardrail.
func (s *Service) Permissions(ctx context.Context) (map[string]bool, error) {
	projects := s.AllowedProjects
	if len(projects) == 0 {
		if s.DefaultProject == "" {
			return nil, newUsageError("no allowed_projects or default_project configured")
		}
		projects = []string{s.DefaultProject}
	}

	merged := make(map[string]bool, len(PermissionKeys))
	for _, k := range PermissionKeys {
		merged[k] = false
	}

	for _, project := range projects {
		granted, err := s.Client.MyPermissions(ctx, project, PermissionKeys)
		if err != nil {
			return nil, wrapAPIError(err)
		}
		for _, k := range PermissionKeys {
			if granted[k] {
				merged[k] = true
			}
		}
	}
	return merged, nil
}
