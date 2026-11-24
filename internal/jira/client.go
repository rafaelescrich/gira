// Package jira is a thin REST client for Jira Cloud platform API v3 (and,
// eventually, the Agile API). It owns HTTP concerns only: auth, retries,
// pagination and error parsing. Business rules (allowlists, read-only,
// name resolution) live in internal/service.
package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// Version is the gira client version, sent as part of the User-Agent
// header. Overridden at build time via -ldflags where useful.
var Version = "dev"

const (
	defaultTimeout = 30 * time.Second

	// maxResponseBytes caps how much of a response body is read.
	maxResponseBytes = 10 << 20 // 10 MiB
	maxRetries       = 4        // total attempts on 429 / retryable 5xx
)

// Client is a thin, low-level Jira Cloud REST client.
type Client struct {
	baseURL    string
	email      string
	token      string
	httpClient *http.Client
	userAgent  string

	// now and sleep are overridable for tests.
	sleep func(time.Duration)
}

// New creates a Client for the given site base URL (e.g.
// "https://example.atlassian.net"), authenticating with HTTP Basic using
// email and API token.
func New(baseURL, email, token string) *Client {
	return &Client{
		baseURL: trimTrailingSlash(baseURL),
		email:   email,
		token:   token,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		userAgent: "gira/" + Version,
		sleep:     time.Sleep,
	}
}

func trimTrailingSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// SetHTTPClient overrides the underlying *http.Client. Primarily for tests
// that point at an httptest.Server; production callers should not usually
// need this.
func (c *Client) SetHTTPClient(h *http.Client) {
	c.httpClient = h
}

// doJSON performs an HTTP request against path (relative to the site base
// URL), optionally marshaling body as the JSON request body, and unmarshals
// a JSON response into out (if non-nil). It retries on 429 (honoring
// Retry-After) and, for GET requests only, on 5xx responses, using
// exponential backoff with jitter, up to maxRetries attempts total.
func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("jira: marshal request body: %w", err)
		}
		bodyBytes = b
	}

	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, respBody, err := c.doOnce(ctx, method, path, bodyBytes)
		if err != nil {
			// Network-level error: don't retry, context handles
			// cancellation/timeouts.
			return err
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = parseAPIError(resp.StatusCode, respBody)
			if attempt == maxRetries {
				return lastErr
			}
			wait := retryAfterDelay(resp.Header.Get("Retry-After"), attempt)
			if !c.waitOrDone(ctx, wait) {
				return ctx.Err()
			}
			continue
		}

		if method == http.MethodGet && resp.StatusCode >= 500 && resp.StatusCode < 600 {
			lastErr = parseAPIError(resp.StatusCode, respBody)
			if attempt == maxRetries {
				return lastErr
			}
			wait := backoffWithJitter(attempt)
			if !c.waitOrDone(ctx, wait) {
				return ctx.Err()
			}
			continue
		}

		if resp.StatusCode >= 400 {
			return parseAPIError(resp.StatusCode, respBody)
		}

		if out != nil && len(respBody) > 0 {
			if err := json.Unmarshal(respBody, out); err != nil {
				return fmt.Errorf("jira: decode response: %w", err)
			}
		}
		return nil
	}
	return lastErr
}

func (c *Client) doOnce(ctx context.Context, method, path string, bodyBytes []byte) (*http.Response, []byte, error) {
	var reader io.Reader
	if bodyBytes != nil {
		reader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, nil, fmt.Errorf("jira: build request: %w", err)
	}
	req.SetBasicAuth(c.email, c.token)
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	if bodyBytes != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("jira: request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, nil, fmt.Errorf("jira: read response body: %w", err)
	}
	if len(respBody) > maxResponseBytes {
		return nil, nil, fmt.Errorf("jira: response body exceeds %d bytes", maxResponseBytes)
	}
	return resp, respBody, nil
}

func (c *Client) waitOrDone(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	default:
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// retryAfterDelay parses a Retry-After header (seconds, per RFC 7231) and
// falls back to exponential backoff with jitter if absent or unparsable.
func retryAfterDelay(header string, attempt int) time.Duration {
	if header != "" {
		if secs, err := strconv.Atoi(header); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return backoffWithJitter(attempt)
}

// backoffWithJitter returns an exponential backoff duration (base 500ms)
// with up to 50% random jitter added. Jitter is not security-sensitive, so
// math/rand is appropriate.
func backoffWithJitter(attempt int) time.Duration {
	base := 500 * time.Millisecond
	backoff := base << (attempt - 1) // 500ms, 1s, 2s, 4s...
	jitter := time.Duration(rand.Int64N(int64(backoff) / 2))
	return backoff + jitter
}

// get performs a GET request and decodes the JSON response into out.
func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, nil, out)
}

// post performs a POST request with a JSON body and decodes the JSON
// response into out (which may be nil).
func (c *Client) post(ctx context.Context, path string, body any, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, body, out)
}
