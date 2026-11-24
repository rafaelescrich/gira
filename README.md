# gira

A Go CLI and MCP server for Jira Cloud. It works with any Jira Cloud site,
project and Scrum/Kanban board: nothing is hardcoded, so you configure your
own site, project(s) and board.

## Compatibility

- **Jira Cloud only** (REST API v3 and Agile 1.0), authenticated with an
  Atlassian email + API token. Jira Server / Data Center is not supported.
- **Any project and board.** Set `site`, `default_project`, `default_board` and
  `allowed_projects` for your own instance.
- **Epics** are linked through the `parent` field, which Jira Cloud supports
  for both team-managed and company-managed projects. Boards that don't
  support the epics endpoint fall back to a JQL search for `issuetype = Epic`.
- **Sprints/backlog** need a board that supports them (Scrum, or Kanban with
  a backlog); other boards return a Jira API error.
- Issue type, priority and status names are passed through as you type them,
  so they must match your project's configuration.

## Build

Requires Go 1.27+.

```sh
make build      # builds ./gira
make test       # go test ./...
make vet        # go vet ./...
make install    # builds and copies to ~/go/bin/gira
```

Or directly:

```sh
go build -o gira ./cmd/gira
```

## Configure

gira talks to a single Jira Cloud site, authenticating with an email + API
token pair over HTTP Basic auth.

### 1. Store your API token

```sh
gira auth login
```

This prompts for your Atlassian account email and API token. The token is
read from the terminal **without echoing** and stored in the OS keychain
(service `gira`, account = your email) — never in the config file, logs, or
command output.

Do not use `security add-generic-password ... -w` to set the token
yourself: its interactive prompt truncates input at 128 characters, which
silently corrupts Atlassian API tokens (typically ~192 characters).

Check it worked:

```sh
gira auth status
```

### 2. Config file

`~/.config/gira/config.yaml` (created automatically by `auth login` for the
`email` field; edit it for everything else):

```yaml
site: https://your-org.atlassian.net
email: you@example.com
default_project: PROJ
default_board: 1
allowed_projects: [PROJ]   # guardrail: keys/JQL outside this list are refused
read_only: false          # true refuses all write operations
output: table              # table | json
```

Precedence for every setting: **flags > `GIRA_*` environment variables >
config file > built-in defaults.**

| Env var | Overrides |
|---|---|
| `GIRA_SITE` | `site` |
| `GIRA_EMAIL` | `email` |
| `GIRA_API_TOKEN` | keychain token lookup (bypasses the keychain entirely — useful for CI) |
| `GIRA_DEFAULT_PROJECT` | `default_project` |
| `GIRA_DEFAULT_BOARD` | `default_board` |
| `GIRA_ALLOWED_PROJECTS` | `allowed_projects` (comma-separated) |
| `GIRA_READ_ONLY` | `read_only` |
| `GIRA_ALLOW_ANY_SITE` | `allow_any_site` (see Security notes) |
| `GIRA_OUTPUT` | `output` |

## Usage

```sh
gira auth status
gira auth login
gira auth logout

gira issue list   [--status S] [--assignee me|unassigned|<name>] [--type T] \
                  [--epic KEY] [--sprint current|<id>] [--jql Q] [--limit N]
gira issue view   <KEY> [--comments]
gira issue create --type Task --summary "..." [--description -|FILE] [--epic PROJ-3]
                  [--assignee me] [--labels a,b] [--priority High]
gira issue edit   <KEY> [--summary ...] [--description ...] [--labels ...] [--epic ...]
gira issue move   <KEY> "In Progress"       # transition by status name
gira issue assign <KEY> me|unassigned|<name>
gira issue comment <KEY> "text" | -          # stdin supported

gira backlog     [--board 1] [--limit N]
gira sprint list [--board 1]
gira sprint issues <current|next|ID|name> [--limit N]
gira sprint add  <sprintID|current> <KEY...>
gira sprint remove <KEY>
gira epic list   [--board 1]

gira mcp [--read-only] [--all-tools]         # run the MCP server on stdio

gira version
```

Global flags (available on every command): `--json`, `--project`, `--board`,
`--read-only`, `--config <path>`.

Exit codes: `0` ok, `1` usage error, `2` Jira API error, `3` auth error,
`4` guardrail refused (e.g. a project outside `allowed_projects`, or a
write attempted while `read_only: true`).

## MCP server

`gira mcp` runs an MCP (Model Context Protocol) server on stdio, so Claude
Code (or any other MCP client) can drive the same use cases as the CLI,
through typed tools with the same guardrails (project allowlist, read-only
mode, no deletes). Logging goes to stderr only; stdout carries the MCP
protocol exclusively.

### Register with Claude Code

```sh
claude mcp add gira -- /path/to/gira mcp              # full access
claude mcp add gira-ro -- /path/to/gira mcp --read-only # read-only
```

(Registration itself is a `claude mcp` operation the person running Claude
Code performs; gira does not run it for you.)

### Tools

| Tool | Mutates | Notes |
|---|---|---|
| `jira_whoami` | no | identity/token check |
| `jira_search_issues` | no | structured filters or raw JQL; capped at 50 results |
| `jira_get_issue` | no | full detail, optional comments |
| `jira_list_transitions` | no | valid next statuses for an issue |
| `jira_get_backlog` | no | board backlog |
| `jira_list_sprints` | no | board sprints |
| `jira_list_epics` | no | board epics |
| `jira_create_issue` | yes | |
| `jira_update_issue` | yes | summary/description/labels/priority/epic/assignee |
| `jira_transition_issue` | yes | by status or transition name |
| `jira_assign_issue` | yes | `me`, `unassigned`, email, or display name |
| `jira_add_comment` | yes | Markdown, converted to ADF |
| `jira_move_to_sprint` | yes | up to 20 issue keys per call |
| `jira_move_to_backlog` | yes | up to 20 issue keys per call |

Every tool returns a short human-readable text summary plus
`structuredContent` JSON, and reports errors as tool errors (`isError`)
with an actionable message (valid options, allowlist hints, etc.) rather
than failing the whole request.

### Modes

- **`--read-only`** (or config `read_only: true` / `GIRA_READ_ONLY=true`):
  only the seven read-only tools above are registered — write tools are
  never offered, rather than registered and then refused.
- **Default (read-write):** at startup, gira checks the authenticated
  account's live Jira permissions (`GET /rest/api/3/mypermissions`, once
  per `allowed_projects` entry: `CREATE_ISSUES`, `EDIT_ISSUES`,
  `TRANSITION_ISSUES`, `ASSIGN_ISSUES`, `ADD_COMMENTS`,
  `SCHEDULE_ISSUES`) and registers each write tool only if its permission
  is granted. Skipped tools are logged to stderr. If the permission check
  itself fails, gira fails open (registers every write tool, with a
  warning logged) rather than silently hiding capabilities.
- **`--all-tools`:** skips the permission check and registers every write
  tool unconditionally.

`gira auth status` also prints the current account's standing on each of
these six permissions, so you can see ahead of time which write tools an
MCP session will expose.

## Safety

- **Project allowlist:** every issue key and every JQL query (including raw
  `--jql`, which is wrapped as `project in (<allowed>) AND (...)`) is
  checked against `allowed_projects`. Anything else is refused before any
  Jira API call is made.
- **Read-only mode:** `read_only: true` (or `--read-only`, or
  `GIRA_READ_ONLY=true`) makes every write operation refuse up front.
- **No deletes.** gira never exposes a delete operation for issues,
  comments, or sprints.

## Security notes

- **Site validation.** `site` must be `https` and its host must be
  `*.atlassian.net` or `api.atlassian.com`, so your API token can't be sent to
  an arbitrary host via `GIRA_SITE`, or the config file. To override
  (e.g. a local mock), set `allow_any_site: true` or `GIRA_ALLOW_ANY_SITE=true`.
- **`GIRA_API_TOKEN`** is an environment variable and is therefore visible to
  other processes of the same user (e.g. via `ps eww` / `/proc/<pid>/environ`)
  and inherited by child processes. Prefer the OS keychain (`gira auth login`);
  use the variable only for CI, ideally from your CI's secret store.
- Responses from Jira are capped at 10 MiB.
- See [SECURITY.md](./SECURITY.md) for reporting vulnerabilities.
