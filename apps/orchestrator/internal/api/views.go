package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/transcript"
)

// SessionLister lists sessions with pagination.
type SessionLister interface {
	ListSessions(ctx context.Context, limit, offset int) ([]session.SessionRecord, error)
	GetSessionByKey(ctx context.Context, sessionKey string) (session.SessionRecord, error)
}

// RunLister lists runs for a session and fetches individual runs.
type RunLister interface {
	ListRunsBySessionKey(ctx context.Context, sessionKey string) ([]run.Record, error)
	GetRun(ctx context.Context, runID string) (run.Record, error)
}

// TranscriptReader retrieves run transcripts.
type TranscriptReader interface {
	GetRunTranscript(ctx context.Context, runID string) (transcript.Transcript, error)
}

// ViewServer serves read-only REST endpoints for sessions, runs, and transcripts.
type ViewServer struct {
	sessions    SessionLister
	runs        RunLister
	transcripts TranscriptReader
	log         *slog.Logger
}

// NewViewServer creates a new ViewServer.
func NewViewServer(sessions SessionLister, runs RunLister, transcripts TranscriptReader, log *slog.Logger) *ViewServer {
	if log == nil {
		log = slog.Default()
	}
	return &ViewServer{
		sessions:    sessions,
		runs:        runs,
		transcripts: transcripts,
		log:         logger.WithComponent(log, "view-api"),
	}
}

// HandleListSessions handles GET /api/v1/sessions
func (v *ViewServer) HandleListSessions() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		if limit <= 0 {
			limit = 50
		}

		sessions, err := v.sessions.ListSessions(r.Context(), limit, offset)
		if err != nil {
			v.log.Error("list sessions failed", slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sessions"})
			return
		}
		if sessions == nil {
			sessions = []session.SessionRecord{}
		}

		items := make([]sessionDTO, 0, len(sessions))
		for _, s := range sessions {
			items = append(items, toSessionDTO(s))
		}
		writeJSON(w, http.StatusOK, map[string]any{"sessions": items})
	})
}

// HandleGetSession handles GET /api/v1/sessions/{key}
func (v *ViewServer) HandleGetSession() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		key := extractPathParam(r.URL.Path, "/api/v1/sessions/")
		if key == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session key is required"})
			return
		}

		sess, err := v.sessions.GetSessionByKey(r.Context(), key)
		if err != nil {
			if err == session.ErrSessionNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
				return
			}
			v.log.Error("get session failed", slog.String("session_key", key), slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get session"})
			return
		}

		runs, err := v.runs.ListRunsBySessionKey(r.Context(), key)
		if err != nil {
			v.log.Error("list runs for session failed", slog.String("session_key", key), slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list runs"})
			return
		}
		if runs == nil {
			runs = []run.Record{}
		}

		runDTOs := make([]runDTO, 0, len(runs))
		for _, r := range runs {
			runDTOs = append(runDTOs, toRunDTO(r))
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"session": toSessionDTO(sess),
			"runs":    runDTOs,
		})
	})
}

// HandleGetRun handles GET /api/v1/runs/{id}
func (v *ViewServer) HandleGetRun() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		runID := extractPathParam(r.URL.Path, "/api/v1/runs/")
		// Strip /transcript suffix if present
		runID = strings.TrimSuffix(runID, "/transcript")
		if runID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run id is required"})
			return
		}

		rec, err := v.runs.GetRun(r.Context(), runID)
		if err != nil {
			if err == run.ErrRunNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
				return
			}
			v.log.Error("get run failed", slog.String("run_id", runID), slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get run"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"run": toRunDTO(rec)})
	})
}

// HandleGetRunTranscript handles GET /api/v1/runs/{id}/transcript
func (v *ViewServer) HandleGetRunTranscript() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Path: /api/v1/runs/{id}/transcript
		runID := extractPathParam(r.URL.Path, "/api/v1/runs/")
		runID = strings.TrimSuffix(runID, "/transcript")
		if runID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run id is required"})
			return
		}

		// Verify run exists
		rec, err := v.runs.GetRun(r.Context(), runID)
		if err != nil {
			if err == run.ErrRunNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
				return
			}
			v.log.Error("get run for transcript failed", slog.String("run_id", runID), slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get run"})
			return
		}

		tx, err := v.transcripts.GetRunTranscript(r.Context(), runID)
		if err != nil {
			v.log.Error("get run transcript failed", slog.String("run_id", runID), slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get transcript"})
			return
		}

		messages := make([]messageDTO, 0, len(tx.Messages))
		for _, m := range tx.Messages {
			messages = append(messages, toMessageDTO(m))
		}
		toolCalls := make([]toolCallDTO, 0, len(tx.ToolCalls))
		for _, tc := range tx.ToolCalls {
			toolCalls = append(toolCalls, toToolCallDTO(tc))
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"run":        toRunDTO(rec),
			"messages":   messages,
			"tool_calls": toolCalls,
		})
	})
}

// --- DTOs ---

type sessionDTO struct {
	SessionKey string `json:"session_key"`
	UserID     string `json:"user_id"`
	Channel    string `json:"channel"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type runDTO struct {
	RunID         string  `json:"run_id"`
	SessionKey    string  `json:"session_key"`
	Status        string  `json:"status"`
	CurrentState  string  `json:"current_state"`
	ModelProvider string  `json:"model_provider"`
	AutonomyMode  string  `json:"autonomy_mode"`
	StartedAt     string  `json:"started_at"`
	UpdatedAt     string  `json:"updated_at"`
	FinishedAt    *string `json:"finished_at"`
	ErrorType     string  `json:"error_type,omitempty"`
	ErrorMessage  string  `json:"error_message,omitempty"`
}

type messageDTO struct {
	MessageID  string `json:"message_id"`
	RunID      string `json:"run_id"`
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id,omitempty"`
	CreatedAt  string `json:"created_at"`
}

type toolCallDTO struct {
	ToolCallID    string  `json:"tool_call_id"`
	RunID         string  `json:"run_id"`
	ToolName      string  `json:"tool_name"`
	ArgsJSON      string  `json:"args_json"`
	Status        string  `json:"status"`
	RuntimeTarget string  `json:"runtime_target"`
	StartedAt     string  `json:"started_at"`
	FinishedAt    *string `json:"finished_at"`
	ResultJSON    string  `json:"result_json"`
	ErrorJSON     string  `json:"error_json,omitempty"`
}

func toSessionDTO(s session.SessionRecord) sessionDTO {
	return sessionDTO{
		SessionKey: s.SessionKey,
		UserID:     s.UserID,
		Channel:    s.Channel,
		CreatedAt:  s.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  s.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toRunDTO(r run.Record) runDTO {
	dto := runDTO{
		RunID:         r.RunID,
		SessionKey:    r.SessionKey,
		Status:        r.Status,
		CurrentState:  r.CurrentState,
		ModelProvider: r.ModelProvider,
		AutonomyMode:  r.AutonomyMode,
		StartedAt:     r.StartedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     r.UpdatedAt.UTC().Format(time.RFC3339),
		ErrorType:     r.ErrorType,
		ErrorMessage:  r.ErrorMessage,
	}
	if r.FinishedAt != nil {
		finished := r.FinishedAt.UTC().Format(time.RFC3339)
		dto.FinishedAt = &finished
	}
	return dto
}

func toMessageDTO(m transcript.Message) messageDTO {
	return messageDTO{
		MessageID:  m.MessageID,
		RunID:      m.RunID,
		Role:       m.Role,
		Content:    m.Content,
		ToolCallID: m.ToolCallID,
		CreatedAt:  m.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func toToolCallDTO(tc transcript.ToolCall) toolCallDTO {
	dto := toolCallDTO{
		ToolCallID:    tc.ToolCallID,
		RunID:         tc.RunID,
		ToolName:      tc.ToolName,
		ArgsJSON:      tc.ArgsJSON,
		Status:        tc.Status,
		RuntimeTarget: tc.RuntimeTarget,
		StartedAt:     tc.StartedAt.UTC().Format(time.RFC3339),
		ResultJSON:    tc.ResultJSON,
		ErrorJSON:     tc.ErrorJSON,
	}
	if tc.FinishedAt != nil {
		finished := tc.FinishedAt.UTC().Format(time.RFC3339)
		dto.FinishedAt = &finished
	}
	return dto
}

// extractPathParam returns the portion of the path after the given prefix.
func extractPathParam(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	return strings.TrimPrefix(path, prefix)
}
