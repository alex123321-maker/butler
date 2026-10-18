package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/consolidation"
	"github.com/butler/butler/internal/memory/jobs"
)

// These are authenticated owner controls. No model tool can prepare a policy,
// select a version, mint a context or supply an accepted-fact decision.
func (s *MemoryCatalogServer) handleConsolidation(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	const prefix = "/api/v4/memory/consolidation"
	if r.URL.Path != prefix && !strings.HasPrefix(r.URL.Path, prefix+"/") {
		return false
	}
	if s.Consolidation == nil {
		writeJSON(w, 503, map[string]string{"error": "consolidation storage unavailable"})
		return true
	}
	reply := func(value any, err error) {
		if err != nil {
			if errors.Is(err, jobs.ErrWaiting) {
				writeJSON(w, 409, map[string]string{"error": "finite consolidation retention or quiet window unavailable"})
			} else if errors.Is(err, jobs.ErrClaim) {
				writeMemoryError(w, catalog.ErrStale)
			} else {
				writeMemoryError(w, err)
			}
		} else {
			writeJSON(w, 200, value)
		}
	}
	if r.Method == http.MethodGet {
		switch r.URL.Path {
		case prefix:
			value, err := s.Consolidation.List(ctx)
			reply(value, err)
		case prefix + "/preview":
			value, err := s.Consolidation.Preview(ctx, r.URL.Query().Get("source_id"))
			reply(value, err)
		default:
			w.WriteHeader(404)
		}
		return true
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return true
	}
	if strings.TrimSpace(r.Header.Get("X-Butler-Delivery-ID")) == "" {
		writeJSON(w, 400, map[string]string{"error": "unique owner delivery identity required"})
		return true
	}
	if (r.URL.Path == prefix+"/prepare" || r.URL.Path == prefix+"/activate") && !s.ConsolidationReady {
		writeJSON(w, 503, map[string]string{"error": "consolidation consumer is not qualified or enabled"})
		return true
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
	d.DisallowUnknownFields()
	decode := func(q any) bool {
		if decodeOwnerRequest(d, q) != nil {
			writeMemoryError(w, catalog.ErrInvalid)
			return false
		}
		return true
	}
	switch r.URL.Path {
	case prefix + "/prepare":
		var q consolidation.PrepareRequest
		if !decode(&q) {
			return true
		}
		value, err := s.Consolidation.Prepare(ctx, q)
		reply(value, err)
	case prefix + "/activate":
		var q consolidation.Activation
		if !decode(&q) {
			return true
		}
		value, err := s.Consolidation.Activate(ctx, q)
		reply(value, err)
	case prefix + "/deactivate":
		var q consolidation.Activation
		if !decode(&q) {
			return true
		}
		value, err := s.Consolidation.Deactivate(ctx, q)
		reply(value, err)
	case prefix + "/retire":
		var q consolidation.Retirement
		if !decode(&q) {
			return true
		}
		value, err := s.Consolidation.Retire(ctx, q)
		reply(value, err)
	default:
		w.WriteHeader(404)
	}
	return true
}
