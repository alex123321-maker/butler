package catalog

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/actionauth"

	"github.com/jackc/pgx/v5"
)

// Propose records extraction confidence separately from acceptance. Even an
// authenticated owner upload containing a rule starts as a candidate.
func (s *Store) Propose(ctx context.Context, p ClaimRequest) (Claim, error) {
	if !bounded(p.Key, 512) || !json.Valid(p.Value) || len(p.Value) > 32*1024 || len(p.Summary) > 8192 || p.Confidence < 0 || p.Confidence > 1 {
		return Claim{}, ErrInvalid
	}
	if e := s.CheckRestore(ctx); e != nil {
		return Claim{}, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Claim{}, e
	}
	defer tx.Rollback(ctx)
	sources, e := s.AccessTx(ctx, tx, []SourceRef{{ID: p.SourceID, Revision: p.SourceRevision}}, "memory_extract")
	if e != nil {
		return Claim{}, e
	}
	src := sources[0]
	id, e := randomID("claim_")
	if e != nil {
		return Claim{}, e
	}
	c := Claim{ID: id, OwnerID: src.OwnerID, ScopeType: src.ScopeType, ScopeID: src.ScopeID, Key: p.Key, SourceID: src.ID, SourceRevision: src.Revision, Value: p.Value, Summary: p.Summary, Status: "candidate", Confidence: p.Confidence, EventTime: src.EventTime}
	if _, e = tx.Exec(ctx, `INSERT INTO memory_claims(claim_id,owner_id,scope_type,scope_id,fact_key,source_id,source_revision,value_json,summary,extraction_confidence,event_time) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, c.ID, c.OwnerID, c.ScopeType, c.ScopeID, c.Key, c.SourceID, c.SourceRevision, c.Value, c.Summary, c.Confidence, c.EventTime); e != nil {
		return Claim{}, e
	}
	return c, tx.Commit(ctx)
}

// Decide requires an exact current active claim. A new document's event time or
// upload time cannot resolve a contradiction. Source and scope are immutable.
func (s *Store) Decide(ctx context.Context, d Decision) (Claim, error) {
	own, e := owner(ctx)
	if e != nil {
		return Claim{}, e
	}
	if e = s.CheckRestore(ctx); e != nil {
		return Claim{}, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Claim{}, e
	}
	defer tx.Rollback(ctx)
	c, e := getClaim(ctx, tx, d.ClaimID, false)
	if e != nil || c.OwnerID != own {
		return Claim{}, ErrDenied
	}
	if _, e = s.AccessTx(ctx, tx, []SourceRef{{ID: c.SourceID, Revision: c.SourceRevision}}, "memory_accept"); e != nil {
		return Claim{}, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO memory_fact_heads(owner_id,scope_type,scope_id,fact_key) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, own, c.ScopeType, c.ScopeID, c.Key); e != nil {
		return Claim{}, e
	}
	var active string
	if e = tx.QueryRow(ctx, `SELECT active_claim_id FROM memory_fact_heads WHERE owner_id=$1 AND scope_type=$2 AND scope_id=$3 AND fact_key=$4 FOR UPDATE`, own, c.ScopeType, c.ScopeID, c.Key).Scan(&active); e != nil {
		return Claim{}, e
	}
	c, e = getClaim(ctx, tx, d.ClaimID, true)
	if e != nil {
		return Claim{}, e
	}
	if c.Status != "candidate" || active != d.ExpectedActiveID {
		return Claim{}, ErrStale
	}
	c.AcceptedBy = own
	if d.Reject {
		c.Status = "rejected"
		_, e = tx.Exec(ctx, `UPDATE memory_claims SET status='rejected',accepted_by=$2,accepted_at=clock_timestamp() WHERE claim_id=$1`, c.ID, own)
		if e != nil {
			return Claim{}, e
		}
		return c, tx.Commit(ctx)
	}
	var previous *int64
	if active != "" {
		var profileID *int64
		if e = tx.QueryRow(ctx, `UPDATE memory_claims SET status='superseded' WHERE claim_id=$1 AND status='confirmed' RETURNING profile_id`, active).Scan(&profileID); e != nil {
			return Claim{}, ErrStale
		}
		previous = profileID
		if profileID != nil {
			if _, e = tx.Exec(ctx, `UPDATE memory_profile SET status='superseded',effective_status='inactive',effective_to=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1`, *profileID); e != nil {
				return Claim{}, e
			}
		}
	}
	var profileID int64
	provenance := mustJSON(map[string]any{"kind": "memory_derived", "sensitivity": "private", "source_refs": []string{c.SourceID}, "catalog": map[string]any{"source_id": c.SourceID, "revision": c.SourceRevision}, "claim_id": c.ID})
	if e = tx.QueryRow(ctx, `INSERT INTO memory_profile(memory_type,scope_type,scope_id,key,value_json,summary,source_type,source_id,provenance,confidence,status,effective_from,supersedes_id,confirmation_state,effective_status,edited_by,edited_at) VALUES('profile',$1,$2,$3,$4,$5,'catalog',$6,$7,$8,'active',clock_timestamp(),$9,'confirmed','active',$10,clock_timestamp()) RETURNING id`, c.ScopeType, c.ScopeID, c.Key, c.Value, c.Summary, c.SourceID, provenance, c.Confidence, previous, own).Scan(&profileID); e != nil {
		return Claim{}, e
	}
	if _, e = tx.Exec(ctx, `UPDATE memory_claims SET status='confirmed',profile_id=$2,supersedes_id=$3,accepted_by=$4,accepted_at=clock_timestamp() WHERE claim_id=$1`, c.ID, profileID, active, own); e != nil {
		return Claim{}, e
	}
	if _, e = tx.Exec(ctx, `UPDATE memory_fact_heads SET active_claim_id=$5,revision=revision+1 WHERE owner_id=$1 AND scope_type=$2 AND scope_id=$3 AND fact_key=$4`, own, c.ScopeType, c.ScopeID, c.Key, c.ID); e != nil {
		return Claim{}, e
	}
	c.Status = "confirmed"
	c.SupersedesID = active
	return c, tx.Commit(ctx)
}
func getClaim(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string, lock bool) (Claim, error) {
	var c Claim
	clause := ""
	if lock {
		clause = " FOR UPDATE"
	}
	e := q.QueryRow(ctx, `SELECT claim_id,owner_id,scope_type,scope_id,fact_key,source_id,source_revision,value_json,summary,status,extraction_confidence,supersedes_id,accepted_by,event_time FROM memory_claims WHERE claim_id=$1`+clause, id).Scan(&c.ID, &c.OwnerID, &c.ScopeType, &c.ScopeID, &c.Key, &c.SourceID, &c.SourceRevision, &c.Value, &c.Summary, &c.Status, &c.Confidence, &c.SupersedesID, &c.AcceptedBy, &c.EventTime)
	return c, e
}
func (s *Store) Claims(ctx context.Context) ([]Claim, error) {
	own, e := owner(ctx)
	if e != nil {
		return nil, e
	}
	if e = s.CheckRestore(ctx); e != nil {
		return nil, e
	}
	rows, e := s.pool.Query(ctx, `SELECT claim_id FROM memory_claims WHERE owner_id=$1 ORDER BY created_at,claim_id LIMIT 2000`, own)
	if e != nil {
		return nil, e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return nil, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	var result []Claim
	for _, id := range ids {
		c, e := getClaim(ctx, s.pool, id, false)
		if e != nil {
			return nil, e
		}
		result = append(result, c)
	}
	return result, nil
}

// GrantSource is a private owner control path. Task versions fence previously
// granted access after amendment; no model argument can mint this record.
func (s *Store) GrantSource(ctx context.Context, taskID, sourceID, purpose string, revision int64) error {
	own, e := owner(ctx)
	if e != nil {
		return e
	}
	if !bounded(purpose, 512) {
		return ErrInvalid
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var current int64
	var status string
	if tx.QueryRow(ctx, `SELECT revision,status FROM tasks WHERE task_id=$1 AND owner_id=$2 FOR SHARE`, taskID, own).Scan(&current, &status) != nil || revision != current || status != "active" {
		return ErrStale
	}
	var now time.Time
	_ = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
	if _, e = actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, own, nil, now); e != nil {
		return e
	}
	src, e := getSourceTx(ctx, tx, sourceID, false)
	if e != nil || src.OwnerID != own || src.Status != "active" {
		return ErrDenied
	}
	_, e = tx.Exec(ctx, `INSERT INTO memory_scope_grants(task_id,source_id,task_revision,access_revision,purpose,owner_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(task_id,source_id) DO UPDATE SET task_revision=EXCLUDED.task_revision,access_revision=EXCLUDED.access_revision,purpose=EXCLUDED.purpose`, taskID, sourceID, current, src.AccessRevision, purpose, own)
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}
