// Package cli implements gira's cobra-based command line interface. Every
// command here does two things only: parse input and format output. All
// Jira calls and safety checks live in internal/service, so the CLI and
// (future) MCP front ends behave identically.
//
// Subcommand files register themselves onto RootCmd, or onto an exported
// subtree such as IssueCmd or AuthCmd, from their own init(). This lets
// new command files (e.g. issue_write.go, agile.go) be added without
// editing this file.
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rafaelescrich/gira/internal/config"
	"github.com/rafaelescrich/gira/internal/jira"
	"github.com/rafaelescrich/gira/internal/secrets"
	"github.com/rafaelescrich/gira/internal/service"
)

// RootCmd is gira's root cobra command.
var RootCmd = &cobra.Command{
	Use:   "gira",
	Short: "gira manages Jira Cloud issues from the terminal",
	Long: "gira is a CLI (and, via \"gira mcp\", an MCP server) for a Jira Cloud site.\n" +
		"Configure the site and project in ~/.config/gira/config.yaml (see README).",
	SilenceUsage:  true,
	SilenceErrors: true,
}

var (
	flagJSON       bool
	flagProject    string
	flagBoard      int
	flagReadOnly   bool
	flagConfigPath string
)

func init() {
	RootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "output JSON instead of a table")
	RootCmd.PersistentFlags().StringVar(&flagProject, "project", "", "override the configured default project")
	RootCmd.PersistentFlags().IntVar(&flagBoard, "board", 0, "override the configured default board")
	RootCmd.PersistentFlags().BoolVar(&flagReadOnly, "read-only", false, "force read-only mode (refuse write operations)")
	RootCmd.PersistentFlags().StringVar(&flagConfigPath, "config", "", "path to config file (default ~/.config/gira/config.yaml)")
}

// SetVersion sets gira's version string, used by `gira version` and (via
// the caller also setting jira.Version) the HTTP User-Agent header.
func SetVersion(v string) { version = v }

// Execute runs the CLI to completion and returns a process exit code
// following gira's convention: 0 ok, 1 usage, 2 API, 3 auth, 4 guardrail.
func Execute() int {
	err := RootCmd.Execute()
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, "error:", err)
	return service.ExitCode(err)
}

// LoadConfig resolves configuration for cmd, applying any explicitly-set
// global flags on top of GIRA_* environment variables and the config file.
func LoadConfig(cmd *cobra.Command) (config.Config, error) {
	flags := cmd.Flags()
	ov := config.Overrides{}

	if flags.Changed("project") {
		v := flagProject
		ov.Project = &v
	}
	if flags.Changed("board") {
		v := flagBoard
		ov.Board = &v
	}
	if flags.Changed("read-only") {
		v := flagReadOnly
		ov.ReadOnly = &v
	}
	if flags.Changed("json") && flagJSON {
		v := "json"
		ov.Output = &v
	}
	if flags.Changed("config") {
		v := flagConfigPath
		ov.ConfigPath = &v
	}

	return config.Load(ov)
}

// ResolveCredentials determines the email/API token pair to authenticate
// with: cfg.Email (already resolved through GIRA_EMAIL by config.Load)
// paired with GIRA_API_TOKEN if set, otherwise the token stored in the OS
// keychain for that email. The token is never logged.
func ResolveCredentials(cfg config.Config) (email, token string, err error) {
	email = cfg.Email
	if email == "" {
		return "", "", fmt.Errorf("no Jira account email configured; run `gira auth login` or set GIRA_EMAIL")
	}

	if tok := os.Getenv("GIRA_API_TOKEN"); tok != "" {
		return email, tok, nil
	}

	tok, err := secrets.GetToken(email)
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) {
			return "", "", fmt.Errorf("no API token stored for %s; run `gira auth login`", email)
		}
		return "", "", fmt.Errorf("read token from keychain: %w", err)
	}
	return email, tok, nil
}

// GetService is the standard entry point command implementations use to
// get a ready-to-call *service.Service plus the resolved config (useful
// for output formatting decisions).
func GetService(cmd *cobra.Command) (*service.Service, config.Config, error) {
	cfg, err := LoadConfig(cmd)
	if err != nil {
		return nil, config.Config{}, err
	}
	email, token, err := ResolveCredentials(cfg)
	if err != nil {
		return nil, cfg, err
	}
	client := jira.New(cfg.Site, email, token)
	svc := service.New(client, service.Options{
		AllowedProjects: cfg.AllowedProjects,
		DefaultProject:  cfg.DefaultProject,
		DefaultBoard:    cfg.DefaultBoard,
		ReadOnly:        cfg.ReadOnly,
	})
	return svc, cfg, nil
}
