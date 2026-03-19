package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/observability"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/internal/logger"
)

// TransitionLister lists state transitions for a run.
type TransitionLister interface {
	ListTransitions(ctx context.Context, runID string) ([]run.StateTransition, error)
}

// EventServer serves SSE and REST endpoints for run observability events.
type EventServer struct {
	transitions TransitionLister
	runs        RunLister
	hub         *observability.Hub
	log         *slog.Logger
}

// NewEventServer creates a new EventServer.
func NewEventServer(transitions TransitionLister, runs RunLister, hub *observability.Hub, log *slog.Logger) *EventServer {
	if log == nil {
		log = slog.Default()
	}
	return &EventServer{
		transitions: transitions,
		runs:        runs,
		hub:         hub,
		log:         logger.WithComponent(log, "event-api"),
	}
}

// HandleSSE handles GET /api/v1/runs/{id}/events — SSE stream with catch-up replay.
func (e *EventServer) HandleSSE() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		runID := extractRunIDFromEventsPath(r.URL.Path)
		if runID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run id is required"})
			return
		}

		// Verify run exists.
		rec, err := e.runs.GetRun(r.Context(), runID)
		if err != nil {
			if err == run.ErrRunNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
				return
			}
			e.log.Error("get run for events failed", slog.String("run_id", runID), slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get run"})
			return
		}

		// Set SSE headers.
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		// Phase 1: Catch-up replay from DB transitions.
		if e.transitions != nil {
			transitions, listErr := e.transitions.ListTransitions(r.Context(), runID)
			if listErr != nil {
				e.log.Warn("failed to load transitions for replay", slog.String("run_id", runID), slog.String("error", listErr.Error()))
			} else {
				for _, t := range transitions {
					evt := observability.Event{
						EventID:   fmt.Sprintf("replay-%d", t.ID),
						RunID:     t.RunID,
						EventType: observability.EventStateTransition,
						Timestamp: t.TransitionedAt,
						Payload: map[string]any{
							"from_state": t.FromState,
							"to_state":   t.ToState,
							"replay":     true,
						},
					}
					writeSSEEvent(w, evt)
				}
				flusher.Flush()
			}
		}

		// If run is already in a terminal state, send completion and close.
		if isTerminalState(rec.CurrentState) {
			writeSSEEvent(w, observability.Event{
				EventID:   "stream-end",
				RunID:     runID,
				EventType: "stream_end",
				Timestamp: time.Now().UTC(),
				Payload:   map[string]any{"reason": "run_terminal", "final_state": rec.CurrentState},
			})
			flusher.Flush()
			return
		}

		// Phase 2: Subscribe to live events from EventHub.
		if e.hub == nil {
			writeSSEEvent(w, observability.Event{
				EventID:   "stream-end",
				RunID:     runID,
				EventType: "stream_end",
				Timestamp: time.Now().UTC(),
				Payload:   map[string]any{"reason": "no_event_hub"},
			})
			flusher.Flush()
			return
		}

		eventCh, cancel := e.hub.Subscribe(runID)
		defer cancel()

		// Send heartbeat to keep connection alive.
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case evt, ok := <-eventCh:
				if !ok {
					// Channel closed (run cleanup or unsubscribe).
					writeSSEEvent(w, observability.Event{
						EventID:   "stream-end",
						RunID:     runID,
						EventType: "stream_end",
						Timestamp: time.Now().UTC(),
						Payload:   map[string]any{"reason": "channel_closed"},
					})
					flusher.Flush()
					return
				}
				writeSSEEvent(w, evt)
				flusher.Flush()

				// If this is a terminal event, close the stream.
				if evt.EventType == observability.EventRunCompleted || evt.EventType == observability.EventRunError {
					writeSSEEvent(w, observability.Event{
						EventID:   "stream-end",
						RunID:     runID,
						EventType: "stream_end",
						Timestamp: time.Now().UTC(),
						Payload:   map[string]any{"reason": "run_terminal"},
					})
					flusher.Flush()
					return
				}
			case <-heartbeat.C:
				fmt.Fprintf(w, ": heartbeat\n\n")
				flusher.Flush()
			}
		}
	})
}

// HandleListTransitions handles GET /api/v1/runs/{id}/transitions — REST endpoint for historical transitions.
func (e *EventServer) HandleListTransitions() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		runID := extractRunIDFromTransitionsPath(r.URL.Path)
		if runID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run id is required"})
			return
		}

		// Verify run exists.
		if _, err := e.runs.GetRun(r.Context(), runID); err != nil {
			if err == run.ErrRunNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "run not found"})
				return
			}
			e.log.Error("get run for transitions failed", slog.String("run_id", runID), slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get run"})
			return
		}

		if e.transitions == nil {
			writeJSON(w, http.StatusOK, map[string]any{"transitions": []any{}})
			return
		}

		transitions, err := e.transitions.ListTransitions(r.Context(), runID)
		if err != nil {
			e.log.Error("list transitions failed", slog.String("run_id", runID), slog.String("error", err.Error()))
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list transitions"})
			return
		}

		dtos := make([]transitionDTO, 0, len(transitions))
		for _, t := range transitions {
			dtos = append(dtos, toTransitionDTO(t))
		}
		writeJSON(w, http.StatusOK, map[string]any{"transitions": dtos})
	})
}

// --- DTOs ---

type transitionDTO struct {
	ID             int64  `json:"id"`
	RunID          string `json:"run_id"`
	FromState      string `json:"from_state"`
	ToState        string `json:"to_state"`
	TriggeredBy    string `json:"triggered_by"`
	MetadataJSON   string `json:"metadata_json"`
	TransitionedAt string `json:"transitioned_at"`
}

func toTransitionDTO(t run.StateTransition) transitionDTO {
	return transitionDTO{
		ID:             t.ID,
		RunID:          t.RunID,
		FromState:      t.FromState,
		ToState:        t.ToState,
		TriggeredBy:    t.TriggeredBy,
		MetadataJSON:   t.MetadataJSON,
		TransitionedAt: t.TransitionedAt.UTC().Format(time.RFC3339Nano),
	}
}

// --- SSE helpers ---

func writeSSEEvent(w http.ResponseWriter, evt observability.Event) {
	data, err := json.Marshal(evt)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.EventType, string(data))
}

func isTerminalState(state string) bool {
	switch state {
	case "completed", "failed", "cancelled", "timed_out":
		return true
	}
	return false
}

// extractRunIDFromEventsPath extracts the run ID from /api/v1/runs/{id}/events
func extractRunIDFromEventsPath(path string) string {
	const prefix = "/api/v1/runs/"
	const suffix = "/events"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return ""
	}
	// Remove prefix and suffix, check that a separator slash existed.
	middle := strings.TrimPrefix(path, prefix)
	// middle should be "{id}/events" — find the last /events.
	idx := strings.LastIndex(middle, suffix)
	if idx <= 0 {
		return ""
	}
	runID := strings.TrimSpace(middle[:idx])
	if runID == "" || strings.Contains(runID, "/") {
		return ""
	}
	return runID
}

// extractRunIDFromTransitionsPath extracts the run ID from /api/v1/runs/{id}/transitions
func extractRunIDFromTransitionsPath(path string) string {
	const prefix = "/api/v1/runs/"
	const suffix = "/transitions"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return ""
	}
	middle := strings.TrimPrefix(path, prefix)
	idx := strings.LastIndex(middle, suffix)
	if idx <= 0 {
		return ""
	}
	runID := strings.TrimSpace(middle[:idx])
	if runID == "" || strings.Contains(runID, "/") {
		return ""
	}
	return runID
}
