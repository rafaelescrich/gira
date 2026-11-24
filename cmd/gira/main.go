// Command gira is a CLI (and, via "gira mcp", an MCP server) for Jira
// Cloud.
package main

import (
	"os"

	"github.com/rafaelescrich/gira/internal/cli"
	"github.com/rafaelescrich/gira/internal/jira"
)

// version is overridden at build time via:
//
//	go build -ldflags "-X main.version=v0.1.0"
var version = "dev"

func main() {
	cli.SetVersion(version)
	jira.Version = version

	os.Exit(cli.Execute())
}
