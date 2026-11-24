# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately using GitHub's "Report a vulnerability"
(private security advisory) feature on this repository. Do not open a public
issue. Never include API tokens, raw Jira payloads or customer data in reports.

## Scope and design

- API tokens are stored in the OS keychain and never written to the config file.
- `site` must be https on `*.atlassian.net` / `api.atlassian.com` unless
  explicitly overridden with `allow_any_site`.
- `allowed_projects` and `read_only` are client-side guardrails, not a
  substitute for a least-privileged Jira API token.
- The MCP server uses stdio only (no network listener).
