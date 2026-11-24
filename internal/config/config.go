// Package config loads gira's configuration from flags, environment
// variables, and a YAML file, in that order of precedence.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultOutput is the default output format. There are intentionally no
// default site, project or board: they must be configured.
const DefaultOutput = "table"

// Config is gira's resolved configuration after applying precedence:
// flags > GIRA_* environment variables > config file > defaults.
type Config struct {
	Site            string   `yaml:"site"`
	Email           string   `yaml:"email"`
	DefaultProject  string   `yaml:"default_project"`
	DefaultBoard    int      `yaml:"default_board"`
	AllowedProjects []string `yaml:"allowed_projects"`
	ReadOnly        bool     `yaml:"read_only"`
	Output          string   `yaml:"output"` // "table" | "json"

	// AllowAnySite disables the Site host/scheme validation. Off by default
	// so credentials are never sent to a non-Atlassian or plaintext host.
	AllowAnySite bool `yaml:"allow_any_site"`
}

// fileConfig mirrors Config for YAML decoding, using pointers for fields
// where an explicit false/zero in the file must override a default.
type fileConfig struct {
	Site            string   `yaml:"site"`
	Email           string   `yaml:"email"`
	DefaultProject  string   `yaml:"default_project"`
	DefaultBoard    int      `yaml:"default_board"`
	AllowedProjects []string `yaml:"allowed_projects"`
	ReadOnly        *bool    `yaml:"read_only"`
	Output          string   `yaml:"output"`
	AllowAnySite    *bool    `yaml:"allow_any_site"`
}

// Overrides carries values sourced from CLI flags. A pointer field left nil
// means "flag not set"; only explicitly-set flags take precedence over
// environment and file values.
type Overrides struct {
	Site       *string
	Project    *string
	Board      *int
	ReadOnly   *bool
	Output     *string
	ConfigPath *string
}

// DefaultPath returns the default config file location, ~/.config/gira/config.yaml.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".config", "gira", "config.yaml"), nil
}

// defaults returns the built-in default configuration.
func defaults() Config {
	return Config{
		ReadOnly: false,
		Output:   DefaultOutput,
	}
}

// Load resolves configuration with precedence flags > env > file > defaults.
func Load(ov Overrides) (Config, error) {
	cfg := defaults()

	// 1. Config file (lowest precedence above defaults).
	path := ""
	if ov.ConfigPath != nil && *ov.ConfigPath != "" {
		path = *ov.ConfigPath
	} else {
		p, err := DefaultPath()
		if err != nil {
			return Config{}, err
		}
		path = p
	}

	if data, err := os.ReadFile(path); err == nil {
		var fileCfg fileConfig
		if err := yaml.Unmarshal(data, &fileCfg); err != nil {
			return Config{}, fmt.Errorf("parse config file %s: %w", path, err)
		}
		mergeFile(&cfg, fileCfg)
	} else if !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("read config file %s: %w", path, err)
	}

	// 2. Environment variables.
	if v := os.Getenv("GIRA_SITE"); v != "" {
		cfg.Site = v
	}
	if v := os.Getenv("GIRA_EMAIL"); v != "" {
		cfg.Email = v
	}
	if v := os.Getenv("GIRA_DEFAULT_PROJECT"); v != "" {
		cfg.DefaultProject = v
	}
	if v := os.Getenv("GIRA_DEFAULT_BOARD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.DefaultBoard = n
		}
	}
	if v := os.Getenv("GIRA_ALLOWED_PROJECTS"); v != "" {
		cfg.AllowedProjects = splitAndTrim(v)
	}
	if v := os.Getenv("GIRA_READ_ONLY"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.ReadOnly = b
		}
	}
	if v := os.Getenv("GIRA_OUTPUT"); v != "" {
		cfg.Output = v
	}
	if v := os.Getenv("GIRA_ALLOW_ANY_SITE"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.AllowAnySite = b
		}
	}

	// 3. Flags (highest precedence).
	if ov.Site != nil && *ov.Site != "" {
		cfg.Site = *ov.Site
	}
	if ov.Project != nil && *ov.Project != "" {
		cfg.DefaultProject = *ov.Project
	}
	if ov.Board != nil {
		cfg.DefaultBoard = *ov.Board
	}
	if ov.ReadOnly != nil {
		cfg.ReadOnly = *ov.ReadOnly
	}
	if ov.Output != nil && *ov.Output != "" {
		cfg.Output = *ov.Output
	}

	if len(cfg.AllowedProjects) == 0 && cfg.DefaultProject != "" {
		cfg.AllowedProjects = []string{cfg.DefaultProject}
	}

	if err := ValidateSite(cfg.Site, cfg.AllowAnySite); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// ValidateSite refuses site URLs that would send the API token somewhere
// unexpected: it must be https and, unless allowAny is set, its host must be
// *.atlassian.net or api.atlassian.com.
func ValidateSite(site string, allowAny bool) error {
	if strings.TrimSpace(site) == "" {
		return fmt.Errorf("no Jira site configured; set `site` in the config file, GIRA_SITE (e.g. https://your-org.atlassian.net)")
	}
	if allowAny {
		return nil
	}
	u, err := url.Parse(site)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid site URL %q", site)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("site %q must use https (set allow_any_site / GIRA_ALLOW_ANY_SITE=true to override)", site)
	}
	if u.User != nil {
		return fmt.Errorf("site %q must not contain credentials", site)
	}
	host := strings.ToLower(u.Hostname())
	if host == "api.atlassian.com" || strings.HasSuffix(host, ".atlassian.net") {
		return nil
	}
	return fmt.Errorf("site host %q is not *.atlassian.net or api.atlassian.com (set allow_any_site / GIRA_ALLOW_ANY_SITE=true to override)", host)
}

// mergeFile overlays set fields from file onto cfg.
func mergeFile(cfg *Config, file fileConfig) {
	if file.Site != "" {
		cfg.Site = file.Site
	}
	if file.Email != "" {
		cfg.Email = file.Email
	}
	if file.DefaultProject != "" {
		cfg.DefaultProject = file.DefaultProject
	}
	if file.DefaultBoard != 0 {
		cfg.DefaultBoard = file.DefaultBoard
	}
	if len(file.AllowedProjects) > 0 {
		cfg.AllowedProjects = file.AllowedProjects
	}
	if file.ReadOnly != nil {
		cfg.ReadOnly = *file.ReadOnly
	}
	if file.AllowAnySite != nil {
		cfg.AllowAnySite = *file.AllowAnySite
	}
	if file.Output != "" {
		cfg.Output = file.Output
	}
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Save writes cfg to path (or the default path if empty) as YAML, creating
// parent directories as needed. It never writes secrets: Config has no
// token field.
func Save(cfg Config, path string) error {
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return err
		}
		path = p
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config file %s: %w", path, err)
	}
	return nil
}
