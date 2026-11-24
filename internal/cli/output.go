package cli

import (
	"encoding/json"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/rafaelescrich/gira/internal/config"
)

// NewTableWriter returns a tabwriter configured for gira's aligned table
// output. Callers write tab-separated rows and must call Flush() when
// done.
func NewTableWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
}

// WriteJSON encodes v as indented JSON to w.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// UseJSON reports whether a command should render JSON output: the --json
// flag takes precedence, then the resolved config's `output: json`.
func UseJSON(cmd *cobra.Command, cfg config.Config) bool {
	if cmd.Flags().Changed("json") {
		return flagJSON
	}
	return cfg.Output == "json"
}

// jiraTimestampLayout matches the timestamp format Jira Cloud uses for
// issue created/updated fields, e.g. "2026-09-24T11:53:37.056-0500".
const jiraTimestampLayout = "2006-01-02T15:04:05.000-0700"

// parseJiraTimestamp parses s using Jira's usual layout, falling back to
// plain RFC 3339 for robustness.
func parseJiraTimestamp(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(jiraTimestampLayout, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// formatShortDate renders a Jira created/updated timestamp as a short
// local date ("2006-01-02"), for table columns. Unparsable or empty input
// renders as "" rather than propagating a raw, hard-to-read timestamp.
func formatShortDate(s string) string {
	t, ok := parseJiraTimestamp(s)
	if !ok {
		return ""
	}
	return t.Local().Format("2006-01-02")
}

// formatLocalDateTime renders a Jira created/updated timestamp as a local
// date and time ("2006-01-02 15:04"), for single-issue text output.
// Unparsable or empty input is returned unchanged so nothing is silently
// dropped.
func formatLocalDateTime(s string) string {
	t, ok := parseJiraTimestamp(s)
	if !ok {
		return s
	}
	return t.Local().Format("2006-01-02 15:04")
}
