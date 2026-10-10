package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
)

type InputReader interface {
	Get(context.Context, string) (durableinput.Record, error)
}
type InputServer struct{ store InputReader }

func NewInputServer(store InputReader) *InputServer { return &InputServer{store: store} }
func (s *InputServer) HandleItem() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/v3/inputs/")
		if id == "" || strings.Contains(id, "/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		record, err := s.store.Get(r.Context(), id)
		if errors.Is(err, durableinput.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "input not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "input read failed"})
			return
		}
		writeJSON(w, http.StatusOK, record)
	})
}
