package cli

import (
	"log/slog"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	"github.com/rafaelescrich/gira/internal/mcpserver"
)

var mcpAllTools bool

func init() {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run gira as an MCP server over stdio",
		Long: "gira mcp runs an MCP (Model Context Protocol) server on stdio, exposing\n" +
			"Jira tools (jira_whoami, jira_search_issues, jira_get_issue, ...) to\n" +
			"Claude Code and other MCP clients.\n\n" +
			"Logging goes to stderr only; stdout carries the MCP protocol exclusively.\n\n" +
			"Use --read-only (a persistent gira flag, also settable via config\n" +
			"read_only: true) to register only the read-only tools. By default,\n" +
			"write tools are registered only if the account's current Jira\n" +
			"permissions (checked once at startup) allow them; pass --all-tools to\n" +
			"skip that check and register every write tool unconditionally.",
		Args: cobra.NoArgs,
		RunE: runMCP,
	}
	cmd.Flags().BoolVar(&mcpAllTools, "all-tools", false, "register every write tool, skipping the Jira permissions check")
	RootCmd.AddCommand(cmd)
}

func runMCP(cmd *cobra.Command, _ []string) error {
	svc, cfg, err := GetService(cmd)
	if err != nil {
		return err
	}

	// stdout is reserved for the MCP protocol (StdioTransport reads/writes
	// it directly); every log line must go to stderr instead.
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	ctx := cmd.Context()
	server := mcpserver.New(ctx, svc, cfg, mcpserver.Options{
		ReadOnly: cfg.ReadOnly,
		AllTools: mcpAllTools,
		Logger:   logger,
	})

	return server.Run(ctx, &mcp.StdioTransport{})
}
