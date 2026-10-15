package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
)

func (s *MemoryCatalogServer) handleTemporal(ctx context.Context, w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/v4/memory/temporal" && r.URL.Path != "/api/v4/memory/facts" {
		return false
	}
	if !s.TemporalEnabled {
		writeJSON(w, 503, map[string]string{"error": "temporal memory is not qualified or enabled"})
		return true
	}
	if r.URL.Path == "/api/v4/memory/facts" && r.Method == http.MethodGet {
		q := catalog.TemporalQuery{}
		for key, target := range map[string]**time.Time{"valid_at": &q.ValidAt, "known_at": &q.KnownAt} {
			raw := r.URL.Query().Get(key)
			if raw == "" {
				continue
			}
			v, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil || v.IsZero() {
				writeMemoryError(w, catalog.ErrInvalid)
				return true
			}
			v = v.UTC()
			*target = &v
		}
		page, err := s.Store.ReadAcceptedFacts(ctx, r.URL.Query()["claim_id"], q)
		if err != nil {
			writeMemoryError(w, err)
		} else {
			writeJSON(w, 200, page)
		}
		return true
	}
	if r.URL.Path != "/api/v4/memory/temporal" || r.Method != http.MethodPost {
		w.WriteHeader(405)
		return true
	}
	if strings.TrimSpace(r.Header.Get("X-Butler-Delivery-ID")) == "" {
		writeJSON(w, 400, map[string]string{"error": "unique owner delivery identity required"})
		return true
	}
	var q catalog.TemporalUpdate
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	d.DisallowUnknownFields()
	if decodeOwnerRequest(d, &q) != nil {
		writeMemoryError(w, catalog.ErrInvalid)
		return true
	}
	v, err := s.Store.SetTemporal(ctx, q)
	if err != nil {
		writeMemoryError(w, err)
	} else {
		writeJSON(w, 200, v)
	}
	return true
}
