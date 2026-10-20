// Package indexing composes source authority, owned local jobs and derived indexes.
package indexing

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/semantic"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

type Runtime struct {
	Catalog *catalog.Store
	Jobs    *jobs.Store
	Backend semantic.Backend
	Target  budget.LocalTarget
	Space   semantic.Space
	Price   budget.ModelPrice
	Reader  *semantic.HybridReader
	Log     *slog.Logger
}
type Request struct {
	Job       jobs.Request `json:"job"`
	ScopeType string       `json:"scope_type"`
	ScopeID   string       `json:"scope_id"`
	MaxChunks int          `json:"max_chunks"`
}
type Receipt struct {
	JobID        string `json:"job_id"`
	GenerationID string `json:"generation_id"`
	State        string `json:"state"`
}

func (r *Runtime) Enqueue(ctx context.Context, q Request) (Receipt, error) {
	if r == nil || q.Job.Kind != "index" || q.MaxChunks < 1 || q.MaxChunks > 10000 {
		return Receipt{}, catalog.ErrInvalid
	}
	if e := r.Catalog.CheckOwnerScope(ctx, q.ScopeType, q.ScopeID); e != nil {
		return Receipt{}, e
	}
	var generationID string
	j, e := r.Jobs.EnqueueWithPublication(ctx, q.Job, func(ctx context.Context, tx pgx.Tx, j jobs.Job) error {
		var typ, id string
		var limit int
		e := tx.QueryRow(ctx, `SELECT generation_id,scope_type,scope_id,max_chunks FROM memory_index_jobs WHERE job_id=$1`, j.ID).Scan(&generationID, &typ, &id, &limit)
		if e == nil {
			if typ != q.ScopeType || id != q.ScopeID || limit != q.MaxChunks {
				return catalog.ErrStale
			}
			return nil
		}
		if e != pgx.ErrNoRows {
			return e
		}
		s := semantic.NewStore(r.Catalog.Pool())
		g, e := s.CreateTx(ctx, tx, r.Space, semantic.Scope{Type: q.ScopeType, ID: q.ScopeID})
		if e != nil {
			return e
		}
		generationID = g.ID
		_, e = tx.Exec(ctx, `INSERT INTO memory_index_jobs(job_id,generation_id,scope_type,scope_id,max_chunks) VALUES($1,$2,$3,$4,$5)`, j.ID, g.ID, q.ScopeType, q.ScopeID, q.MaxChunks)
		return e
	})
	return Receipt{JobID: j.ID, GenerationID: generationID, State: j.Status}, e
}
func (r *Runtime) ProcessOne(ctx context.Context) (bool, error) {
	if r == nil {
		return false, catalog.ErrInvalid
	}
	j, ok, e := r.Jobs.ClaimKind(ctx, "index")
	if e != nil || !ok {
		return ok, e
	}
	bounded, stop := context.WithDeadline(ctx, j.Deadline)
	defer stop()
	var id, typ, scopeID string
	var limit int
	e = r.Catalog.Pool().QueryRow(bounded, `SELECT generation_id,scope_type,scope_id,max_chunks FROM memory_index_jobs WHERE job_id=$1`, j.ID).Scan(&id, &typ, &scopeID, &limit)
	if e == nil {
		var guard *semantic.CatalogGuard
		guard, e = semantic.NewCatalogGuard(r.Catalog, j.Purpose)
		if e == nil {
			var gate *semantic.JobAdmission
			gate, e = semantic.NewJobAdmission(r.Jobs, j, guard, r.Target, r.Price)
			if e == nil {
				s := semantic.NewStore(r.Catalog.Pool())
				s.Guard = guard
				adapter := &semantic.Adapter{Backend: r.Backend, Space: r.Space, Admission: gate}
				_, e = semantic.Rebuild(gate.Context(bounded), s, adapter, id, limit)
				if e == nil {
					e = r.publishAutomatic(gate.Context(bounded), j, s, id)
				}
			}
		}
	}
	code := ""
	retry := false
	if e != nil {
		code = "index_rejected"
		if errors.Is(e, semantic.ErrSourceLimit) {
			code = "scope_chunk_limit"
		}
		if errors.Is(e, semantic.ErrUnavailable) {
			code = "local_model_unavailable"
			retry = true
		}
	}
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	finishErr := r.Jobs.Finish(finishCtx, j, code, retry)
	if e != nil {
		return true, e
	}
	return true, finishErr
}
func (r *Runtime) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := r.Schedule(ctx); err != nil && r.Log != nil {
			r.Log.Warn("automatic memory index scheduling deferred")
		}
		worked, err := r.ProcessOne(ctx)
		if worked && err != nil && r.Log != nil {
			// Persisted job diagnostics carry the outcome; no source text, token or
			// raw model/SQL error is copied into application logs.
			r.Log.Warn("owned memory indexing attempt rejected")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Owner review activates a derived index, never turns retrieved facts into
// permissions or confirmed profile entries. A model cannot invoke this API.
func (r *Runtime) Review(ctx context.Context, id, expected string, p semantic.Proof) error {
	if _, ok := owneridentity.FromContext(ctx); !ok {
		return catalog.ErrDenied
	}
	s := semantic.NewStore(r.Catalog.Pool())
	g, e := s.Generation(ctx, id)
	if e != nil {
		return e
	}
	if e = r.Catalog.CheckOwnerScope(ctx, g.Scope.Type, g.Scope.ID); e != nil {
		return e
	}
	guard, e := semantic.NewCatalogGuard(r.Catalog, "memory_review")
	if e != nil {
		return e
	}
	s.Guard = guard
	if e = s.Verify(ctx, id, p); e != nil {
		return e
	}
	return s.Activate(ctx, id, expected)
}
func (r *Runtime) View(ctx context.Context) ([]json.RawMessage, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return nil, catalog.ErrDenied
	}
	if e := r.Catalog.CheckRestore(ctx); e != nil {
		return nil, e
	}
	rows, e := r.Catalog.Pool().Query(ctx, `SELECT to_jsonb(j)-ARRAY['sources','authority','claim_id']||to_jsonb(i) FROM memory_aux_jobs j JOIN memory_index_jobs i USING(job_id) WHERE j.owner_id=$1 ORDER BY j.created_at DESC LIMIT 100`, a.OwnerID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var row json.RawMessage
		if e = rows.Scan(&row); e != nil {
			return nil, e
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
