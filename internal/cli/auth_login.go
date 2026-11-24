package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/rafaelescrich/gira/internal/config"
	"github.com/rafaelescrich/gira/internal/secrets"
)

var authLoginEmail string

func init() {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store a Jira API token in the OS keychain",
		Long: `Prompts for your Atlassian API token without echoing it to the terminal,
and stores it in the OS keychain (service "gira").

Do not populate the token with 'security add-generic-password ... -w': its
interactive prompt truncates input at 128 characters, which silently
corrupts Atlassian API tokens (typically ~192 characters). This command
reads the full value itself via a no-echo terminal prompt.`,
		RunE: runAuthLogin,
	}
	cmd.Flags().StringVar(&authLoginEmail, "email", "", "Atlassian account email (default: configured email, or prompted)")
	AuthCmd.AddCommand(cmd)
}

func runAuthLogin(cmd *cobra.Command, _ []string) error {
	cfg, err := LoadConfig(cmd)
	if err != nil {
		return err
	}

	email := strings.TrimSpace(authLoginEmail)
	if email == "" {
		email = cfg.Email
	}
	if email == "" {
		email, err = promptLine(cmd, "Atlassian account email: ")
		if err != nil {
			return err
		}
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return fmt.Errorf("email is required")
	}

	token, err := promptSecret(cmd, fmt.Sprintf("API token for %s: ", email))
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return fmt.Errorf("token is required")
	}

	if err := secrets.SetToken(email, token); err != nil {
		return err
	}

	// Persist the email (never the token) to the config file so future
	// commands don't need --email or GIRA_EMAIL.
	if cfg.Email != email {
		cfg.Email = email
		if err := config.Save(cfg, flagConfigPath); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: token stored, but failed to save email to config: %v\n", err)
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Stored API token for %s in the OS keychain.\n", email)
	return nil
}

// promptLine reads a single line of (echoed) input, e.g. for the account
// email.
func promptLine(cmd *cobra.Command, prompt string) (string, error) {
	fmt.Fprint(cmd.OutOrStdout(), prompt)
	reader := bufio.NewReader(cmd.InOrStdin())
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// promptSecret reads a single line without echoing it to the terminal,
// via golang.org/x/term. When stdin isn't an interactive terminal (e.g.
// piped input, as in tests or non-interactive automation), it falls back
// to a plain line read since there is no TTY to suppress echo on.
func promptSecret(cmd *cobra.Command, prompt string) (string, error) {
	fmt.Fprint(cmd.OutOrStdout(), prompt)

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return promptLine(cmd, "")
	}

	tokenBytes, err := term.ReadPassword(fd)
	fmt.Fprintln(cmd.OutOrStdout())
	if err != nil {
		return "", fmt.Errorf("read token: %w", err)
	}
	return string(tokenBytes), nil
}
