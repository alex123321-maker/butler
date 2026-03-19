package api

import (
	"net/http"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SystemServer struct {
	pool            *pgxpool.Pool
	tasks           run.TaskReader
	approvals       approvals.Repository
	activeProvider  string
	hasOpenAIKey    bool
	pipelineEnabled bool
}

func NewSystemServer(pool *pgxpool.Pool, tasks run.TaskReader, approvalsRepo approvals.Repository, activeProvider string, hasOpenAIKey bool, pipelineEnabled bool) *SystemServer {
	return &SystemServer{
		pool:            pool,
		tasks:           tasks,
		approvals:       approvalsRepo,
		activeProvider:  activeProvider,
		hasOpenAIKey:    hasOpenAIKey,
		pipelineEnabled: pipelineEnabled,
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
			"pending_approvals":   pendingApprovals,
			"recent_failures":     recentFailures,
			"degraded_components": degraded,
			"partial_errors":      partialErrors,
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
