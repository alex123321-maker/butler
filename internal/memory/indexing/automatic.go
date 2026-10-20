package indexing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/semantic"
	"github.com/jackc/pgx/v5"
)

// CreatePolicy binds the owner's reviewed fixed model space to finite local
// maintenance. This never accepts a memory claim or authorizes cloud inference.
func (r *Runtime) CreatePolicy(ctx context.Context, p jobs.IndexPolicy) (jobs.IndexPolicy, error) {
	var proof semantic.Proof
	if p.SpaceID != r.Space.ID() || json.Unmarshal(p.Proof, &proof) != nil || !semantic.ValidProof(proof) {
		return p, semantic.ErrUnverified
	}
	return r.Jobs.CreateIndexPolicy(ctx, p)
}

func (r *Runtime) Schedule(ctx context.Context) error {
	policies, e := r.Jobs.IndexPolicies(ctx)
	if e != nil {
		return e
	}
	var failures error
	for _, p := range policies {
		if e = r.schedule(ctx, p); e != nil && !errors.Is(e, jobs.ErrWaiting) {
			failures = errors.Join(failures, e)
		}
		_, _ = r.Catalog.Pool().Exec(ctx, `UPDATE memory_index_policies SET next_at=clock_timestamp()+interval '30 seconds' WHERE policy_id=$1`, p.ID)
	}
	return failures
}
func (r *Runtime) schedule(ctx context.Context, p jobs.IndexPolicy) error {
	if p.SpaceID != r.Space.ID() {
		return semantic.ErrInvalid
	}
	owned := r.Jobs.IndexPolicyContext(ctx, p)
	// Repair notes saved before atomic fragment publication was introduced.
	rows, e := r.Catalog.Pool().Query(ctx, `SELECT s.source_id,s.head_revision,s.access_revision,v.content_hash FROM memory_sources s JOIN memory_source_revisions v ON v.source_id=s.source_id AND v.revision=s.head_revision WHERE s.owner_id=$1 AND s.scope_type='user' AND s.scope_id=$1 AND s.status='active' AND v.locator->>'adapter'='android_message_note_v1' AND NOT EXISTS(SELECT 1 FROM memory_object_sources o WHERE o.source_id=s.source_id AND o.memory_type='chunk') ORDER BY s.source_id LIMIT 20`, p.OwnerID)
	if e != nil {
		return e
	}
	var repair []catalog.SourceRef
	for rows.Next() {
		var ref catalog.SourceRef
		if e = rows.Scan(&ref.ID, &ref.Revision, &ref.AccessRevision, &ref.Hash); e != nil {
			rows.Close()
			return e
		}
		repair = append(repair, ref)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, ref := range repair {
		tx, e := r.Catalog.Pool().Begin(owned)
		if e != nil {
			return e
		}
		src, e := r.Catalog.AccessTx(owned, tx, []catalog.SourceRef{ref}, "memory_ingest")
		if e != nil {
			tx.Rollback(ctx)
			continue
		}
		if len(src) != 1 {
			tx.Rollback(ctx)
			return catalog.ErrInvalid
		}
		_, e = r.Catalog.PublishFragmentsTx(owned, tx, ref, []catalog.Fragment{{Key: "note", Title: src[0].Title, Text: src[0].Body, Locator: src[0].Locator}})
		if e != nil {
			tx.Rollback(ctx)
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
	}
	rows, e = r.Catalog.Pool().Query(ctx, `SELECT c.id,c.updated_at,s.source_id,s.head_revision,s.access_revision,v.content_hash FROM memory_chunks c JOIN memory_object_sources o ON o.object_id=c.id AND o.memory_type='chunk' JOIN memory_sources s ON s.source_id=o.source_id JOIN memory_source_revisions v ON v.source_id=s.source_id AND v.revision=s.head_revision WHERE c.scope_type='user' AND c.scope_id=$1 AND s.owner_id=$1 AND s.status='active' ORDER BY c.id LIMIT $2`, p.OwnerID, p.MaxChunks+1)
	if e != nil {
		return e
	}
	var refs []catalog.SourceRef
	seen := map[string]bool{}
	h := sha256.New()
	count := 0
	for rows.Next() {
		var id int64
		var at time.Time
		var ref catalog.SourceRef
		if e = rows.Scan(&id, &at, &ref.ID, &ref.Revision, &ref.AccessRevision, &ref.Hash); e != nil {
			rows.Close()
			return e
		}
		count++
		raw, _ := json.Marshal([]any{id, at, ref})
		h.Write(raw)
		if !seen[ref.ID] {
			refs = append(refs, ref)
			seen[ref.ID] = true
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	if count == 0 {
		return nil
	}
	if count > p.MaxChunks {
		return semantic.ErrSourceLimit
	}
	fingerprint := hex.EncodeToString(h.Sum(nil))
	if p.LastJobID != "" {
		var status string
		if e = r.Catalog.Pool().QueryRow(ctx, `SELECT status FROM memory_aux_jobs WHERE job_id=$1`, p.LastJobID).Scan(&status); e != nil {
			return e
		}
		if status == "pending" || status == "running" || p.LastFingerprint == fingerprint && status == "done" {
			return nil
		}
	}
	id, e := budget.NewOperationID()
	if e != nil {
		return e
	}
	q := jobs.Request{ID: "auto-index:" + id, Kind: "index", Purpose: "memory_index_maintenance", Sources: refs, Allowance: p.Allowance, Deadline: time.Now().UTC().Add(5 * time.Minute)}
	if p.ExpiresAt.Before(q.Deadline) {
		q.Deadline = p.ExpiresAt
	}
	_, e = r.Jobs.EnqueueIndexPolicy(owned, p, fingerprint, q, func(ctx context.Context, tx pgx.Tx, j jobs.Job) error {
		store := semantic.NewStore(r.Catalog.Pool())
		g, e := store.CreateTx(ctx, tx, r.Space, semantic.Scope{Type: "user", ID: p.OwnerID})
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, `INSERT INTO memory_index_jobs(job_id,generation_id,scope_type,scope_id,max_chunks) VALUES($1,$2,'user',$3,$4)`, j.ID, g.ID, p.OwnerID, p.MaxChunks)
		return e
	})
	return e
}

func (r *Runtime) publishAutomatic(ctx context.Context, j jobs.Job, store *semantic.Store, generationID string) error {
	var expected string
	e := r.Catalog.Pool().QueryRow(ctx, `SELECT expected_previous FROM memory_index_policy_jobs WHERE job_id=$1`, j.ID).Scan(&expected)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	if e = store.VerifyMaintenance(ctx, generationID, j.ID, r.Space.ID()); e != nil {
		return e
	}
	return store.Activate(ctx, generationID, expected)
}
