// Package jobs owns bounded post-run inference independently of session leases.
package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrClaim = errors.New("auxiliary job claim or source authority unavailable")

type Job struct {
	PolicyID            string                `json:"policy_id,omitempty"`
	Contexts            []ModelContextPin     `json:"contexts,omitempty"`
	ID                  string                `json:"job_id"`
	OwnerID             string                `json:"owner_id"`
	Kind                string                `json:"kind"`
	Purpose             string                `json:"purpose"`
	TaskID              string                `json:"task_id"`
	TaskRevision        int64                 `json:"task_revision"`
	ConstraintsRevision int64                 `json:"constraints_revision"`
	OriginalRunID       string                `json:"original_run_id"`
	Authority           actionauth.Assignment `json:"authority"`
	Sources             []catalog.SourceRef   `json:"sources"`
	Fingerprint         string                `json:"source_fingerprint"`
	Allowance           budget.Units          `json:"allowance"`
	Deadline            time.Time             `json:"deadline"`
	Status              string                `json:"status"`
	ClaimID             string                `json:"claim_id"`
	ClaimEpoch          int64                 `json:"claim_epoch"`
	ClaimUntil          *time.Time            `json:"claim_until"`
	Attempts            int                   `json:"attempts"`
}
type Request struct {
	ID            string              `json:"job_id"`
	Kind          string              `json:"kind"`
	Purpose       string              `json:"purpose"`
	TaskID        string              `json:"task_id"`
	OriginalRunID string              `json:"original_run_id"`
	Sources       []catalog.SourceRef `json:"sources"`
	Allowance     budget.Units        `json:"allowance"`
	Deadline      time.Time           `json:"deadline"`
}
type Store struct {
	pool    *pgxpool.Pool
	catalog *catalog.Store
	budgets *budget.Controller
	target  *LocalTarget
}

// LocalTarget is operator-configured service identity, independent of price.
// The transport adapter must use exactly Endpoint. The caller cannot infer
// locality from a zero/no-overage quote or provide arbitrary remote URLs.
type LocalTarget struct{ Endpoint, Identity string }

func New(pool *pgxpool.Pool, c *catalog.Store, b *budget.Controller, t *LocalTarget) *Store {
	return &Store{pool, c, b, t}
}
func canonical(refs []catalog.SourceRef) ([]catalog.SourceRef, string, error) {
	if len(refs) < 1 || len(refs) > 2000 {
		return nil, "", ErrClaim
	}
	refs = append([]catalog.SourceRef(nil), refs...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	for i, r := range refs {
		if r.ID == "" || r.Revision < 1 || r.AccessRevision < 1 || len(r.Hash) != 64 || i > 0 && r.ID == refs[i-1].ID {
			return nil, "", ErrClaim
		}
	}
	b, _ := json.Marshal(refs)
	sum := sha256.Sum256(b)
	return refs, hex.EncodeToString(sum[:]), nil
}
func (s *Store) Enqueue(ctx context.Context, r Request) (Job, error) {
	return s.enqueue(ctx, r, nil)
}

// EnqueueWithPublication atomically persists trusted owning-module metadata.
// The callback uses this transaction; it cannot borrow another pool connection.
func (s *Store) EnqueueWithPublication(ctx context.Context, r Request, publish func(context.Context, pgx.Tx, Job) error) (Job, error) {
	return s.enqueue(ctx, r, publish)
}
func (s *Store) enqueue(ctx context.Context, r Request, publish func(context.Context, pgx.Tx, Job) error) (Job, error) {
	r.Deadline = r.Deadline.UTC().Truncate(time.Microsecond)
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return Job{}, ErrClaim
	}
	if len(r.ID) < 8 || len(r.ID) > 512 || len(r.Purpose) < 1 || len(r.Purpose) > 512 {
		return Job{}, ErrClaim
	}
	switch r.Kind {
	case "index", "extract", "summary", "ingest":
	default:
		return Job{}, ErrClaim
	}
	if r.Allowance.ModelTurns < 1 || r.Allowance.ModelTurns > 10000 || r.Allowance.ActiveMillis < 1 || r.Allowance.ActiveMillis > 86400000 || r.Allowance.ProviderInputBytes < 1 || r.Allowance.ProviderInputBytes > 1<<32 || r.Allowance.Tokens < 1 || r.Allowance.Tokens > 1<<32 || r.Allowance.CostMicros != 0 || r.Allowance.EgressBytes != 0 || r.Allowance.ExternalCalls != 0 || r.Allowance.Children != 0 {
		return Job{}, ErrClaim
	}
	refs, sum, e := canonical(r.Sources)
	if e != nil {
		return Job{}, e
	}
	if e = s.catalog.CheckRestore(ctx); e != nil {
		return Job{}, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Job{}, e
	}
	defer tx.Rollback(ctx)
	var now time.Time
	_ = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
	if !r.Deadline.After(now) || r.Deadline.After(now.Add(7*24*time.Hour)) {
		return Job{}, ErrClaim
	}
	j := Job{ID: r.ID, OwnerID: a.OwnerID, Kind: r.Kind, Purpose: r.Purpose, TaskID: r.TaskID, OriginalRunID: r.OriginalRunID, Sources: refs, Fingerprint: sum, Allowance: r.Allowance, Deadline: r.Deadline, Status: "pending"}
	if j.TaskID != "" {
		var status, own string
		if tx.QueryRow(ctx, `SELECT owner_id,status,revision,constraints_revision FROM tasks WHERE task_id=$1 FOR SHARE`, j.TaskID).Scan(&own, &status, &j.TaskRevision, &j.ConstraintsRevision) != nil || own != a.OwnerID || status != "active" {
			return Job{}, ErrClaim
		}
		j.Authority, e = actionauth.CheckTaskAuthorityTx(ctx, tx, j.TaskID, own, nil, now)
		if e != nil {
			return Job{}, e
		}
		if j.Authority.TaskID != "" && j.Authority.Purpose != r.Purpose {
			return Job{}, ErrClaim
		}
	}
	if j.OriginalRunID != "" {
		var taskID, own string
		if tx.QueryRow(ctx, `SELECT COALESCE(r.task_id,''),s.user_id FROM runs r JOIN sessions s USING(session_key) WHERE r.run_id=$1`, j.OriginalRunID).Scan(&taskID, &own) != nil || taskID != j.TaskID || own != a.OwnerID {
			return Job{}, ErrClaim
		}
	}
	refs, e = s.catalog.CanonicalSourcesTx(ctx, tx, refs)
	if e != nil {
		return Job{}, e
	}
	current, e := s.catalog.AccessTx(ctx, tx, refs, "memory_job_create")
	if e != nil {
		return Job{}, e
	}
	refs = make([]catalog.SourceRef, 0, len(current))
	for _, source := range current {
		refs = append(refs, catalog.SourceRef{ID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash})
	}
	refs, sum, e = canonical(refs)
	if e != nil {
		return Job{}, e
	}
	j.Sources, j.Fingerprint = refs, sum
	if _, e = s.catalog.AccessTx(ctx, tx, refs, "memory_job_create"); e != nil {
		return Job{}, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO memory_aux_jobs(job_id,owner_id,kind,purpose,task_id,task_revision,constraints_revision,original_run_id,authority,sources,source_fingerprint,allowance,deadline) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT(job_id) DO NOTHING`, j.ID, j.OwnerID, j.Kind, j.Purpose, j.TaskID, j.TaskRevision, j.ConstraintsRevision, j.OriginalRunID, jsonBytes(j.Authority), jsonBytes(j.Sources), j.Fingerprint, jsonBytes(j.Allowance), j.Deadline)
	if e != nil {
		return Job{}, e
	}
	old, e := load(ctx, tx, j.ID, false)
	if e != nil {
		return Job{}, e
	}
	if old.OwnerID != j.OwnerID || old.Fingerprint != j.Fingerprint || old.Kind != j.Kind || old.Purpose != j.Purpose || old.TaskID != j.TaskID || old.TaskRevision != j.TaskRevision || old.ConstraintsRevision != j.ConstraintsRevision || !old.Deadline.Equal(j.Deadline) || old.Allowance != j.Allowance || old.OriginalRunID != j.OriginalRunID {
		return Job{}, ErrClaim
	}
	if publish != nil {
		if e = publish(ctx, tx, old); e != nil {
			return Job{}, e
		}
	}
	return old, tx.Commit(ctx)
}
func jsonBytes(v any) []byte { b, _ := json.Marshal(v); return b }
func load(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id string, lock bool) (Job, error) {
	var j Job
	var refs, authority, allowance, contexts []byte
	clause := ""
	if lock {
		clause = " FOR SHARE"
	}
	e := q.QueryRow(ctx, `SELECT job_id,owner_id,kind,purpose,task_id,task_revision,constraints_revision,original_run_id,authority,sources,source_fingerprint,allowance,deadline,status,claim_id,claim_epoch,claim_until,attempts FROM memory_aux_jobs WHERE job_id=$1`+clause, id).Scan(&j.ID, &j.OwnerID, &j.Kind, &j.Purpose, &j.TaskID, &j.TaskRevision, &j.ConstraintsRevision, &j.OriginalRunID, &authority, &refs, &j.Fingerprint, &allowance, &j.Deadline, &j.Status, &j.ClaimID, &j.ClaimEpoch, &j.ClaimUntil, &j.Attempts)
	if e == nil {
		if json.Unmarshal(refs, &j.Sources) != nil || json.Unmarshal(authority, &j.Authority) != nil || json.Unmarshal(allowance, &j.Allowance) != nil {
			return j, ErrClaim
		}
		e = q.QueryRow(ctx, `SELECT COALESCE(policy_id,''),context_manifest FROM memory_aux_jobs WHERE job_id=$1`, id).Scan(&j.PolicyID, &contexts)
		if e == nil && json.Unmarshal(contexts, &j.Contexts) != nil {
			return j, ErrClaim
		}
		if len(j.Contexts) == 0 {
			j.Contexts = nil
		}
	}
	return j, e
}

// Claim is a bounded durable queue claim, not execution permission. Every
// actual inference/publication subsequently checks current source authority.
func (s *Store) Claim(ctx context.Context) (Job, bool, error) {
	return s.ClaimKind(ctx, "")
}
func (s *Store) ClaimKind(ctx context.Context, kind string) (Job, bool, error) {
	return s.claimSelected(ctx, kind, "")
}
func (s *Store) claimSelected(ctx context.Context, kind, purpose string) (Job, bool, error) {
	return s.claimSelectedOwner(ctx, kind, purpose, "", "")
}
func (s *Store) claimSelectedOwner(ctx context.Context, kind, purpose, jobID, ownerID string) (Job, bool, error) {
	switch kind {
	case "", "index", "extract", "summary", "ingest", "model":
	default:
		return Job{}, false, ErrClaim
	}
	if e := s.catalog.CheckRestore(ctx); e != nil {
		return Job{}, false, e
	}
	id, e := budget.NewOperationID()
	if e != nil {
		return Job{}, false, e
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return Job{}, false, e
	}
	defer tx.Rollback(ctx)
	// Expired pending jobs must become terminal even while workers are paused.
	if _, e = tx.Exec(ctx, `UPDATE memory_aux_jobs SET status='failed',error_code='deadline',updated_at=clock_timestamp() WHERE status='pending' AND (deadline<=clock_timestamp() OR attempts>=max_attempts)`); e != nil {
		return Job{}, false, e
	}
	_, e = tx.Exec(ctx, `UPDATE memory_aux_jobs SET status=CASE WHEN attempts>=max_attempts OR deadline<=clock_timestamp() THEN 'failed' ELSE 'pending' END,claim_id='',claim_until=NULL,error_code='claim_expired',updated_at=clock_timestamp() WHERE status='running' AND claim_until<=clock_timestamp()`)
	if e != nil {
		return Job{}, false, e
	}
	var jid string
	e = tx.QueryRow(ctx, `UPDATE memory_aux_jobs SET status='running',claim_id=$1,claim_epoch=claim_epoch+1,claim_until=LEAST(deadline,clock_timestamp()+CASE WHEN kind='model' THEN INTERVAL '6 minutes' ELSE INTERVAL '2 minutes' END),attempts=attempts+1,updated_at=clock_timestamp() WHERE job_id=(SELECT job_id FROM memory_aux_jobs WHERE status='pending' AND (policy_id IS NULL OR EXISTS(SELECT 1 FROM memory_model_policies p WHERE p.policy_id=memory_aux_jobs.policy_id AND p.state='active')) AND ($2='' OR kind=$2) AND ($3='' OR purpose=$3) AND ($4='' OR job_id=$4) AND ($5='' OR owner_id=$5) AND next_attempt_at<=clock_timestamp() AND deadline>clock_timestamp() AND attempts<max_attempts ORDER BY next_attempt_at,job_id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING job_id`, id, kind, purpose, jobID, ownerID).Scan(&jid)
	if e == pgx.ErrNoRows {
		return Job{}, false, tx.Commit(ctx)
	}
	if e != nil {
		return Job{}, false, e
	}
	j, e := load(ctx, tx, jid, false)
	if e != nil {
		return Job{}, false, e
	}
	return j, true, tx.Commit(ctx)
}
func (s *Store) Finish(ctx context.Context, j Job, code string, retry bool) error {
	status := "done"
	if code != "" {
		status = "failed"
		if retry {
			status = "pending"
		}
	}
	tag, e := s.pool.Exec(ctx, `UPDATE memory_aux_jobs SET status=CASE WHEN $4='pending' AND (attempts>=max_attempts OR deadline<=clock_timestamp()) THEN 'failed' ELSE $4 END,claim_id='',claim_until=NULL,error_code=$5,next_attempt_at=clock_timestamp()+INTERVAL '10 seconds',updated_at=clock_timestamp() WHERE job_id=$1 AND claim_id=$2 AND claim_epoch=$3 AND status='running' AND claim_until>clock_timestamp()`, j.ID, j.ClaimID, j.ClaimEpoch, status, code)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrClaim
	}
	return nil
}

type claimChecker struct {
	s *Store
	j Job
}

func (s *Store) Context(ctx context.Context, j Job) context.Context {
	j = copyJob(j)
	return catalog.WithAccessChecker(owneridentity.WithActor(ctx, owneridentity.Actor{}), &claimChecker{s, j})
}

func copyJob(j Job) Job {
	j.Sources = append([]catalog.SourceRef(nil), j.Sources...)
	j.Contexts = append([]ModelContextPin(nil), j.Contexts...)
	if j.ClaimUntil != nil {
		until := *j.ClaimUntil
		j.ClaimUntil = &until
	}
	return j
}

// A private caller's Job is a pinned snapshot, never a policy input. Verify its
// complete manifest before using any field, then compare the live row without
// locking it ahead of source rows. Finalize locks and repeats this comparison.
func (c *claimChecker) checkSnapshot(ctx context.Context, tx pgx.Tx, lock bool) (Job, error) {
	j := c.j
	sum, e := modelFingerprint(j.Sources, j.Contexts)
	if e != nil || sum != j.Fingerprint {
		return Job{}, ErrClaim
	}
	if len(j.Contexts) > 0 {
		if j.PolicyID == "" || j.Kind != "model" || j.Purpose != ConsolidationHandler || j.TaskID != "" || j.OriginalRunID != "" {
			return Job{}, ErrClaim
		}
		// Domain/authority locks precede even a direct snapshot's job lock.
		if _, err := c.s.checkModelPolicyTx(ctx, tx, j); err != nil {
			return Job{}, err
		}
	}
	live, e := load(ctx, tx, j.ID, lock)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return Job{}, ErrClaim
		}
		return Job{}, e
	}
	if live.PolicyID != j.PolicyID {
		return Job{}, ErrClaim
	}
	liveSum, e := modelFingerprint(live.Sources, live.Contexts)
	var now time.Time
	if sqlErr := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); sqlErr != nil {
		return Job{}, sqlErr
	}
	if e != nil || liveSum != live.Fingerprint || live.Status != "running" || live.ClaimID != j.ClaimID || live.ClaimEpoch != j.ClaimEpoch || live.ClaimUntil == nil || j.ClaimUntil == nil || !live.ClaimUntil.Equal(*j.ClaimUntil) || !live.ClaimUntil.After(now) || !live.Deadline.After(now) || !live.Deadline.Equal(j.Deadline) || live.Fingerprint != j.Fingerprint || live.OwnerID != j.OwnerID || live.TaskID != j.TaskID || live.Authority != j.Authority || live.TaskRevision != j.TaskRevision || live.ConstraintsRevision != j.ConstraintsRevision || live.Purpose != j.Purpose || live.Kind != j.Kind || live.Allowance != j.Allowance || live.OriginalRunID != j.OriginalRunID || string(jsonBytes(live.Contexts)) != string(jsonBytes(j.Contexts)) {
		return Job{}, ErrClaim
	}
	return live, nil
}
func (c *claimChecker) PinnedMemorySources() []catalog.SourceRef {
	return append([]catalog.SourceRef(nil), c.j.Sources...)
}
func (c *claimChecker) CheckMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []catalog.SourceRef, purpose string) (string, error) {
	j := c.j
	if _, e := c.checkSnapshot(ctx, tx, false); e != nil {
		return "", e
	}
	if j.PolicyID != "" {
		if _, e := c.s.checkModelPolicyTx(ctx, tx, j); e != nil {
			return "", e
		}
	} else if j.TaskID != "" {
		var own, status string
		var revision, constraints int64
		if tx.QueryRow(ctx, `SELECT owner_id,status,revision,constraints_revision FROM tasks WHERE task_id=$1 FOR SHARE`, j.TaskID).Scan(&own, &status, &revision, &constraints) != nil || own != j.OwnerID || status != "active" || revision != j.TaskRevision || constraints != j.ConstraintsRevision {
			return "", ErrClaim
		}
		var now time.Time
		_ = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now)
		if _, e := actionauth.CheckTaskAuthorityTx(ctx, tx, j.TaskID, j.OwnerID, &j.Authority, now); e != nil {
			return "", e
		}
	}
	permitted := map[string]catalog.SourceRef{}
	for _, r := range j.Sources {
		permitted[r.ID] = r
	}
	for _, r := range refs {
		p, ok := permitted[r.ID]
		if !ok || r.Revision != 0 && r.Revision != p.Revision || r.Hash != "" && r.Hash != p.Hash || r.AccessRevision != 0 && r.AccessRevision != p.AccessRevision {
			return "", ErrClaim
		}
	}
	return j.OwnerID, nil
}
func (c *claimChecker) FinalizeMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []catalog.SourceRef, purpose string) error {
	// Policy/task rows were locked before the source graph. Repeat deadlines
	// after waits; this never obtains new authority or borrows a connection.
	if c.j.PolicyID != "" {
		if _, e := c.s.checkModelPolicyTx(ctx, tx, c.j); e != nil {
			return e
		}
	}
	live, e := c.checkSnapshot(ctx, tx, true)
	if e != nil {
		return e
	}
	// Read full pinned manifest again so caller cannot omit changed source hashes
	// from a publication subset or invent a job using a correct claim ID.
	for _, r := range live.Sources {
		var rev, access int64
		var sum, status string
		if e := tx.QueryRow(ctx, `SELECT s.head_revision,s.access_revision,s.status,r.content_hash FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision WHERE s.source_id=$1`, r.ID).Scan(&rev, &access, &status, &sum); e != nil {
			if errors.Is(e, pgx.ErrNoRows) {
				return ErrClaim
			}
			return e
		}
		if rev != r.Revision || access != r.AccessRevision || sum != r.Hash || status != "active" {
			return ErrClaim
		}
	}
	return nil
}
