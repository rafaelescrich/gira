package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/rafaelescrich/gira/internal/service"
)

func init() {
	AuthCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show the current Jira account and verify the stored token",
		RunE:  runAuthStatus,
	})
}

func runAuthStatus(cmd *cobra.Command, _ []string) error {
	svc, cfg, err := GetService(cmd)
	if err != nil {
		return err
	}

	me, err := svc.Me(cmd.Context())
	if err != nil {
		return err
	}

	if UseJSON(cmd, cfg) {
		return WriteJSON(cmd.OutOrStdout(), me)
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Logged in to %s as %s <%s>\n", cfg.Site, me.DisplayName, me.Email)
	fmt.Fprintf(out, "Account ID: %s\n", me.AccountID)
	fmt.Fprintf(out, "Default project: %s   Default board: %d   Read-only: %v\n", cfg.DefaultProject, cfg.DefaultBoard, cfg.ReadOnly)

	if perms, permErr := svc.Permissions(cmd.Context()); permErr == nil {
		names := make([]string, len(service.PermissionKeys))
		for i, k := range service.PermissionKeys {
			mark := "no"
			if perms[k] {
				mark = "yes"
			}
			names[i] = fmt.Sprintf("%s=%s", k, mark)
		}
		fmt.Fprintf(out, "Permissions (%s): %s\n", strings.Join(cfg.AllowedProjects, ","), strings.Join(names, "  "))
	} else {
		fmt.Fprintf(out, "Permissions: could not check (%v)\n", permErr)
	}
	return nil
}
