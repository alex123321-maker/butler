package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/internal/memory/transcript"
)

type tasksDebugRunReader interface {
	GetRun(ctx context.Context, runID string) (run.Record, error)
}

type tasksDebugTranscriptReader interface {
	GetRunTranscript(ctx context.Context, runID string) (transcript.Transcript, error)
}

type TasksDebugServer struct {
	runs        tasksDebugRunReader
	transcripts tasksDebugTranscriptReader
}

func NewTasksDebugServer(runs tasksDebugRunReader, transcripts tasksDebugTranscriptReader) *TasksDebugServer {
	return &TasksDebugServer{runs: runs, transcripts: transcripts}
}

func (s *TasksDebugServer) HandleGetTaskDebug(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.runs == nil || s.transcripts == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "debug dependencies are not configured"})
			return
		}
		runID := extractPathParam(r.URL.Path, prefix)
		runID = strings.TrimSuffix(runID, "/debug")
		if runID == "" || strings.Contains(runID, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task id is required"})
			return
		}
		rec, err := s.runs.GetRun(r.Context(), runID)
		if err != nil {
			if err == run.ErrRunNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load task"})
			return
		}
		tx, err := s.transcripts.GetRunTranscript(r.Context(), runID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load transcript"})
			return
		}

		messages := make([]map[string]any, 0, len(tx.Messages))
		for _, msg := range tx.Messages {
			messages = append(messages, map[string]any{
				"message_id":    msg.MessageID,
				"run_id":        msg.RunID,
				"role":          msg.Role,
				"content":       msg.Content,
				"tool_call_id":  msg.ToolCallID,
				"metadata_json": msg.MetadataJSON,
				"created_at":    msg.CreatedAt.UTC().Format(time.RFC3339),
			})
		}

		toolCalls := make([]map[string]any, 0, len(tx.ToolCalls))
		for _, tc := range tx.ToolCalls {
			toolCalls = append(toolCalls, map[string]any{
				"tool_call_id":   tc.ToolCallID,
				"tool_name":      tc.ToolName,
				"args_json":      tc.ArgsJSON,
				"status":         tc.Status,
				"runtime_target": tc.RuntimeTarget,
				"result_json":    tc.ResultJSON,
				"error_json":     tc.ErrorJSON,
				"started_at":     tc.StartedAt.UTC().Format(time.RFC3339),
				"finished_at":    formatOptionalTime(tc.FinishedAt),
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"run": map[string]any{
				"run_id":               rec.RunID,
				"session_key":          rec.SessionKey,
				"status":               rec.Status,
				"current_state":        rec.CurrentState,
				"model_provider":       rec.ModelProvider,
				"provider_session_ref": rec.ProviderSessionRef,
				"autonomy_mode":        rec.AutonomyMode,
				"metadata_json":        rec.MetadataJSON,
				"error_type":           rec.ErrorType,
				"error_message":        rec.ErrorMessage,
				"started_at":           rec.StartedAt.UTC().Format(time.RFC3339),
				"updated_at":           rec.UpdatedAt.UTC().Format(time.RFC3339),
				"finished_at":          formatOptionalTime(rec.FinishedAt),
			},
			"transcript": map[string]any{
				"messages":   messages,
				"tool_calls": toolCalls,
			},
		})
	})
}
