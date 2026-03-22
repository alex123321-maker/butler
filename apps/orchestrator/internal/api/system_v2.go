package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultExtensionInstancesLimit      = 50
	maxExtensionInstancesLimit          = 200
	defaultExtensionHeartbeatTTLSeconds = 90
)

type SystemServer struct {
	pool                        *pgxpool.Pool
	tasks                       run.TaskReader
	approvals                   approvals.Repository
	activeProvider              string
	hasOpenAIKey                bool
	pipelineEnabled             bool
	singleTabTransportMode      string
	extensionAuthConfigured     bool
	singleTabRelayHeartbeatTTLS int
}

func NewSystemServer(
	pool *pgxpool.Pool,
	tasks run.TaskReader,
	approvalsRepo approvals.Repository,
	activeProvider string,
	hasOpenAIKey bool,
	pipelineEnabled bool,
	singleTabTransportMode string,
	extensionAuthConfigured bool,
	singleTabRelayHeartbeatTTLS int,
) *SystemServer {
	return &SystemServer{
		pool:                        pool,
		tasks:                       tasks,
		approvals:                   approvalsRepo,
		activeProvider:              activeProvider,
		hasOpenAIKey:                hasOpenAIKey,
		pipelineEnabled:             pipelineEnabled,
		singleTabTransportMode:      singleTabTransportMode,
		extensionAuthConfigured:     extensionAuthConfigured,
		singleTabRelayHeartbeatTTLS: singleTabRelayHeartbeatTTLS,
	}
}

func (s *SystemServer) HandleGetSystem() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		degraded := make([]string, 0)
		partialErrors := make([]map[string]string, 0)

		healthStatus := "healthy"
		recentFailures := make([]map[string]any, 0)
		if s.tasks != nil {
			failed, err := s.tasks.ListTasks(r.Context(), run.TaskListParams{Status: "failed", Sort: run.TaskSortUpdatedAtDesc, Limit: 5})
			if err != nil {
				partialErrors = append(partialErrors, map[string]string{"source": "recent_failures", "error": "failed to load failed tasks"})
				degraded = appendIfMissing(degraded, "tasks")
			} else {
				for _, item := range failed {
					recentFailures = append(recentFailures, map[string]any{
						"run_id":     item.RunID,
						"status":     "failed",
						"error":      item.ErrorSummary,
						"updated_at": item.UpdatedAt.UTC().Format(time.RFC3339),
					})
				}
			}
		}

		pendingApprovals := 0
		if s.approvals != nil {
			items, err := s.approvals.ListApprovals(r.Context(), approvals.StatusPending, "", "", 200, 0)
			if err != nil {
				partialErrors = append(partialErrors, map[string]string{"source": "pending_approvals", "error": "failed to load pending approvals"})
				degraded = appendIfMissing(degraded, "approvals")
			} else {
				pendingApprovals = len(items)
			}
		}

		doctor := map[string]any{"status": "unknown", "checked_at": "", "stale": true}
		activeSingleTabSessions := 0
		hostDisconnectedSessions := 0
		extensionInstances := make([]map[string]any, 0)
		if s.pool != nil {
			var status string
			var checkedAt time.Time
			err := s.pool.QueryRow(r.Context(), `SELECT status, checked_at FROM doctor_reports ORDER BY checked_at DESC LIMIT 1`).Scan(&status, &checkedAt)
			if err == nil {
				stale := time.Since(checkedAt.UTC()) > 2*time.Hour
				doctor = map[string]any{"status": status, "checked_at": checkedAt.UTC().Format(time.RFC3339), "stale": stale}
				if status != "healthy" || stale {
					degraded = appendIfMissing(degraded, "doctor")
				}
			} else {
				partialErrors = append(partialErrors, map[string]string{"source": "doctor", "error": "failed to load doctor report"})
				degraded = appendIfMissing(degraded, "doctor")
			}

			err = s.pool.QueryRow(r.Context(), `
				SELECT
					COUNT(*) FILTER (WHERE status = 'ACTIVE'),
					COUNT(*) FILTER (WHERE status = 'HOST_DISCONNECTED')
				FROM single_tab_sessions
			`).Scan(&activeSingleTabSessions, &hostDisconnectedSessions)
			if err != nil {
				partialErrors = append(partialErrors, map[string]string{"source": "single_tab_sessions", "error": "failed to load single-tab session stats"})
				degraded = appendIfMissing(degraded, "single_tab")
			}

			instances, instanceErrors := s.loadExtensionInstances(r.Context(), defaultExtensionInstancesLimit)
			if len(instanceErrors) > 0 {
				partialErrors = append(partialErrors, instanceErrors...)
				degraded = appendIfMissing(degraded, "single_tab")
			}
			for _, instance := range instances {
				extensionInstances = append(extensionInstances, toExtensionInstanceDTO(instance))
			}
		}

		if hostDisconnectedSessions > 0 {
			degraded = appendIfMissing(degraded, "single_tab")
		}
		if len(degraded) > 0 || pendingApprovals > 0 || len(recentFailures) > 0 {
			healthStatus = "degraded"
		}

		response := map[string]any{
			"health": map[string]any{
				"status":              healthStatus,
				"degraded_components": degraded,
			},
			"doctor": doctor,
			"providers": []map[string]any{{
				"name":       s.activeProvider,
				"active":     true,
				"configured": s.hasOpenAIKey || s.activeProvider != "openai",
			}},
			"queues": map[string]any{
				"memory_pipeline": map[string]any{
					"enabled": s.pipelineEnabled,
					"status":  ternary(s.pipelineEnabled, "running", "disabled"),
				},
			},
			"single_tab_extension": map[string]any{
				"transport_mode":              firstNonEmptySystemString(s.singleTabTransportMode, "dual"),
				"relay_enabled":               firstNonEmptySystemString(s.singleTabTransportMode, "dual") != "native_only",
				"extension_auth_configured":   s.extensionAuthConfigured,
				"relay_heartbeat_ttl_seconds": s.singleTabRelayHeartbeatTTLS,
				"active_sessions":             activeSingleTabSessions,
				"host_disconnected_sessions":  hostDisconnectedSessions,
				"instances":                   extensionInstances,
			},
			"pending_approvals":   pendingApprovals,
			"recent_failures":     recentFailures,
			"degraded_components": degraded,
			"partial_errors":      partialErrors,
		}

		writeJSON(w, http.StatusOK, response)
	})
}

func (s *SystemServer) HandleListExtensionInstances() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		limit := normalizeExtensionInstanceLimit(r.URL.Query().Get("limit"))
		stateFilter, stateFilterList, err := parseExtensionInstanceStateFilter(r.URL.Query().Get("state"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		instances, partialErrors := s.loadExtensionInstances(r.Context(), maxExtensionInstancesLimit)
		filtered, matched, truncated := filterExtensionInstances(instances, stateFilter, limit)
		counters := countExtensionInstanceStates(filtered)
		activeSessions, hostDisconnectedSessions := sumExtensionSessionCounters(filtered)

		response := map[string]any{
			"items": toExtensionInstanceDTOList(filtered),
			"summary": map[string]any{
				"instances_total":            len(instances),
				"instances_matched":          matched,
				"online":                     counters["online"],
				"stale":                      counters["stale"],
				"disconnected":               counters["disconnected"],
				"unknown":                    counters["unknown"],
				"active_sessions":            activeSessions,
				"host_disconnected_sessions": hostDisconnectedSessions,
				"truncated":                  truncated,
				"captured_at":                time.Now().UTC().Format(time.RFC3339),
			},
			"meta": map[string]any{
				"limit":                       limit,
				"state_filter":                stateFilterList,
				"transport_mode":              firstNonEmptySystemString(s.singleTabTransportMode, "dual"),
				"relay_enabled":               firstNonEmptySystemString(s.singleTabTransportMode, "dual") != "native_only",
				"relay_heartbeat_ttl_seconds": s.extensionHeartbeatTTLSeconds(),
			},
			"liveness_policy": map[string]any{
				"heartbeat_ttl_seconds":                        s.extensionHeartbeatTTLSeconds(),
				"online_when_last_seen_within_ttl":             true,
				"stale_when_last_seen_exceeds_ttl":             true,
				"disconnected_when_only_disconnected_sessions": true,
				"unknown_when_last_seen_missing":               true,
			},
			"partial_errors": partialErrors,
		}

		writeJSON(w, http.StatusOK, response)
	})
}

func ternary[T any](cond bool, t, f T) T {
	if cond {
		return t
	}
	return f
}

func firstNonEmptySystemString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func formatOptionalRFC3339(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func classifyExtensionInstanceState(lastSeenAt *time.Time, activeSessions, disconnectedSessions, heartbeatTTLSeconds int) string {
	if lastSeenAt == nil {
		if disconnectedSessions > 0 {
			return "disconnected"
		}
		return "unknown"
	}
	if heartbeatTTLSeconds <= 0 {
		heartbeatTTLSeconds = 90
	}
	if activeSessions <= 0 && disconnectedSessions > 0 {
		return "disconnected"
	}
	age := time.Since(lastSeenAt.UTC())
	if age <= time.Duration(heartbeatTTLSeconds)*time.Second {
		return "online"
	}
	if disconnectedSessions > 0 {
		return "disconnected"
	}
	return "stale"
}

type extensionInstanceStatus struct {
	BrowserInstanceID        string
	LastSeenAt               *time.Time
	ActiveSessions           int
	HostDisconnectedSessions int
	State                    string
}

func (s *SystemServer) loadExtensionInstances(ctx context.Context, limit int) ([]extensionInstanceStatus, []map[string]string) {
	instances := make([]extensionInstanceStatus, 0)
	partialErrors := make([]map[string]string, 0)
	if s.pool == nil {
		return instances, partialErrors
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
			browser_instance_id,
			MAX(last_seen_at) AS last_seen_at,
			COUNT(*) FILTER (WHERE status = 'ACTIVE') AS active_sessions,
			COUNT(*) FILTER (WHERE status = 'HOST_DISCONNECTED') AS host_disconnected_sessions
		FROM single_tab_sessions
		WHERE COALESCE(browser_instance_id, '') <> ''
		GROUP BY browser_instance_id
		ORDER BY MAX(last_seen_at) DESC NULLS LAST
		LIMIT $1
	`, clampExtensionInstanceLimit(limit))
	if err != nil {
		return instances, append(partialErrors, map[string]string{"source": "single_tab_extension_instances", "error": "failed to load extension instance status"})
	}
	defer rows.Close()

	decodeErrAdded := false
	for rows.Next() {
		var browserInstanceID string
		var lastSeenAt *time.Time
		var activeSessions int
		var disconnectedSessions int
		if scanErr := rows.Scan(&browserInstanceID, &lastSeenAt, &activeSessions, &disconnectedSessions); scanErr != nil {
			if !decodeErrAdded {
				partialErrors = append(partialErrors, map[string]string{"source": "single_tab_extension_instances", "error": "failed to decode extension instance status"})
				decodeErrAdded = true
			}
			continue
		}
		instances = append(instances, extensionInstanceStatus{
			BrowserInstanceID:        browserInstanceID,
			LastSeenAt:               lastSeenAt,
			ActiveSessions:           activeSessions,
			HostDisconnectedSessions: disconnectedSessions,
			State:                    classifyExtensionInstanceState(lastSeenAt, activeSessions, disconnectedSessions, s.extensionHeartbeatTTLSeconds()),
		})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		partialErrors = append(partialErrors, map[string]string{"source": "single_tab_extension_instances", "error": "failed to iterate extension instance status"})
	}
	return instances, partialErrors
}

func toExtensionInstanceDTO(instance extensionInstanceStatus) map[string]any {
	return map[string]any{
		"browser_instance_id":        instance.BrowserInstanceID,
		"last_seen_at":               formatOptionalRFC3339(instance.LastSeenAt),
		"active_sessions":            instance.ActiveSessions,
		"host_disconnected_sessions": instance.HostDisconnectedSessions,
		"state":                      instance.State,
	}
}

func toExtensionInstanceDTOList(items []extensionInstanceStatus) []map[string]any {
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, toExtensionInstanceDTO(item))
	}
	return result
}

func normalizeExtensionInstanceLimit(raw string) int {
	if strings.TrimSpace(raw) == "" {
		return defaultExtensionInstancesLimit
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return defaultExtensionInstancesLimit
	}
	if value < 1 {
		return 1
	}
	if value > maxExtensionInstancesLimit {
		return maxExtensionInstancesLimit
	}
	return value
}

func clampExtensionInstanceLimit(value int) int {
	if value < 1 {
		return 1
	}
	if value > maxExtensionInstancesLimit {
		return maxExtensionInstancesLimit
	}
	return value
}

func parseExtensionInstanceStateFilter(raw string) (map[string]struct{}, []string, error) {
	filter := make(map[string]struct{})
	list := make([]string, 0)
	normalizedRaw := strings.TrimSpace(raw)
	if normalizedRaw == "" {
		return filter, list, nil
	}
	for _, part := range strings.Split(normalizedRaw, ",") {
		state := strings.ToLower(strings.TrimSpace(part))
		if state == "" {
			continue
		}
		if !isAllowedExtensionState(state) {
			return nil, nil, &apiError{message: "state filter supports only: online, stale, disconnected, unknown"}
		}
		if _, exists := filter[state]; exists {
			continue
		}
		filter[state] = struct{}{}
		list = append(list, state)
	}
	return filter, list, nil
}

type apiError struct {
	message string
}

func (e *apiError) Error() string {
	if e == nil {
		return ""
	}
	return e.message
}

func isAllowedExtensionState(state string) bool {
	switch state {
	case "online", "stale", "disconnected", "unknown":
		return true
	default:
		return false
	}
}

func filterExtensionInstances(items []extensionInstanceStatus, stateFilter map[string]struct{}, limit int) ([]extensionInstanceStatus, int, bool) {
	result := make([]extensionInstanceStatus, 0, len(items))
	matched := 0
	truncated := false
	normalizedLimit := clampExtensionInstanceLimit(limit)
	for _, item := range items {
		if len(stateFilter) > 0 {
			if _, ok := stateFilter[item.State]; !ok {
				continue
			}
		}
		matched++
		if len(result) >= normalizedLimit {
			truncated = true
			continue
		}
		result = append(result, item)
	}
	return result, matched, truncated
}

func countExtensionInstanceStates(items []extensionInstanceStatus) map[string]int {
	counters := map[string]int{
		"online":       0,
		"stale":        0,
		"disconnected": 0,
		"unknown":      0,
	}
	for _, item := range items {
		if _, ok := counters[item.State]; ok {
			counters[item.State]++
			continue
		}
		counters["unknown"]++
	}
	return counters
}

func sumExtensionSessionCounters(items []extensionInstanceStatus) (int, int) {
	active := 0
	disconnected := 0
	for _, item := range items {
		active += item.ActiveSessions
		disconnected += item.HostDisconnectedSessions
	}
	return active, disconnected
}

func (s *SystemServer) extensionHeartbeatTTLSeconds() int {
	if s == nil || s.singleTabRelayHeartbeatTTLS <= 0 {
		return defaultExtensionHeartbeatTTLSeconds
	}
	return s.singleTabRelayHeartbeatTTLS
}
