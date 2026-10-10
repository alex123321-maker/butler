package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
)

// Catalog APIs are private owner controls, including reads and exports. A caller
// cannot choose an owner ID, authority origin, source parents or model approval.
type MemoryCatalogServer struct {
	Store *catalog.Store
	Auth  *OwnerControlAuth
}

func (s *MemoryCatalogServer) Handle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := s.Auth.Authenticate(r)
		if !ok {
			writeJSON(w, 401, map[string]string{"error": "authenticated owner required"})
			return
		}
		if s.Store == nil {
			writeJSON(w, 503, map[string]string{"error": "memory catalog unavailable"})
			return
		}
		ctx, cancel := context.WithTimeout(owneridentity.WithActor(r.Context(), a), 30*time.Second)
		defer cancel()
		w.Header().Set("Cache-Control", "no-store")
		if r.Method == http.MethodGet {
			switch r.URL.Path {
			case "/api/v4/memory":
				v, e := s.Store.View(ctx)
				if e != nil {
					writeMemoryError(w, e)
					return
				}
				c, e := s.Store.Claims(ctx)
				if e != nil {
					writeMemoryError(w, e)
					return
				}
				writeJSON(w, 200, map[string]any{"sources": v, "claims": c, "source_list_limit": 1000, "claim_list_limit": 2000, "export_path": "/api/v4/memory/export"})
			case "/api/v4/memory/export":
				if e := s.Store.CheckRestore(ctx); e != nil {
					writeMemoryError(w, e)
					return
				}
				w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
				w.Header().Set("Content-Disposition", `attachment; filename="butler-memory.ndjson"`)
				e := s.Store.Export(ctx, json.NewEncoder(w).Encode)
				if e != nil {
					_ = json.NewEncoder(w).Encode(map[string]string{"record_type": "export_incomplete", "error": "export interrupted"})
				}
			case "/api/v4/memory/citation":
				id, e := strconv.ParseInt(r.URL.Query().Get("chunk_id"), 10, 64)
				if e != nil {
					writeMemoryError(w, catalog.ErrInvalid)
					return
				}
				v, e := s.Store.Citation(ctx, id)
				if e != nil {
					writeMemoryError(w, e)
					return
				}
				writeJSON(w, 200, v)
			default:
				w.WriteHeader(404)
			}
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		if strings.TrimSpace(r.Header.Get("X-Butler-Delivery-ID")) == "" {
			writeJSON(w, 400, map[string]string{"error": "unique owner delivery identity required"})
			return
		}
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 9*1024*1024))
		d.DisallowUnknownFields()
		decode := func(v any) bool {
			if decodeOwnerRequest(d, v) != nil {
				writeMemoryError(w, catalog.ErrInvalid)
				return false
			}
			return true
		}
		switch r.URL.Path {
		case "/api/v4/memory/sources":
			var q catalog.PutSource
			if !decode(&q) {
				return
			}
			v, e := s.Store.Put(ctx, q)
			if e != nil {
				writeMemoryError(w, e)
				return
			}
			writeJSON(w, 200, v)
		case "/api/v4/memory/correct":
			var q catalog.Correction
			if !decode(&q) {
				return
			}
			v, e := s.Store.Correct(ctx, q)
			if e != nil {
				writeMemoryError(w, e)
				return
			}
			writeJSON(w, 200, v)
		case "/api/v4/memory/claims":
			var q catalog.ClaimRequest
			if !decode(&q) {
				return
			}
			v, e := s.Store.Propose(ctx, q)
			if e != nil {
				writeMemoryError(w, e)
				return
			}
			writeJSON(w, 201, v)
		case "/api/v4/memory/decisions":
			var q catalog.Decision
			if !decode(&q) {
				return
			}
			v, e := s.Store.Decide(ctx, q)
			if e != nil {
				writeMemoryError(w, e)
				return
			}
			writeJSON(w, 200, v)
		case "/api/v4/memory/delete":
			var q struct {
				SourceID         string `json:"source_id"`
				Action           string `json:"action"`
				ExpectedRevision int64  `json:"expected_revision"`
			}
			if !decode(&q) {
				return
			}
			v, e := s.Store.Delete(ctx, q.SourceID, q.Action, q.ExpectedRevision)
			if e != nil {
				writeMemoryError(w, e)
				return
			}
			writeJSON(w, 200, v)
		case "/api/v4/memory/grants":
			var q struct {
				TaskID       string `json:"task_id"`
				SourceID     string `json:"source_id"`
				Purpose      string `json:"purpose"`
				TaskRevision int64  `json:"task_revision"`
			}
			if !decode(&q) {
				return
			}
			if e := s.Store.GrantSource(ctx, q.TaskID, q.SourceID, q.Purpose, q.TaskRevision); e != nil {
				writeMemoryError(w, e)
				return
			}
			writeJSON(w, 200, map[string]string{"status": "granted"})
		default:
			w.WriteHeader(404)
		}
	})
}
func writeMemoryError(w http.ResponseWriter, e error) {
	status, message := 409, "memory source or decision rejected"
	switch {
	case errors.Is(e, catalog.ErrRestore):
		status, message = 503, "memory deletion anchor requires operator reconciliation"
	case errors.Is(e, catalog.ErrDenied):
		status, message = 403, "current memory access denied"
	case errors.Is(e, catalog.ErrInvalid):
		status, message = 400, "invalid memory request"
	}
	writeJSON(w, status, map[string]string{"error": message})
}

// Transitional old endpoints are owner-readable only and cannot mutate memory
// without a versioned source/decision. Scope proof precedes their class queries.
func (s *MemoryCatalogServer) LegacyRead(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := s.Auth.Authenticate(r)
		if !ok {
			writeJSON(w, 401, map[string]string{"error": "authenticated owner required"})
			return
		}
		if r.Method != http.MethodGet {
			writeJSON(w, 409, map[string]string{"error": "use versioned memory source and decision API"})
			return
		}
		ctx := owneridentity.WithActor(r.Context(), a)
		if e := s.Store.CheckOwnerScope(ctx, r.URL.Query().Get("scope_type"), r.URL.Query().Get("scope_id")); e != nil {
			writeMemoryError(w, e)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
