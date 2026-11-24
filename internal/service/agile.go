package service

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/rafaelescrich/gira/internal/jira"
)

// MaxAgileMoveKeys caps how many issue keys a single MoveToSprint or
// MoveToBacklog call may touch. This is gira's own guardrail (blueprint
// §5/§8's "batch caps (20 keys)"), distinct from and tighter than the
// 50-per-HTTP-call limit internal/jira chunks around.
const MaxAgileMoveKeys = 20

// resolveBoardID returns boardID if given (non-zero), else DefaultBoard,
// erroring out if neither is set.
func (s *Service) resolveBoardID(boardID int) (int, error) {
	if boardID > 0 {
		return boardID, nil
	}
	if s.DefaultBoard > 0 {
		return s.DefaultBoard, nil
	}
	return 0, newUsageError("no board specified and no default_board configured")
}

// Backlog returns up to limit backlog issues for boardID (DefaultBoard
// when boardID is 0), plus the server-reported total issue count.
func (s *Service) Backlog(ctx context.Context, boardID, limit int) ([]jira.Issue, int, error) {
	board, err := s.resolveBoardID(boardID)
	if err != nil {
		return nil, 0, err
	}
	issues, total, err := s.Client.BacklogAll(ctx, board, limit)
	if err != nil {
		return nil, 0, wrapAPIError(err)
	}
	return issues, total, nil
}

// Sprints returns a board's sprints. By default only active and future
// sprints are returned (the common case for planning); includeClosed also
// includes closed sprints.
func (s *Service) Sprints(ctx context.Context, boardID int, includeClosed bool) ([]jira.Sprint, error) {
	board, err := s.resolveBoardID(boardID)
	if err != nil {
		return nil, err
	}
	states := []string{"active", "future"}
	if includeClosed {
		states = append(states, "closed")
	}
	sprints, _, err := s.Client.SprintsAll(ctx, board, states, jira.MaxAgileResults)
	if err != nil {
		return nil, wrapAPIError(err)
	}
	return sprints, nil
}

// ResolveSprint resolves a sprint reference to a concrete sprint:
//   - "current": the sole active sprint (a usage error if there is none,
//     or if there is more than one, since callers must disambiguate).
//   - "next": the future sprint with the earliest start date.
//   - a numeric sprint ID: looked up directly (any state).
//   - anything else: matched case-insensitively by name among
//     active/future sprints.
func (s *Service) ResolveSprint(ctx context.Context, boardID int, ref string) (jira.Sprint, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return jira.Sprint{}, newUsageError("sprint is required (\"current\", \"next\", an ID, or a name)")
	}

	board, err := s.resolveBoardID(boardID)
	if err != nil {
		return jira.Sprint{}, err
	}

	if id, convErr := strconv.Atoi(ref); convErr == nil {
		sprints, _, sErr := s.Client.SprintsAll(ctx, board, []string{"active", "future", "closed"}, jira.MaxAgileResults)
		if sErr != nil {
			return jira.Sprint{}, wrapAPIError(sErr)
		}
		for _, sp := range sprints {
			if sp.ID == id {
				return sp, nil
			}
		}
		return jira.Sprint{}, newUsageError("sprint %d not found on board %d", id, board)
	}

	sprints, _, err := s.Client.SprintsAll(ctx, board, []string{"active", "future"}, jira.MaxAgileResults)
	if err != nil {
		return jira.Sprint{}, wrapAPIError(err)
	}

	switch strings.ToLower(ref) {
	case "current":
		active := filterSprintsByState(sprints, "active")
		if len(active) == 0 {
			return jira.Sprint{}, newUsageError("no active sprint on board %d", board)
		}
		if len(active) > 1 {
			return jira.Sprint{}, newUsageError("multiple active sprints on board %d; specify a sprint ID or name", board)
		}
		return active[0], nil
	case "next":
		future := filterSprintsByState(sprints, "future")
		if len(future) == 0 {
			return jira.Sprint{}, newUsageError("no future sprint on board %d", board)
		}
		sort.Slice(future, func(i, j int) bool { return future[i].StartDate < future[j].StartDate })
		return future[0], nil
	default:
		var matches []jira.Sprint
		for _, sp := range sprints {
			if strings.EqualFold(sp.Name, ref) {
				matches = append(matches, sp)
			}
		}
		switch len(matches) {
		case 0:
			return jira.Sprint{}, newUsageError("sprint %q not found among active/future sprints on board %d", ref, board)
		case 1:
			return matches[0], nil
		default:
			return jira.Sprint{}, newUsageError("multiple sprints named %q on board %d; specify a sprint ID", ref, board)
		}
	}
}

func filterSprintsByState(sprints []jira.Sprint, state string) []jira.Sprint {
	var out []jira.Sprint
	for _, sp := range sprints {
		if strings.EqualFold(sp.State, state) {
			out = append(out, sp)
		}
	}
	return out
}

// SprintIssues resolves sprintRef (see ResolveSprint) and returns up to
// limit issues in that sprint, plus the resolved sprint itself.
func (s *Service) SprintIssues(ctx context.Context, boardID int, sprintRef string, limit int) ([]jira.Issue, jira.Sprint, error) {
	sprint, err := s.ResolveSprint(ctx, boardID, sprintRef)
	if err != nil {
		return nil, jira.Sprint{}, err
	}
	issues, _, err := s.Client.SprintIssuesAll(ctx, sprint.ID, limit)
	if err != nil {
		return nil, sprint, wrapAPIError(err)
	}
	return issues, sprint, nil
}

// MoveToSprint resolves sprintRef and moves keys into it. It refuses in
// read-only mode and validates every key against AllowedProjects and the
// batch cap before making any Jira API call.
func (s *Service) MoveToSprint(ctx context.Context, boardID int, sprintRef string, keys []string) (jira.Sprint, error) {
	if err := s.requireWritable(); err != nil {
		return jira.Sprint{}, err
	}
	normalized, err := s.normalizeAgileKeys(keys)
	if err != nil {
		return jira.Sprint{}, err
	}

	sprint, err := s.ResolveSprint(ctx, boardID, sprintRef)
	if err != nil {
		return jira.Sprint{}, err
	}

	if err := s.Client.MoveToSprint(ctx, sprint.ID, normalized); err != nil {
		return sprint, wrapAPIError(err)
	}
	return sprint, nil
}

// MoveToBacklog moves keys out of whatever sprint they're in and back to
// the backlog. It refuses in read-only mode and validates every key
// against AllowedProjects and the batch cap before making any Jira API
// call.
func (s *Service) MoveToBacklog(ctx context.Context, keys []string) error {
	if err := s.requireWritable(); err != nil {
		return err
	}
	normalized, err := s.normalizeAgileKeys(keys)
	if err != nil {
		return err
	}
	if err := s.Client.MoveToBacklog(ctx, normalized); err != nil {
		return wrapAPIError(err)
	}
	return nil
}

// normalizeAgileKeys upper-cases, trims, and de-duplicates keys, checking
// each one against AllowedProjects and enforcing MaxAgileMoveKeys. It
// returns a usage error if the resulting set is empty or too large. No
// network call is made here, so callers can rely on it running entirely
// before any Jira API request.
func (s *Service) normalizeAgileKeys(keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, newUsageError("at least one issue key is required")
	}
	if len(keys) > MaxAgileMoveKeys {
		return nil, newUsageError("too many issue keys (%d); max %d per call", len(keys), MaxAgileMoveKeys)
	}

	out := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, k := range keys {
		k = strings.ToUpper(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		if err := s.checkKeyAllowed(k); err != nil {
			return nil, err
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	if len(out) == 0 {
		return nil, newUsageError("at least one issue key is required")
	}
	return out, nil
}

// Epics returns the epics for boardID (DefaultBoard when 0). It tries the
// Agile API's board epic endpoint first; on team-managed ("simple")
// boards that endpoint can error out, in which case Epics falls back to a JQL search for
// `issuetype = Epic` scoped to the resolved default project.
func (s *Service) Epics(ctx context.Context, boardID int) ([]jira.Epic, error) {
	board, err := s.resolveBoardID(boardID)
	if err != nil {
		return nil, err
	}

	epics, _, err := s.Client.BoardEpicsAll(ctx, board, jira.MaxAgileResults)
	if err != nil {
		if !isEpicEndpointUnsupported(err) {
			return nil, wrapAPIError(err)
		}
		return s.epicsViaJQL(ctx)
	}
	return epics, nil
}

// isEpicEndpointUnsupported reports whether err looks like the board epic
// endpoint being unavailable (as opposed to, say, an auth failure), which
// should trigger the JQL fallback rather than being surfaced directly.
func isEpicEndpointUnsupported(err error) bool {
	var apiErr *jira.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusBadRequest || apiErr.StatusCode == http.StatusNotFound
	}
	return false
}

func (s *Service) epicsViaJQL(ctx context.Context) ([]jira.Epic, error) {
	page, err := s.SearchIssues(ctx, SearchParams{Type: "Epic", Limit: jira.MaxSearchResults})
	if err != nil {
		return nil, err
	}
	epics := make([]jira.Epic, 0, len(page.Issues))
	for _, iss := range page.Issues {
		id, _ := strconv.Atoi(iss.ID)
		epics = append(epics, jira.Epic{
			ID:      id,
			Key:     iss.Key,
			Name:    iss.Fields.Summary,
			Summary: iss.Fields.Summary,
			Done:    strings.EqualFold(iss.Fields.Status.Category.Key, "done"),
		})
	}
	return epics, nil
}
