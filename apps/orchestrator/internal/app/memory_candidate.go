package app

import (
	"context"
	"encoding/json"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/candidatecontrol"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MemoryCandidates is the owning publication boundary. Model fields are DATA;
// actual dispatch, source scope, worker/input claim and dedup identity are private.
type MemoryCandidates struct {
	pool    *pgxpool.Pool
	sources *catalog.Store
	leases  *session.PostgresLeaseManager
	inputs  *durableinput.Store
	hash    string
}

func NewMemoryCandidates(pool *pgxpool.Pool, sources *catalog.Store) (*MemoryCandidates, error) {
	if pool == nil || sources == nil || sources.Pool() != pool {
		return nil, candidatecontrol.ErrDenied
	}
	hash, err := actionauth.ContractHash(candidatecontrol.Contract())
	if err != nil {
		return nil, err
	}
	return &MemoryCandidates{pool: pool, sources: sources, leases: session.NewPostgresLeaseManager(pool), inputs: durableinput.NewStore(pool), hash: hash}, nil
}
func (s *MemoryCandidates) CheckReady(ctx context.Context) error {
	if s == nil || s.sources.CheckRestore(ctx) != nil {
		return candidatecontrol.ErrDenied
	}
	_, err := s.pool.Exec(ctx, `SELECT d.owner_id,d.source_id,d.source_revision,d.dedup_key,d.request_hash,d.claim_id,c.claim_id,c.status,i.claim_id,i.claim_expires_at,i.expires_at FROM memory_extraction_dedup d,memory_claims c,accepted_inputs i WHERE FALSE`)
	return err
}
func (s *MemoryCandidates) readDispatch(ctx context.Context, q admissionQuery, e candidatecontrol.Envelope, lock bool) (actionauth.Grant, error) {
	return readAdmissionDispatch(ctx, q, delegation.AdmissionEnvelope{RunID: e.RunID, ToolCallID: e.ToolCallID, EffectID: e.EffectID, ArgsJSON: e.ArgsJSON, ContractHash: e.ContractHash}, candidatecontrol.Tool, s.hash, lock)
}
func safeCandidate(a candidatecontrol.Args) bool {
	if sanitize.Text(a.Summary) != a.Summary || sanitize.Text(a.Key) != a.Key {
		return false
	}
	// Compare without rewriting model intent. Sensitive keys and recognized raw
	// credential patterns are denied, rather than persisted after redaction.
	var value any
	if json.Unmarshal(a.Value, &value) != nil {
		return false
	}
	raw, err := json.Marshal(map[string]any{a.Key: value})
	return err == nil && sanitize.JSON(string(raw)) == string(raw)
}
func (s *MemoryCandidates) ProposeDispatched(ctx context.Context, e candidatecontrol.Envelope) (candidatecontrol.Receipt, error) {
	a, err := candidatecontrol.DecodeArgs(e.ArgsJSON)
	if s == nil || err != nil || !safeCandidate(a) || e.ContractHash != s.hash || e.RunID == "" || e.ToolCallID == "" || e.EffectID == "" || len(e.RunID) > 512 || len(e.ToolCallID) > 512 || len(e.EffectID) > 512 {
		return candidatecontrol.Receipt{}, candidatecontrol.ErrDenied
	}
	if _, owner := owneridentity.FromContext(ctx); owner {
		return candidatecontrol.Receipt{}, candidatecontrol.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	grant, err := s.readDispatch(ctx, s.pool, e, false)
	if err != nil {
		return candidatecontrol.Receipt{}, candidatecontrol.ErrDenied
	}
	var key string
	var expiry time.Time
	if s.pool.QueryRow(ctx, `SELECT r.session_key,o.expires_at FROM runs r JOIN session_ownership o USING(session_key) WHERE r.run_id=$1 AND o.run_id=r.run_id AND o.lease_id=$2 AND o.fence_epoch=$3 AND o.expires_at>clock_timestamp()`, e.RunID, grant.LeaseID, grant.LeaseEpoch).Scan(&key, &expiry) != nil {
		return candidatecontrol.Receipt{}, candidatecontrol.ErrDenied
	}
	var claim durableinput.Record
	if s.pool.QueryRow(ctx, `SELECT input_id,claim_id FROM accepted_inputs WHERE run_id=$1 AND session_key=$2 AND owner_id=$3 AND task_id=$4 AND status='running' AND checkpoint='tool_dispatching' AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp()`, e.RunID, key, grant.Binding.OwnerID, grant.Binding.TaskID).Scan(&claim.InputID, &claim.ClaimID) != nil || grant.InputClaimHash == "" || actionauth.InputClaimHash(claim.InputID, claim.ClaimID) != grant.InputClaimHash {
		return candidatecontrol.Receipt{}, candidatecontrol.ErrDenied
	}
	if grant.ExpiresAt.Before(expiry) {
		expiry = grant.ExpiresAt
	}
	token := ownership.Token{SessionKey: key, RunID: e.RunID, LeaseID: grant.LeaseID, FenceEpoch: grant.LeaseEpoch}
	// Reconstruct authority without inheriting a caller's auxiliary checker or
	// other ownership values. Keep the private RPC deadline and cancellation.
	deadline, _ := ctx.Deadline()
	base, stop := context.WithDeadline(context.Background(), deadline)
	forward := context.AfterFunc(ctx, stop)
	defer stop()
	defer forward()
	owned := ownership.WithToken(durableinput.WithClaim(base, claim), s.leases.Authority(), token)
	b := resources.Binding{OwnerID: grant.Binding.OwnerID, SessionKey: key, RunID: e.RunID, TaskID: grant.Binding.TaskID, TaskRevision: grant.Binding.TaskRevision, ConstraintsRevision: grant.Binding.ConstraintsRevision, LeaseEpoch: grant.LeaseEpoch, Deadline: expiry}
	tx, err := s.pool.Begin(owned)
	if err != nil {
		return candidatecontrol.Receipt{}, err
	}
	defer tx.Rollback(context.Background())
	check := func() error {
		if (resources.WorkerGuard{}).CheckTx(owned, tx, b) != nil || s.inputs.CheckDispatchClaimTx(owned, tx, e.RunID, e.ToolCallID, e.EffectID, grant.ID) != nil {
			return candidatecontrol.ErrDenied
		}
		current, err := s.readDispatch(owned, tx, e, true)
		if err != nil || current.ID != grant.ID || current.InputClaimHash != grant.InputClaimHash || current.Binding.ActionHash != grant.Binding.ActionHash {
			return candidatecontrol.ErrDenied
		}
		return s.sources.CheckRunSourcesTx(owned, tx, e.RunID)
	}
	if err = check(); err != nil {
		return candidatecontrol.Receipt{}, candidatecontrol.ErrDenied
	}
	// ProposeTx may pin accessible sources, but this control may only use an
	// exact source already pinned by the actual run. A pin is privacy/evidence
	// metadata; it does not prove that the model read every original ancestor.
	var pinned bool
	if tx.QueryRow(owned, `SELECT source_revision=$3 FROM memory_run_sources WHERE run_id=$1 AND source_id=$2`, e.RunID, a.SourceID, a.SourceRevision).Scan(&pinned) != nil || !pinned {
		return candidatecontrol.Receipt{}, candidatecontrol.ErrDenied
	}
	c, err := s.sources.ProposeTx(owned, tx, catalog.ClaimRequest{SourceID: a.SourceID, SourceRevision: a.SourceRevision, Key: a.Key, Value: a.Value, Summary: a.Summary, Confidence: 0, DedupKey: e.EffectID})
	if err != nil || c.Status != "candidate" {
		return candidatecontrol.Receipt{}, candidatecontrol.ErrDenied
	}
	if err = check(); err != nil {
		return candidatecontrol.Receipt{}, candidatecontrol.ErrDenied
	}
	if err = tx.Commit(owned); err != nil {
		return candidatecontrol.Receipt{}, err
	}
	return candidatecontrol.Receipt{ClaimID: c.ID, Status: "candidate"}, nil
}
