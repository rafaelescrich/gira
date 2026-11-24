package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	// Point at a config path that doesn't exist so we only see defaults.
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	t.Setenv("GIRA_SITE", "https://example.atlassian.net")
	cfg, err := Load(Overrides{ConfigPath: &missing})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultProject != "" || cfg.DefaultBoard != 0 || len(cfg.AllowedProjects) != 0 {
		t.Errorf("expected no project/board defaults, got %+v", cfg)
	}
	if cfg.ReadOnly {
		t.Errorf("ReadOnly = true, want false")
	}
	if cfg.Output != DefaultOutput {
		t.Errorf("Output = %q, want %q", cfg.Output, DefaultOutput)
	}
}

func TestLoadRequiresSite(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	t.Setenv("GIRA_SITE", "")
	if _, err := Load(Overrides{ConfigPath: &missing}); err == nil {
		t.Fatal("expected error when no site is configured")
	}
}

func TestLoadFileOverridesDefaults(t *testing.T) {
	path := writeTempConfig(t, `
site: https://example.atlassian.net
email: file@example.com
default_project: FOO
default_board: 42
allowed_projects: [FOO, BAR]
read_only: true
output: json
`)
	cfg, err := Load(Overrides{ConfigPath: &path})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Site != "https://example.atlassian.net" {
		t.Errorf("Site = %q", cfg.Site)
	}
	if cfg.Email != "file@example.com" {
		t.Errorf("Email = %q", cfg.Email)
	}
	if cfg.DefaultProject != "FOO" {
		t.Errorf("DefaultProject = %q", cfg.DefaultProject)
	}
	if cfg.DefaultBoard != 42 {
		t.Errorf("DefaultBoard = %d", cfg.DefaultBoard)
	}
	if len(cfg.AllowedProjects) != 2 {
		t.Errorf("AllowedProjects = %v", cfg.AllowedProjects)
	}
	if !cfg.ReadOnly {
		t.Errorf("ReadOnly = false, want true")
	}
	if cfg.Output != "json" {
		t.Errorf("Output = %q", cfg.Output)
	}
}

func TestEnvOverridesFile(t *testing.T) {
	path := writeTempConfig(t, `
default_project: FOO
default_board: 42
`)
	t.Setenv("GIRA_SITE", "https://example.atlassian.net")
	t.Setenv("GIRA_DEFAULT_PROJECT", "ENVPROJ")
	t.Setenv("GIRA_DEFAULT_BOARD", "99")
	t.Setenv("GIRA_EMAIL", "env@example.com")

	cfg, err := Load(Overrides{ConfigPath: &path})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultProject != "ENVPROJ" {
		t.Errorf("DefaultProject = %q, want ENVPROJ (env should beat file)", cfg.DefaultProject)
	}
	if cfg.DefaultBoard != 99 {
		t.Errorf("DefaultBoard = %d, want 99", cfg.DefaultBoard)
	}
	if cfg.Email != "env@example.com" {
		t.Errorf("Email = %q", cfg.Email)
	}
}

func TestFlagsOverrideEnvAndFile(t *testing.T) {
	path := writeTempConfig(t, `
default_project: FOO
`)
	t.Setenv("GIRA_SITE", "https://example.atlassian.net")
	t.Setenv("GIRA_DEFAULT_PROJECT", "ENVPROJ")

	flagProject := "FLAGPROJ"
	cfg, err := Load(Overrides{ConfigPath: &path, Project: &flagProject})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultProject != "FLAGPROJ" {
		t.Errorf("DefaultProject = %q, want FLAGPROJ (flag should beat env+file)", cfg.DefaultProject)
	}
}

func TestBoardFlagZeroValueDoesNotOverride(t *testing.T) {
	// A *int pointing at 0 is an explicit override (flag was Changed), so
	// it should win. This documents that behavior.
	path := writeTempConfig(t, "site: https://example.atlassian.net\ndefault_board: 42")
	zero := 0
	cfg, err := Load(Overrides{ConfigPath: &path, Board: &zero})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultBoard != 0 {
		t.Errorf("DefaultBoard = %d, want 0 (explicit flag override)", cfg.DefaultBoard)
	}
}

func TestValidateSite(t *testing.T) {
	cases := []struct {
		site     string
		allowAny bool
		wantErr  bool
	}{
		{"https://example.atlassian.net", false, false},
		{"https://api.atlassian.com/ex/jira/abc", false, false},
		{"http://example.atlassian.net", false, true},
		{"https://evil.example.com", false, true},
		{"https://atlassian.net.evil.com", false, true},
		{"https://user:pw@example.atlassian.net", false, true},
		{"not a url", false, true},
		{"http://localhost:8080", true, false},
	}
	for _, tc := range cases {
		err := ValidateSite(tc.site, tc.allowAny)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateSite(%q, %v) err = %v, wantErr %v", tc.site, tc.allowAny, err, tc.wantErr)
		}
	}
}

func TestLoadRejectsBadSiteFromEnv(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "none.yaml")
	t.Setenv("GIRA_SITE", "http://evil.example.com")
	if _, err := Load(Overrides{ConfigPath: &missing}); err == nil {
		t.Fatal("expected error for non-atlassian http site")
	}
	t.Setenv("GIRA_ALLOW_ANY_SITE", "true")
	if _, err := Load(Overrides{ConfigPath: &missing}); err != nil {
		t.Fatalf("opt-out should allow site: %v", err)
	}
}

func TestFileReadOnlyFalseOverridesEnvDefault(t *testing.T) {
	cfg := Config{ReadOnly: true}
	f := false
	mergeFile(&cfg, fileConfig{ReadOnly: &f})
	if cfg.ReadOnly {
		t.Error("explicit read_only: false in file should override")
	}
}
