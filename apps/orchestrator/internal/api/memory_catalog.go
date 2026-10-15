package api

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/compaction"
	"github.com/butler/butler/internal/memory/consolidation"
	"github.com/butler/butler/internal/memory/indexing"
	"github.com/butler/butler/internal/memory/ingestion"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/semantic"
	"github.com/butler/butler/internal/owneridentity"
)

// Catalog APIs are private owner controls, including reads and exports. A caller
// cannot choose an owner ID, authority origin, source parents or model approval.
type MemoryCatalogServer struct {
	Store                 *catalog.Store
	Auth                  *OwnerControlAuth
	Ingestion             *ingestion.Store
	Indexing              *indexing.Runtime
	ModelJobs             *jobs.Store
	ModelWorkerConfigured bool
	SummaryPolicies       *compaction.Policies
	TemporalEnabled       bool
	Consolidation         *consolidation.Store
	ConsolidationReady    bool
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
		if s.handleIndexRetirement(ctx, w, r) {
			return
		}
		if s.handleModelJobs(ctx, w, r) {
			return
		}
		if s.handleTemporal(ctx, w, r) {
			return
		}
		if s.handleConsolidation(ctx, w, r) {
			return
		}
		if r.Method == http.MethodGet {
			switch r.URL.Path {
			case "/api/v4/memory/original":
				if s.Ingestion == nil {
					writeJSON(w, 503, map[string]string{"error": "source ingestion unavailable"})
					return
				}
				revision, e := strconv.ParseInt(r.URL.Query().Get("revision"), 10, 64)
				if e != nil {
					writeMemoryError(w, catalog.ErrInvalid)
					return
				}
				raw, path, e := s.Ingestion.Original(ctx, r.URL.Query().Get("source_id"), revision)
				if e != nil {
					writeMemoryError(w, e)
					return
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("X-Content-Type-Options", "nosniff")
				w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(path)}))
				_, _ = w.Write(raw)
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
				var indexJobs any = []any{}
				if s.Indexing != nil {
					indexJobs, e = s.Indexing.View(ctx)
					if e != nil {
						writeMemoryError(w, e)
						return
					}
				}
				writeJSON(w, 200, map[string]any{"sources": v, "claims": c, "index_jobs": indexJobs, "semantic_enabled": s.Indexing != nil, "source_list_limit": 1000, "claim_list_limit": 2000, "export_path": "/api/v4/memory/export"})
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
			case "/api/v4/memory/source":
				v, e := s.Store.Inspect(ctx, r.URL.Query().Get("source_id"))
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
		d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 12*1024*1024))
		d.DisallowUnknownFields()
		decode := func(v any) bool {
			if decodeOwnerRequest(d, v) != nil {
				writeMemoryError(w, catalog.ErrInvalid)
				return false
			}
			return true
		}
		switch r.URL.Path {
		case "/api/v4/memory/index":
			if s.Indexing == nil {
				writeJSON(w, 503, map[string]string{"error": "owned local indexing unavailable"})
				return
			}
			var q indexing.Request
			if !decode(&q) {
				return
			}
			v, e := s.Indexing.Enqueue(ctx, q)
			if e != nil {
				writeMemoryError(w, e)
				return
			}
			writeJSON(w, 202, v)
		case "/api/v4/memory/index-review":
			if s.Indexing == nil {
				writeJSON(w, 503, map[string]string{"error": "owned local indexing unavailable"})
				return
			}
			var q struct {
				GenerationID     string         `json:"generation_id"`
				ExpectedPrevious string         `json:"expected_previous"`
				Proof            semantic.Proof `json:"proof"`
			}
			if !decode(&q) {
				return
			}
			if e := s.Indexing.Review(ctx, q.GenerationID, q.ExpectedPrevious, q.Proof); e != nil {
				writeMemoryError(w, e)
				return
			}
			writeJSON(w, 200, map[string]string{"status": "ready"})
		case "/api/v4/memory/ingest":
			if s.Ingestion == nil {
				writeJSON(w, 503, map[string]string{"error": "source ingestion unavailable"})
				return
			}
			var q ingestion.Upload
			if !decode(&q) {
				return
			}
			v, e := s.Ingestion.Ingest(ctx, q)
			if e != nil {
				writeMemoryError(w, e)
				return
			}
			writeJSON(w, 200, v)
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
	case errors.Is(e, ingestion.ErrLimit):
		status, message = 413, "source exceeds parsing limits"
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
