package cli

import "testing"

func TestFormatShortDate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string // "" means "don't check exact value, just that it's non-empty/parsed"
	}{
		{name: "empty", in: "", want: ""},
		{name: "garbage", in: "not-a-date", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatShortDate(tt.in)
			if got != tt.want {
				t.Errorf("formatShortDate(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}

	// A well-formed Jira timestamp should parse into just its date part,
	// regardless of the local timezone the test runs in.
	if got := formatShortDate("2026-09-24T11:53:37.056-0500"); len(got) != len("2006-01-02") {
		t.Errorf("formatShortDate(valid timestamp) = %q, want a YYYY-MM-DD date", got)
	}
}

func TestFormatLocalDateTime(t *testing.T) {
	if got := formatLocalDateTime(""); got != "" {
		t.Errorf("formatLocalDateTime(\"\") = %q, want \"\"", got)
	}
	// Unparsable input is returned unchanged rather than silently dropped.
	if got := formatLocalDateTime("not-a-date"); got != "not-a-date" {
		t.Errorf("formatLocalDateTime(garbage) = %q, want input echoed back", got)
	}
	if got := formatLocalDateTime("2026-09-24T11:53:37.056-0500"); len(got) != len("2006-01-02 15:04") {
		t.Errorf("formatLocalDateTime(valid timestamp) = %q, want \"YYYY-MM-DD HH:MM\"", got)
	}
}
