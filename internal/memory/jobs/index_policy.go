package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

// IndexPolicy is an explicit owner-user scope permission, not a run lease.
// Space and resource limits are immutable; enable changes fence existing jobs.
type IndexPolicy struct {
	ID              string          `json:"policy_id"`
	OwnerID         string          `json:"-"`
	SpaceID         string          `json:"space_id"`
	Enabled         bool            `json:"enabled"`
	Revision        int64           `json:"revision"`
	MaxChunks       int             `json:"max_chunks"`
	MaxJobsPerDay   int             `json:"max_jobs_per_day"`
	Allowance       budget.Units    `json:"allowance"`
	ExpiresAt       time.Time       `json:"expires_at"`
	Proof           json.RawMessage `json:"quality_proof"`
	LastFingerprint string          `json:"-"`
	LastJobID       string          `json:"-"`
}

const indexPolicyColumns = `policy_id,owner_id,space_id,enabled,revision,max_chunks,max_jobs_per_day,allowance,expires_at,quality_proof,last_fingerprint,last_job_id`

func scanIndexPolicy(row pgx.Row) (IndexPolicy, error) {
	var p IndexPolicy
	var raw []byte
	err := row.Scan(&p.ID, &p.OwnerID, &p.SpaceID, &p.Enabled, &p.Revision, &p.MaxChunks, &p.MaxJobsPerDay, &raw, &p.ExpiresAt, &p.Proof, &p.LastFingerprint, &p.LastJobID)
	if err == nil {
		err = json.Unmarshal(raw, &p.Allowance)
	}
	return p, err
}
func (s *Store) CreateIndexPolicy(ctx context.Context, p IndexPolicy) (IndexPolicy, error) {
	a, ok := owneridentity.FromContext(ctx)
	limits := p.Allowance
	limits.ModelTurns = min(limits.ModelTurns, 128)
	if !ok || len(p.ID) < 8 || len(p.ID) > 128 || len(p.SpaceID) > 128 || p.SpaceID == "" || p.MaxChunks < 1 || p.MaxChunks > 10000 || p.MaxJobsPerDay < 1 || p.MaxJobsPerDay > 96 || p.Allowance.ModelTurns > 10000 || !validAllowance(limits) || p.Allowance.CostMicros != 0 || !json.Valid(p.Proof) {
		return p, ErrClaim
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return p, e
	}
	defer tx.Rollback(ctx)
	if e = s.catalog.CheckRestoreTx(ctx, tx); e != nil {
		return p, e
	}
	var now time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
		return p, e
	}
	if !p.ExpiresAt.After(now) || p.ExpiresAt.After(now.Add(366*24*time.Hour)) {
		return p, ErrClaim
	}
	p.OwnerID = a.OwnerID
	p.Enabled = true
	p.Revision = 1
	_, e = tx.Exec(ctx, `INSERT INTO memory_index_policies(policy_id,owner_id,space_id,max_chunks,max_jobs_per_day,allowance,expires_at,quality_proof) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(policy_id) DO NOTHING`, p.ID, p.OwnerID, p.SpaceID, p.MaxChunks, p.MaxJobsPerDay, jsonBytes(p.Allowance), p.ExpiresAt, p.Proof)
	if e != nil {
		return p, e
	}
	old, e := scanIndexPolicy(tx.QueryRow(ctx, `SELECT `+indexPolicyColumns+` FROM memory_index_policies WHERE policy_id=$1`, p.ID))
	var oldProof, newProof any
	json.Unmarshal(old.Proof, &oldProof)
	json.Unmarshal(p.Proof, &newProof)
	if e != nil || old.OwnerID != p.OwnerID || old.SpaceID != p.SpaceID || old.MaxChunks != p.MaxChunks || old.MaxJobsPerDay != p.MaxJobsPerDay || old.Allowance != p.Allowance || !old.ExpiresAt.Equal(p.ExpiresAt.UTC().Truncate(time.Microsecond)) || string(jsonBytes(oldProof)) != string(jsonBytes(newProof)) {
		return p, ErrClaim
	}
	return old, tx.Commit(ctx)
}
func (s *Store) ControlIndexPolicy(ctx context.Context, id string, revision int64, enabled bool) error {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return ErrClaim
	}
	tag, e := s.pool.Exec(ctx, `UPDATE memory_index_policies SET enabled=$4,revision=revision+1,next_at=clock_timestamp() WHERE policy_id=$1 AND owner_id=$2 AND revision=$3`, id, a.OwnerID, revision, enabled)
	if e == nil && tag.RowsAffected() != 1 {
		return ErrClaim
	}
	return e
}
func (s *Store) IndexPolicies(ctx context.Context) ([]IndexPolicy, error) {
	rows, e := s.pool.Query(ctx, `SELECT `+indexPolicyColumns+` FROM memory_index_policies WHERE enabled AND expires_at>clock_timestamp() AND next_at<=clock_timestamp() ORDER BY policy_id LIMIT 32`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []IndexPolicy
	for rows.Next() {
		p, e := scanIndexPolicy(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) OwnerIndexPolicies(ctx context.Context) ([]IndexPolicy, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return nil, ErrClaim
	}
	if e := s.catalog.CheckRestore(ctx); e != nil {
		return nil, e
	}
	rows, e := s.pool.Query(ctx, `SELECT `+indexPolicyColumns+` FROM memory_index_policies WHERE owner_id=$1`, a.OwnerID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []IndexPolicy{}
	for rows.Next() {
		p, e := scanIndexPolicy(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type indexPolicyKey struct{}
type indexPolicyAdmission struct {
	Policy      IndexPolicy
	Fingerprint string
}

func (s *Store) IndexPolicyContext(ctx context.Context, p IndexPolicy) context.Context {
	return catalog.WithAccessChecker(owneridentity.WithActor(ctx, owneridentity.Actor{}), &indexPolicyChecker{s: s, p: p})
}
func (s *Store) EnqueueIndexPolicy(ctx context.Context, p IndexPolicy, fingerprint string, r Request, publish func(context.Context, pgx.Tx, Job) error) (Job, error) {
	if r.Kind != "index" || r.Purpose != "memory_index_maintenance" || r.TaskID != "" || r.OriginalRunID != "" || r.Allowance != p.Allowance || len(fingerprint) != 64 {
		return Job{}, ErrClaim
	}
	ctx = context.WithValue(s.IndexPolicyContext(ctx, p), indexPolicyKey{}, indexPolicyAdmission{p, fingerprint})
	return s.enqueue(ctx, r, func(ctx context.Context, tx pgx.Tx, j Job) error {
		_, e := tx.Exec(ctx, `INSERT INTO memory_index_policy_jobs(job_id,policy_id,policy_revision,expected_previous) SELECT $1,$2,$3,COALESCE((SELECT generation_id FROM semantic_index_active WHERE scope_type='user' AND scope_id=$4),'')`, j.ID, p.ID, p.Revision, p.OwnerID)
		if e != nil {
			return e
		}
		if publish != nil {
			return publish(ctx, tx, j)
		}
		return nil
	})
}
func (s *Store) checkIndexPolicyTx(ctx context.Context, tx pgx.Tx, p IndexPolicy) error {
	var current bool
	e := tx.QueryRow(ctx, `SELECT enabled AND revision=$2 AND owner_id=$3 AND space_id=$4 AND expires_at>clock_timestamp() AND max_chunks=$5 AND max_jobs_per_day=$6 AND allowance=$7::jsonb AND expires_at=$8 FROM memory_index_policies WHERE policy_id=$1 FOR SHARE`, p.ID, p.Revision, p.OwnerID, p.SpaceID, p.MaxChunks, p.MaxJobsPerDay, jsonBytes(p.Allowance), p.ExpiresAt).Scan(&current)
	if e != nil {
		return e
	}
	if !current {
		return ErrClaim
	}
	return nil
}
func (s *Store) checkIndexJobPolicyTx(ctx context.Context, tx pgx.Tx, j Job) error {
	if j.Kind != "index" {
		return nil
	}
	var valid bool
	e := tx.QueryRow(ctx, `SELECT p.owner_id=$2 AND p.enabled AND p.revision=l.policy_revision AND p.expires_at>clock_timestamp() AND p.expires_at>=$3 AND p.allowance=$4::jsonb FROM memory_index_policy_jobs l JOIN memory_index_policies p USING(policy_id) WHERE l.job_id=$1 FOR SHARE OF p`, j.ID, j.OwnerID, j.Deadline, jsonBytes(j.Allowance)).Scan(&valid)
	if e == pgx.ErrNoRows {
		return nil
	}
	if e != nil {
		return e
	}
	if !valid {
		return ErrClaim
	}
	return nil
}

type indexClaimKey struct{}

func IndexClaimID(ctx context.Context) string {
	id, _ := ctx.Value(indexClaimKey{}).(string)
	return id
}

type indexPolicyChecker struct {
	s *Store
	p IndexPolicy
}

func (c *indexPolicyChecker) PinnedMemorySources() []catalog.SourceRef { return nil }
func (c *indexPolicyChecker) CheckMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []catalog.SourceRef, _ string) (string, error) {
	if e := c.s.checkIndexPolicyTx(ctx, tx, c.p); e != nil {
		return "", e
	}
	for _, r := range refs {
		var ok bool
		e := tx.QueryRow(ctx, `SELECT owner_id=$2 AND scope_type='user' AND scope_id=$2 FROM memory_sources WHERE source_id=$1`, r.ID, c.p.OwnerID).Scan(&ok)
		if e != nil {
			return "", e
		}
		if !ok {
			return "", ErrClaim
		}
	}
	return c.p.OwnerID, nil
}
func (c *indexPolicyChecker) FinalizeMemoryAccessTx(ctx context.Context, tx pgx.Tx, _ []catalog.SourceRef, _ string) error {
	return c.s.checkIndexPolicyTx(ctx, tx, c.p)
}
