package mcpserver

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rafaelescrich/gira/internal/config"
	"github.com/rafaelescrich/gira/internal/jira"
	"github.com/rafaelescrich/gira/internal/service"
)

// Options configures how New builds the MCP server.
type Options struct {
	// ReadOnly, when true, registers only the read-only tools (mirrors
	// `gira mcp --read-only` / config read_only).
	ReadOnly bool
	// AllTools, when true, skips the mypermissions check and registers
	// every write tool regardless of the account's Jira permissions
	// (`gira mcp --all-tools`). Has no effect when ReadOnly is set.
	AllTools bool
	// Logger receives server activity. Defaults to a logger that writes to
	// os.Stderr if nil. Never write anything to stdout: it carries the MCP
	// protocol.
	Logger *slog.Logger
}

func (o Options) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.Default()
}

// writeToolPermission maps each write tool to the mypermissions key that
// gates its registration (blueprint §7 M4 item 6).
var writeToolPermission = map[string]string{
	"jira_create_issue":     "CREATE_ISSUES",
	"jira_update_issue":     "EDIT_ISSUES",
	"jira_transition_issue": "TRANSITION_ISSUES",
	"jira_assign_issue":     "ASSIGN_ISSUES",
	"jira_add_comment":      "ADD_COMMENTS",
	"jira_move_to_sprint":   "SCHEDULE_ISSUES",
	"jira_move_to_backlog":  "SCHEDULE_ISSUES",
}

// writeToolOrder fixes registration/logging order (map iteration order in
// Go is randomized, and stable tool listings make tests and transcripts
// easier to read).
var writeToolOrder = []string{
	"jira_create_issue",
	"jira_update_issue",
	"jira_transition_issue",
	"jira_assign_issue",
	"jira_add_comment",
	"jira_move_to_sprint",
	"jira_move_to_backlog",
}

// New builds an MCP server around svc. cfg supplies the defaults mentioned
// in the server's instructions (default project/board). Write tools are
// registered according to opts.ReadOnly, opts.AllTools, and (barring
// either of those) the account's live Jira permissions, checked via
// svc.Permissions — see registerWriteTools.
func New(ctx context.Context, svc *service.Service, cfg config.Config, opts Options) *mcp.Server {
	log := opts.logger()

	impl := &mcp.Implementation{Name: "gira", Version: jira.Version}
	instructions := fmt.Sprintf(
		"gira manages Jira Cloud issues on %s. Defaults: project %s, board %d. "+
			"On team-managed projects, epics link to issues via the parent "+
			"field, not a separate epic-link field. Search results and key lists "+
			"are capped (search <=50, key lists <=20) to keep responses small; "+
			"call jira_get_issue for full detail on any one issue.",
		cfg.Site, cfg.DefaultProject, cfg.DefaultBoard,
	)

	server := mcp.NewServer(impl, &mcp.ServerOptions{
		Instructions: instructions,
		Logger:       log,
	})

	registerReadTools(server, svc)

	if opts.ReadOnly {
		log.Info("gira mcp: read-only mode, write tools not registered")
		return server
	}

	registerWriteTools(ctx, server, svc, opts)
	return server
}

// registerWriteTools registers each write tool gated by its mypermissions
// key, per blueprint §7 M4 item 6:
//   - opts.AllTools skips the check and registers everything.
//   - otherwise, svc.Permissions() is called once (one Jira request per
//     allowed project); a failed check fails open (registers everything,
//     with a warning logged to stderr) rather than silently hiding tools.
//   - a tool whose permission is not granted is skipped, with an Info log
//     line naming the tool and the missing permission.
func registerWriteTools(ctx context.Context, server *mcp.Server, svc *service.Service, opts Options) {
	log := opts.logger()

	perms := make(map[string]bool, len(service.PermissionKeys))
	switch {
	case opts.AllTools:
		for _, k := range service.PermissionKeys {
			perms[k] = true
		}
		log.Info("gira mcp: --all-tools set, skipping mypermissions check")
	default:
		p, err := svc.Permissions(ctx)
		if err != nil {
			log.Warn("gira mcp: mypermissions check failed; registering all write tools (fail open)", "error", err)
			for _, k := range service.PermissionKeys {
				perms[k] = true
			}
		} else {
			perms = p
		}
	}

	for _, name := range writeToolOrder {
		permKey := writeToolPermission[name]
		if !perms[permKey] {
			log.Info("gira mcp: skipping write tool (permission not granted)", "tool", name, "permission", permKey)
			continue
		}
		addWriteTool(server, svc, name)
	}
}

// addWriteTool registers a single write tool by name. Kept as a switch
// (rather than a registration func stored per-entry) so each add*Tool
// function below can keep its natural, fully-typed signature.
func addWriteTool(server *mcp.Server, svc *service.Service, name string) {
	switch name {
	case "jira_create_issue":
		addCreateIssueTool(server, svc)
	case "jira_update_issue":
		addUpdateIssueTool(server, svc)
	case "jira_transition_issue":
		addTransitionIssueTool(server, svc)
	case "jira_assign_issue":
		addAssignIssueTool(server, svc)
	case "jira_add_comment":
		addAddCommentTool(server, svc)
	case "jira_move_to_sprint":
		addMoveToSprintTool(server, svc)
	case "jira_move_to_backlog":
		addMoveToBacklogTool(server, svc)
	}
}

// boolPtr and falsePtr are small helpers for building *mcp.ToolAnnotations
// literals below (DestructiveHint is a *bool: nil means "use the spec
// default", which for a non-read-only tool is true).
func boolPtr(b bool) *bool { return &b }
