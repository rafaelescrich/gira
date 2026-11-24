// Package secrets wraps macOS Keychain (and, via go-keyring, other OS
// credential stores) access for the Jira API token. The token is never
// written to the config file, logs, or command output.
package secrets

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// Service is the Keychain service name used for all gira credentials.
const Service = "gira"

// ErrNotFound is returned when no token is stored for the given account.
var ErrNotFound = keyring.ErrNotFound

// GetToken reads the API token for the given Atlassian account (email)
// from the OS keychain.
//
// IMPORTANT: callers must never print, log, or otherwise persist the
// returned value.
func GetToken(email string) (string, error) {
	if email == "" {
		return "", errors.New("secrets: email is required")
	}
	tok, err := keyring.Get(Service, email)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("secrets: read token: %w", err)
	}
	return tok, nil
}

// SetToken stores the API token for the given Atlassian account in the OS
// keychain, replacing any existing value.
//
// Do not use the `security` CLI's interactive -w prompt to populate this
// value out of band: it truncates input at 128 characters, which silently
// corrupts Atlassian API tokens (typically ~192 characters). Always go
// through this function (or `gira auth login`, which reads the token via a
// no-echo terminal prompt and calls this).
func SetToken(email, token string) error {
	if email == "" {
		return errors.New("secrets: email is required")
	}
	if token == "" {
		return errors.New("secrets: token is required")
	}
	if err := keyring.Set(Service, email, token); err != nil {
		return fmt.Errorf("secrets: store token: %w", err)
	}
	return nil
}

// DeleteToken removes the stored API token for the given Atlassian account.
func DeleteToken(email string) error {
	if email == "" {
		return errors.New("secrets: email is required")
	}
	if err := keyring.Delete(Service, email); err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("secrets: delete token: %w", err)
	}
	return nil
}
