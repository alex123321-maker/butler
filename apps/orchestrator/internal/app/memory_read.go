package app

import (
	"context"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/memory/reader"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MemoryReads is the owning service boundary. The permanent Broker dispatch
// supplies identity; model arguments can select only bounded exact DATA reads.
type MemoryReads struct {
	pool    *pgxpool.Pool
	sources *catalog.Store
	leases  *session.PostgresLeaseManager
	inputs  *durableinput.Store
	hashes  map[string]string
}

func NewMemoryReads(pool *pgxpool.Pool, sources *catalog.Store) (*MemoryReads, error) {
	if pool == nil || sources == nil || sources.Pool() != pool {
		return nil, readcontrol.ErrDenied
	}
	s := &MemoryReads{pool: pool, sources: sources, leases: session.NewPostgresLeaseManager(pool), inputs: durableinput.NewStore(pool), hashes: map[string]string{}}
	for _, c := range readcontrol.Contracts() {
		h, e := actionauth.ContractHash(c)
		if e != nil {
			return nil, e
		}
		s.hashes[c.ToolName] = h
	}
	return s, nil
}
func (s *MemoryReads) CheckReady(ctx context.Context) error {
	if s == nil {
		return readcontrol.ErrDenied
	}
	return s.sources.CheckRestore(ctx)
}
func (s *MemoryReads) ReadDispatched(ctx context.Context, e readcontrol.Envelope) (readcontrol.Result, error) {
	a, err := readcontrol.DecodeArgs(e.Tool, e.ArgsJSON)
	if s == nil || err != nil || e.RunID == "" || e.ToolCallID == "" || e.EffectID == "" || e.ContractHash != s.hashes[e.Tool] {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	if _, owner := owneridentity.FromContext(ctx); owner {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	envelope := delegation.AdmissionEnvelope{RunID: e.RunID, ToolCallID: e.ToolCallID, EffectID: e.EffectID, ArgsJSON: e.ArgsJSON, ContractHash: e.ContractHash}
	read := func(ctx context.Context, q admissionQuery, lock bool) (actionauth.Grant, error) {
		return readAdmissionDispatchEffect(ctx, q, envelope, e.Tool, s.hashes[e.Tool], actionauth.EffectRead, lock)
	}
	grant, err := read(ctx, s.pool, false)
	if err != nil {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	var key string
	var expiry time.Time
	if s.pool.QueryRow(ctx, `SELECT r.session_key,o.expires_at FROM runs r JOIN session_ownership o USING(session_key) WHERE r.run_id=$1 AND o.run_id=r.run_id AND o.lease_id=$2 AND o.fence_epoch=$3 AND o.expires_at>clock_timestamp()`, e.RunID, grant.LeaseID, grant.LeaseEpoch).Scan(&key, &expiry) != nil {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	var claim durableinput.Record
	if s.pool.QueryRow(ctx, `SELECT input_id,claim_id FROM accepted_inputs WHERE run_id=$1 AND session_key=$2 AND owner_id=$3 AND task_id=$4 AND status='running' AND checkpoint='tool_dispatching' AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp()`, e.RunID, key, grant.Binding.OwnerID, grant.Binding.TaskID).Scan(&claim.InputID, &claim.ClaimID) != nil || grant.InputClaimHash == "" || actionauth.InputClaimHash(claim.InputID, claim.ClaimID) != grant.InputClaimHash {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	if grant.ExpiresAt.Before(expiry) {
		expiry = grant.ExpiresAt
	}
	deadline, _ := ctx.Deadline()
	base, stop := context.WithDeadline(context.Background(), deadline)
	forward := context.AfterFunc(ctx, stop)
	defer stop()
	defer forward()
	token := ownership.Token{SessionKey: key, RunID: e.RunID, LeaseID: grant.LeaseID, FenceEpoch: grant.LeaseEpoch}
	owned := ownership.WithToken(durableinput.WithClaim(base, claim), s.leases.Authority(), token)
	binding := resources.Binding{OwnerID: grant.Binding.OwnerID, SessionKey: key, RunID: e.RunID, TaskID: grant.Binding.TaskID, TaskRevision: grant.Binding.TaskRevision, ConstraintsRevision: grant.Binding.ConstraintsRevision, LeaseEpoch: grant.LeaseEpoch, Deadline: expiry}
	tx, err := s.pool.Begin(owned)
	if err != nil {
		return readcontrol.Empty("backend_error"), err
	}
	defer tx.Rollback(context.Background())
	check := func() error {
		if (resources.WorkerGuard{}).CheckTx(owned, tx, binding) != nil || s.inputs.CheckDispatchClaimTx(owned, tx, e.RunID, e.ToolCallID, e.EffectID, grant.ID) != nil {
			return readcontrol.ErrDenied
		}
		current, err := read(owned, tx, true)
		if err != nil || current.ID != grant.ID || current.InputClaimHash != grant.InputClaimHash || current.Binding.ActionHash != grant.Binding.ActionHash {
			return readcontrol.ErrDenied
		}
		return s.sources.CheckRunSourcesTx(owned, tx, e.RunID)
	}
	if check() != nil {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	result, err := (reader.Reader{Sources: s.sources}).ReadTx(owned, tx, e.Tool, a)
	if err != nil {
		return readcontrol.Empty("backend_error"), err
	}
	// Repeat after all source/fragment lock waits; a result cannot outlive its
	// exact task, input, lease, source ancestor or deletion watermark.
	if check() != nil {
		return readcontrol.Empty("denied"), readcontrol.ErrDenied
	}
	if err = tx.Commit(owned); err != nil {
		return readcontrol.Empty("backend_error"), err
	}
	return result, nil
}
