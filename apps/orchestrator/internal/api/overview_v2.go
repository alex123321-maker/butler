package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	deliveryevents "github.com/butler/butler/apps/orchestrator/internal/deliveryevents"
	"github.com/butler/butler/apps/orchestrator/internal/run"
)

// OverviewServer serves aggregated overview data for task-centric UI.
type OverviewServer struct {
	tasks    run.TaskReader
	delivery overviewDeliveryReader
}

type overviewDeliveryReader interface {
	LatestByRun(ctx context.Context, runID string) (*deliveryevents.Record, error)
}

// NewOverviewServer creates an overview aggregation server.
func NewOverviewServer(tasks run.TaskReader, delivery overviewDeliveryReader) *OverviewServer {
	return &OverviewServer{tasks: tasks, delivery: delivery}
}

// HandleGetOverview handles GET /api/v2/overview and GET /api/overview.
func (s *OverviewServer) HandleGetOverview() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.tasks == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "overview reader is not configured"})
			return
		}

		partialErrors := make([]map[string]string, 0)
		degradedComponents := make([]string, 0)

		activeTasks, err := s.tasks.ListTasks(r.Context(), run.TaskListParams{
			Status: "in_progress",
			Sort:   run.TaskSortUpdatedAtDesc,
			Limit:  12,
		})
		if err != nil {
			activeTasks = []run.TaskRow{}
			partialErrors = append(partialErrors, map[string]string{"source": "active_tasks", "error": "failed to load active tasks"})
			degradedComponents = append(degradedComponents, "active_tasks")
		}

		attentionItems := make([]overviewTaskItemDTO, 0)
		for _, status := range []string{"waiting_for_approval", "waiting_for_reply_in_telegram", "failed"} {
			items, listErr := s.tasks.ListTasks(r.Context(), run.TaskListParams{
				Status: status,
				Sort:   run.TaskSortUpdatedAtDesc,
				Limit:  6,
			})
			if listErr != nil {
				partialErrors = append(partialErrors, map[string]string{"source": "attention_items", "error": "failed to load attention items"})
				degradedComponents = appendIfMissing(degradedComponents, "attention_items")
				continue
			}
			for _, item := range items {
				dto := toOverviewTaskItemDTO(item)
				if s.delivery != nil {
					if latest, err := s.delivery.LatestByRun(r.Context(), item.RunID); err == nil && latest != nil && latest.Channel == deliveryevents.ChannelTelegram && latest.State == deliveryevents.StateWaiting {
						dto.Status = "waiting_for_reply_in_telegram"
						dto.WaitingReason = "approval_required"
						dto.UserActionChannel = "telegram"
						dto.NeedsUserAction = true
					}
				}
				attentionItems = append(attentionItems, dto)
			}
		}
		attentionItems = uniqueOverviewItems(attentionItems)

		recentResults, err := s.loadRecentResults(r)
		if err != nil {
			recentResults = []overviewTaskItemDTO{}
			partialErrors = append(partialErrors, map[string]string{"source": "recent_results", "error": "failed to load recent results"})
			degradedComponents = append(degradedComponents, "recent_results")
		}

		pendingApprovals := countStatus(attentionItems, "waiting_for_approval") + countStatus(attentionItems, "waiting_for_reply_in_telegram")
		telegramWaits := countStatus(attentionItems, "waiting_for_reply_in_telegram")
		failedTasks := countStatus(attentionItems, "failed") + countStatus(recentResults, "failed") + countStatus(recentResults, "completed_with_issues")

		systemState := "healthy"
		if len(degradedComponents) > 0 || failedTasks > 0 {
			systemState = "degraded"
		}

		response := map[string]any{
			"attention_items": attentionItems,
			"active_tasks":    toOverviewItems(activeTasks),
			"recent_results":  recentResults,
			"system_summary": map[string]any{
				"state":               systemState,
				"pending_approvals":   pendingApprovals,
				"recent_failures":     failedTasks,
				"degraded_components": degradedComponents,
			},
			"counts": map[string]int{
				"attention_items":                        len(attentionItems),
				"active_tasks":                           len(activeTasks),
				"recent_results":                         len(recentResults),
				"approvals_pending_count":                pendingApprovals,
				"tasks_waiting_for_telegram_reply_count": telegramWaits,
				"failed_tasks_count":                     failedTasks,
				"degraded_services_count":                len(degradedComponents),
			},
			"partial_errors": partialErrors,
		}

		writeJSON(w, http.StatusOK, response)
	})
}

func (s *OverviewServer) loadRecentResults(r *http.Request) ([]overviewTaskItemDTO, error) {
	all := make([]overviewTaskItemDTO, 0)
	for _, status := range []string{"completed", "completed_with_issues", "failed", "cancelled"} {
		items, err := s.tasks.ListTasks(r.Context(), run.TaskListParams{
			Status: status,
			Sort:   run.TaskSortUpdatedAtDesc,
			Limit:  8,
		})
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			all = append(all, toOverviewTaskItemDTO(item))
		}
	}
	all = uniqueOverviewItems(all)
	sort.SliceStable(all, func(i, j int) bool {
		left, _ := time.Parse(time.RFC3339, all[i].UpdatedAt)
		right, _ := time.Parse(time.RFC3339, all[j].UpdatedAt)
		return left.After(right)
	})
	if len(all) > 12 {
		all = all[:12]
	}
	return all, nil
}

type overviewTaskItemDTO struct {
	TaskID            string `json:"task_id"`
	RunID             string `json:"run_id"`
	SessionKey        string `json:"session_key"`
	SourceChannel     string `json:"source_channel"`
	Status            string `json:"status"`
	RunState          string `json:"run_state"`
	NeedsUserAction   bool   `json:"needs_user_action"`
	UserActionChannel string `json:"user_action_channel"`
	WaitingReason     string `json:"waiting_reason"`
	UpdatedAt         string `json:"updated_at"`
	OutcomeSummary    string `json:"outcome_summary"`
	ErrorSummary      string `json:"error_summary"`
	RiskLevel         string `json:"risk_level"`
	Provider          string `json:"provider"`
}

func toOverviewItems(rows []run.TaskRow) []overviewTaskItemDTO {
	items := make([]overviewTaskItemDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, toOverviewTaskItemDTO(row))
	}
	return uniqueOverviewItems(items)
}

func toOverviewTaskItemDTO(item run.TaskRow) overviewTaskItemDTO {
	status, needsUserAction, actionChannel, waitingReason := mapTaskPresentation(item.RunState, item.SessionChannel)
	outcomeSummary := strings.TrimSpace(item.OutcomeSummary)
	if len(outcomeSummary) > 220 {
		outcomeSummary = outcomeSummary[:220]
	}
	errorSummary := strings.TrimSpace(item.ErrorSummary)
	if len(errorSummary) > 220 {
		errorSummary = errorSummary[:220]
	}
	return overviewTaskItemDTO{
		TaskID:            item.TaskID,
		RunID:             item.RunID,
		SessionKey:        item.SessionKey,
		SourceChannel:     item.SourceChannel,
		Status:            status,
		RunState:          item.RunState,
		NeedsUserAction:   needsUserAction,
		UserActionChannel: actionChannel,
		WaitingReason:     waitingReason,
		UpdatedAt:         item.UpdatedAt.UTC().Format(time.RFC3339),
		OutcomeSummary:    outcomeSummary,
		ErrorSummary:      errorSummary,
		RiskLevel:         item.RiskLevel,
		Provider:          item.ModelProvider,
	}
}

func uniqueOverviewItems(items []overviewTaskItemDTO) []overviewTaskItemDTO {
	seen := make(map[string]struct{}, len(items))
	result := make([]overviewTaskItemDTO, 0, len(items))
	for _, item := range items {
		if _, exists := seen[item.TaskID]; exists {
			continue
		}
		seen[item.TaskID] = struct{}{}
		result = append(result, item)
	}
	return result
}

func appendIfMissing(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	return append(items, value)
}

func countStatus(items []overviewTaskItemDTO, status string) int {
	count := 0
	for _, item := range items {
		if item.Status == status {
			count++
		}
	}
	return count
}
