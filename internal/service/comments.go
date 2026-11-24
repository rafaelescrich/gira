package service

import (
	"context"
	"strings"

	"github.com/rafaelescrich/gira/internal/jira"
)

// AddComment posts a comment (Markdown, converted to ADF) to key.
func (s *Service) AddComment(ctx context.Context, key, markdown string) (jira.Comment, error) {
	if err := s.requireWritable(); err != nil {
		return jira.Comment{}, err
	}
	key = strings.ToUpper(strings.TrimSpace(key))
	if err := s.checkKeyAllowed(key); err != nil {
		return jira.Comment{}, err
	}
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return jira.Comment{}, newUsageError("comment text is required")
	}

	doc := jira.MarkdownToADF(markdown)
	c, err := s.Client.AddComment(ctx, key, doc)
	if err != nil {
		return jira.Comment{}, wrapAPIError(err)
	}
	return c, nil
}

// ListComments returns key's comments (read-only; still subject to the
// project allowlist). Used by `gira issue view --comments`.
func (s *Service) ListComments(ctx context.Context, key string) ([]jira.Comment, error) {
	key = strings.ToUpper(strings.TrimSpace(key))
	if err := s.checkKeyAllowed(key); err != nil {
		return nil, err
	}
	comments, err := s.Client.ListComments(ctx, key)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	return comments, nil
}
