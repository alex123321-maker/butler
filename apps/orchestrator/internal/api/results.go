package api

import (
	"errors"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"net/http"
	"strings"
)

type ResultServer struct{ results *completion.Coordinator }

func NewResultServer(results *completion.Coordinator) *ResultServer {
	return &ResultServer{results: results}
}

// Read-only, same private admin perimeter as existing transcript/artifact APIs.
func (s *ResultServer) HandleByRun(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, prefix)
		if id == "" || strings.Contains(id, "/") {
			w.WriteHeader(404)
			return
		}
		if s.results == nil {
			writeJSON(w, 503, map[string]string{"error": "results_unavailable"})
			return
		}
		record, err := s.results.GetByRun(r.Context(), id)
		if errors.Is(err, completion.ErrNotFound) {
			w.WriteHeader(404)
			return
		}
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "result_read_failed"})
			return
		}
		notifications, err := s.results.Outbox.ListByRun(r.Context(), id)
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "delivery_read_failed"})
			return
		}
		auxiliary, err := s.results.ListAuxiliary(r.Context(), id)
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "auxiliary_read_failed"})
			return
		}
		writeJSON(w, 200, map[string]any{"result": record, "notifications": notifications, "delivery_state": deliveryoutbox.State(notifications), "auxiliary_jobs": auxiliary, "web_ack_semantics": "result_ready_in_postgresql; client_read_unobservable"})
	})
}
