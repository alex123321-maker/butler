package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	artifacts "github.com/butler/butler/apps/orchestrator/internal/artifacts"
	deliveryevents "github.com/butler/butler/apps/orchestrator/internal/deliveryevents"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/memory/transcript"
)

// TaskViewServer serves task-centric read endpoints for Web UI.
type TaskViewServer struct {
	tasks       run.TaskReader
	runs        RunLister
	sessions    SessionLister
	transcripts TranscriptReader
	transitions TransitionLister
	artifacts   taskArtifactsReader
	delivery    taskDeliveryReader
}

type taskArtifactsReader interface {
	ListArtifactsByRun(ctx context.Context, runID string, limit int) ([]artifacts.Record, error)
}

type taskDeliveryReader interface {
	LatestByRun(ctx context.Context, runID string) (*deliveryevents.Record, error)
}

// NewTaskViewServer creates a task-centric view server.
func NewTaskViewServer(tasks run.TaskReader, runs RunLister, sessions SessionLister, transcripts TranscriptReader, transitions TransitionLister, taskArtifacts taskArtifactsReader, taskDelivery taskDeliveryReader) *TaskViewServer {
	return &TaskViewServer{tasks: tasks, runs: runs, sessions: sessions, transcripts: transcripts, transitions: transitions, artifacts: taskArtifacts, delivery: taskDelivery}
}

// HandleListTasks handles GET /api/v2/tasks and GET /api/tasks.
func (s *TaskViewServer) HandleListTasks() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.tasks == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "task reader is not configured"})
			return
		}

		params, err := taskListParamsFromRequest(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		items, err := s.tasks.ListTasks(r.Context(), params)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list tasks"})
			return
		}
		if items == nil {
			items = []run.TaskRow{}
		}

		payload := make([]taskListItemDTO, 0, len(items))
		for _, item := range items {
			payload = append(payload, toTaskListItemDTO(item))
		}

		writeJSON(w, http.StatusOK, map[string]any{"tasks": payload})
	})
}

// HandleGetTaskDetail handles GET /api/v2/tasks/{id} and GET /api/tasks/{id}.
func (s *TaskViewServer) HandleGetTaskDetail(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.runs == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "task detail dependencies are not configured"})
			return
		}

		taskID := extractPathParam(r.URL.Path, prefix)
		if taskID == "" || strings.Contains(taskID, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task id is required"})
			return
		}

		runRecord, err := s.runs.GetRun(r.Context(), taskID)
		if err != nil {
			if err == run.ErrRunNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load task"})
			return
		}
		if s.transcripts == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "task detail dependencies are not configured"})
			return
		}

		tx, err := s.transcripts.GetRunTranscript(r.Context(), runRecord.RunID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load task transcript"})
			return
		}

		sourceChannel := ""
		if s.sessions != nil {
			sessionRecord, sessionErr := s.sessions.GetSessionByKey(r.Context(), runRecord.SessionKey)
			if sessionErr == nil {
				sourceChannel = sessionRecord.Channel
			} else if sessionErr != session.ErrSessionNotFound {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load task session"})
				return
			}
		}
		if sourceChannel == "" {
			sourceChannel = inferSourceChannel(runRecord.SessionKey)
		}

		status, needsUserAction, actionChannel, waitingReason := mapTaskPresentation(runRecord.CurrentState, sourceChannel)
		status, waitingReason, actionChannel = applyDeliveryWaitingOverride(status, waitingReason, actionChannel, s.loadDeliveryState(r, runRecord.RunID))
		riskLevel := deriveRiskLevel(runRecord.CurrentState)
		sourcePreview, sourceFull := extractSourceMessage(tx.Messages)
		resultSummary := extractAssistantFinal(tx.Messages)
		timelinePreview := s.loadTimelinePreview(r, runRecord.RunID)
		deliveryState := s.loadDeliveryState(r, runRecord.RunID)

		response := map[string]any{
			"task": map[string]any{
				"task_id":             runRecord.RunID,
				"run_id":              runRecord.RunID,
				"session_key":         runRecord.SessionKey,
				"status":              status,
				"run_state":           runRecord.CurrentState,
				"current_stage":       runRecord.CurrentState,
				"needs_user_action":   needsUserAction,
				"user_action_channel": actionChannel,
				"waiting_reason":      waitingReason,
				"started_at":          runRecord.StartedAt.UTC().Format(time.RFC3339),
				"updated_at":          runRecord.UpdatedAt.UTC().Format(time.RFC3339),
				"finished_at":         formatOptionalTime(runRecord.FinishedAt),
				"outcome_summary":     resultSummary,
				"error_summary":       runRecord.ErrorMessage,
				"risk_level":          riskLevel,
				"source_channel":      sourceChannel,
				"model_provider":      runRecord.ModelProvider,
				"autonomy_mode":       runRecord.AutonomyMode,
			},
			"summary_bar": map[string]any{
				"status":         status,
				"risk_level":     riskLevel,
				"source_channel": sourceChannel,
				"started_at":     runRecord.StartedAt.UTC().Format(time.RFC3339),
				"updated_at":     runRecord.UpdatedAt.UTC().Format(time.RFC3339),
				"finished_at":    formatOptionalTime(runRecord.FinishedAt),
			},
			"source": map[string]any{
				"channel":                sourceChannel,
				"session_key":            runRecord.SessionKey,
				"source_message_preview": sourcePreview,
				"source_message_full":    sourceFull,
			},
			"waiting_state": map[string]any{
				"needs_user_action":       needsUserAction,
				"user_action_channel":     actionChannel,
				"waiting_reason":          waitingReason,
				"telegram_delivery_state": deliveryState,
				"note":                    waitingStateNote(status, sourceChannel),
			},
			"result": map[string]any{
				"outcome_summary": resultSummary,
				"has_result":      strings.TrimSpace(resultSummary) != "",
			},
			"error": map[string]any{
				"error_type":    runRecord.ErrorType,
				"error_summary": runRecord.ErrorMessage,
				"has_error":     strings.TrimSpace(runRecord.ErrorMessage) != "",
			},
			"timeline_preview": timelinePreview,
			"artifacts":        s.loadTaskArtifacts(r, runRecord.RunID),
			"delivery":         deliveryState,
			"debug_refs": map[string]any{
				"run":         fmt.Sprintf("/api/v1/runs/%s", runRecord.RunID),
				"transcript":  fmt.Sprintf("/api/v1/runs/%s/transcript", runRecord.RunID),
				"transitions": fmt.Sprintf("/api/v1/runs/%s/transitions", runRecord.RunID),
				"artifacts":   fmt.Sprintf("/api/v2/tasks/%s/artifacts", runRecord.RunID),
			},
		}

		writeJSON(w, http.StatusOK, response)
	})
}

func (s *TaskViewServer) loadDeliveryState(r *http.Request, runID string) map[string]any {
	if s.delivery == nil {
		return map[string]any{"channel": "", "state": "", "delivery_type": "", "error_message": ""}
	}
	event, err := s.delivery.LatestByRun(r.Context(), runID)
	if err != nil || event == nil {
		return map[string]any{"channel": "", "state": "", "delivery_type": "", "error_message": ""}
	}
	return map[string]any{
		"channel":       event.Channel,
		"state":         event.State,
		"delivery_type": event.DeliveryType,
		"error_message": event.ErrorMessage,
		"created_at":    event.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func applyDeliveryWaitingOverride(status, waitingReason, actionChannel string, delivery map[string]any) (string, string, string) {
	channel, _ := delivery["channel"].(string)
	state, _ := delivery["state"].(string)
	if strings.EqualFold(channel, "telegram") && state == deliveryevents.StateWaiting {
		return "waiting_for_reply_in_telegram", "approval_required", "telegram"
	}
	return status, waitingReason, actionChannel
}

func (s *TaskViewServer) loadTaskArtifacts(r *http.Request, runID string) []map[string]any {
	if s.artifacts == nil {
		return []map[string]any{}
	}
	items, err := s.artifacts.ListArtifactsByRun(r.Context(), runID, 20)
	if err != nil {
		return []map[string]any{}
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{
			"artifact_id":    item.ArtifactID,
			"artifact_type":  item.ArtifactType,
			"title":          item.Title,
			"summary":        item.Summary,
			"content_format": item.ContentFormat,
			"source_type":    item.SourceType,
			"source_ref":     item.SourceRef,
			"created_at":     item.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	return result
}

type taskListItemDTO struct {
	TaskID            string  `json:"task_id"`
	RunID             string  `json:"run_id"`
	SessionKey        string  `json:"session_key"`
	SourceChannel     string  `json:"source_channel"`
	Status            string  `json:"status"`
	RunState          string  `json:"run_state"`
	NeedsUserAction   bool    `json:"needs_user_action"`
	UserActionChannel string  `json:"user_action_channel"`
	WaitingReason     string  `json:"waiting_reason"`
	StartedAt         string  `json:"started_at"`
	UpdatedAt         string  `json:"updated_at"`
	FinishedAt        *string `json:"finished_at"`
	OutcomeSummary    string  `json:"outcome_summary"`
	ErrorSummary      string  `json:"error_summary"`
	RiskLevel         string  `json:"risk_level"`
	Provider          string  `json:"provider"`
	AutonomyMode      string  `json:"autonomy_mode"`
}

func toTaskListItemDTO(item run.TaskRow) taskListItemDTO {
	status, needsUserAction, actionChannel, waitingReason := mapTaskPresentation(item.RunState, item.SessionChannel)
	dto := taskListItemDTO{
		TaskID:            item.TaskID,
		RunID:             item.RunID,
		SessionKey:        item.SessionKey,
		SourceChannel:     item.SourceChannel,
		Status:            status,
		RunState:          item.RunState,
		NeedsUserAction:   needsUserAction,
		UserActionChannel: actionChannel,
		WaitingReason:     waitingReason,
		StartedAt:         item.StartedAt.UTC().Format(time.RFC3339),
		UpdatedAt:         item.UpdatedAt.UTC().Format(time.RFC3339),
		OutcomeSummary:    item.OutcomeSummary,
		ErrorSummary:      item.ErrorSummary,
		RiskLevel:         item.RiskLevel,
		Provider:          item.ModelProvider,
		AutonomyMode:      item.AutonomyMode,
	}
	if item.FinishedAt != nil {
		value := item.FinishedAt.UTC().Format(time.RFC3339)
		dto.FinishedAt = &value
	}
	return dto
}

func (s *TaskViewServer) loadTimelinePreview(r *http.Request, runID string) []map[string]any {
	if s.transitions == nil {
		return []map[string]any{}
	}
	transitions, err := s.transitions.ListTransitions(r.Context(), runID)
	if err != nil {
		return []map[string]any{}
	}
	max := len(transitions)
	if max > 10 {
		max = 10
	}
	items := make([]map[string]any, 0, max)
	for i := 0; i < max; i++ {
		item := transitions[i]
		items = append(items, map[string]any{
			"from_state":      item.FromState,
			"to_state":        item.ToState,
			"triggered_by":    item.TriggeredBy,
			"transitioned_at": item.TransitionedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	return items
}

func taskListParamsFromRequest(r *http.Request) (run.TaskListParams, error) {
	query := r.URL.Query()
	params := run.TaskListParams{
		Status:        strings.TrimSpace(query.Get("status")),
		WaitingReason: strings.TrimSpace(query.Get("waiting_reason")),
		SourceChannel: strings.TrimSpace(query.Get("source_channel")),
		Provider:      strings.TrimSpace(query.Get("provider")),
		Query:         strings.TrimSpace(query.Get("query")),
		Sort:          run.TaskSort(strings.TrimSpace(query.Get("sort"))),
	}

	if limitRaw := strings.TrimSpace(query.Get("limit")); limitRaw != "" {
		limit, err := strconv.Atoi(limitRaw)
		if err != nil {
			return run.TaskListParams{}, errBadRequest("limit must be integer")
		}
		params.Limit = limit
	}
	if offsetRaw := strings.TrimSpace(query.Get("offset")); offsetRaw != "" {
		offset, err := strconv.Atoi(offsetRaw)
		if err != nil {
			return run.TaskListParams{}, errBadRequest("offset must be integer")
		}
		params.Offset = offset
	}

	if actionRaw := strings.TrimSpace(query.Get("needs_user_action")); actionRaw != "" {
		value, err := strconv.ParseBool(actionRaw)
		if err != nil {
			return run.TaskListParams{}, errBadRequest("needs_user_action must be boolean")
		}
		params.NeedsUserAction = &value
	}

	if fromRaw := strings.TrimSpace(query.Get("from")); fromRaw != "" {
		value, err := parseDateTime(fromRaw)
		if err != nil {
			return run.TaskListParams{}, errBadRequest("from must be RFC3339 datetime")
		}
		params.From = &value
	}
	if toRaw := strings.TrimSpace(query.Get("to")); toRaw != "" {
		value, err := parseDateTime(toRaw)
		if err != nil {
			return run.TaskListParams{}, errBadRequest("to must be RFC3339 datetime")
		}
		params.To = &value
	}

	if params.Sort != "" {
		switch params.Sort {
		case run.TaskSortStartedAtDesc, run.TaskSortStartedAtAsc, run.TaskSortUpdatedAtDesc, run.TaskSortUpdatedAtAsc:
		default:
			return run.TaskListParams{}, errBadRequest("sort is invalid")
		}
	}

	return params, nil
}

func parseDateTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed.UTC(), nil
	}
	parsed, err = time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func mapTaskPresentation(runState, sourceChannel string) (status string, needsUserAction bool, actionChannel string, waitingReason string) {
	switch runState {
	case "created", "queued", "acquired", "preparing", "model_running", "tool_pending", "tool_running", "awaiting_model_resume", "finalizing":
		status = "in_progress"
	case "awaiting_approval":
		needsUserAction = true
		waitingReason = "approval_required"
		if strings.EqualFold(sourceChannel, "telegram") {
			status = "waiting_for_reply_in_telegram"
			actionChannel = "telegram"
		} else {
			status = "waiting_for_approval"
			actionChannel = "web"
		}
	case "completed":
		status = "completed"
	case "failed":
		status = "failed"
	case "cancelled":
		status = "cancelled"
	case "timed_out":
		status = "completed_with_issues"
	default:
		status = "in_progress"
	}

	if actionChannel == "" {
		actionChannel = "none"
	}
	if waitingReason == "" && runState == "awaiting_model_resume" {
		waitingReason = "awaiting_model_resume"
	}

	return status, needsUserAction, actionChannel, waitingReason
}

func deriveRiskLevel(runState string) string {
	switch runState {
	case "failed", "timed_out":
		return "high"
	case "awaiting_approval", "tool_pending", "tool_running":
		return "medium"
	default:
		return "low"
	}
}

func extractSourceMessage(messages []transcript.Message) (preview string, full string) {
	for _, message := range messages {
		if strings.EqualFold(message.Role, "user") {
			full = strings.TrimSpace(message.Content)
			if len(full) > 200 {
				preview = full[:200]
			} else {
				preview = full
			}
			return preview, full
		}
	}
	return "", ""
}

func extractAssistantFinal(messages []transcript.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(messages[i].Role, "assistant") {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}

func inferSourceChannel(sessionKey string) string {
	parts := strings.Split(strings.TrimSpace(sessionKey), ":")
	if len(parts) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parts[0]))
}

func waitingStateNote(status, sourceChannel string) string {
	if status == "waiting_for_reply_in_telegram" && strings.EqualFold(sourceChannel, "telegram") {
		return "Awaiting user reply in Telegram. Continue from Telegram chat."
	}
	if status == "waiting_for_approval" {
		return "Task is waiting for approval action."
	}
	return ""
}

func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}

type badRequestError string

func (e badRequestError) Error() string { return string(e) }

func errBadRequest(message string) error { return badRequestError(message) }
