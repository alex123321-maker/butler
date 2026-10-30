package resourceexec

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SourceGuard func(context.Context, pgx.Tx, string) error
type Store struct {
	pool        *pgxpool.Pool
	resources   *resources.Store
	budgets     *budget.Controller
	sourceGuard SourceGuard
	profile     Profile
	singleTab   SingleTabGuard
}

func New(pool *pgxpool.Pool, r *resources.Store, b *budget.Controller, source SourceGuard, p Profile) (*Store, error) {
	if pool == nil || r == nil || b == nil || source == nil || p.ID == "" || len(p.ID) > 128 || p.RuntimeTarget == "" || len(p.ContractHashes) == 0 {
		return nil, ErrDenied
	}
	copyHashes := map[string]string{}
	for name, h := range p.ContractHashes {
		if !strings.HasPrefix(name, "browser.") || len(h) != 64 || !isHex(h) {
			return nil, ErrDenied
		}
		copyHashes[name] = h
	}
	p.ContractHashes = copyHashes
	return &Store{pool: pool, resources: r, budgets: b, sourceGuard: source, profile: p}, nil
}

// PrepareContext is called by Broker only after exact registry/authorization
// validation, before Claim. The final same-TX hook validates its hashes again.
func (s *Store) PrepareContext(ctx context.Context, call *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract, g actionauth.Grant) (context.Context, error) {
	if call == nil || !Protected(call.GetToolName()) {
		return ctx, nil
	}
	sel, e := browserSelection(s.profile, call, c, g)
	if s.singleTab != nil {
		sel, e = s.singleSelection(call, c, g)
	}
	if e != nil {
		return ctx, e
	}
	return context.WithValue(ctx, selectionKey{}, sel), nil
}

// Lock the complete authority/source set before effect/budget/resource locks.
// The final hook repeats these checks without introducing new authority rows.
func (s *Store) CheckAdmissionTx(ctx context.Context, tx pgx.Tx, g actionauth.Grant) error {
	if !Protected(g.Binding.ToolName) {
		return nil
	}
	sel, ok := ctx.Value(selectionKey{}).(selection)
	if !ok || sel.authorization != g.ID || sel.tool != g.Binding.ToolName || sel.contract != g.Binding.ContractHash || sel.args != g.Binding.ArgsHash {
		return ErrDenied
	}
	var session string
	var expiry time.Time
	if e := tx.QueryRow(ctx, `SELECT session_key,expires_at FROM session_ownership WHERE run_id=$1 AND lease_id=$2 AND fence_epoch=$3 FOR SHARE`, g.Binding.RunID, g.LeaseID, g.LeaseEpoch).Scan(&session, &expiry); e != nil {
		return ErrDenied
	}
	deadline := g.ExpiresAt
	if expiry.Before(deadline) {
		deadline = expiry
	}
	b := resources.Binding{OwnerID: g.Binding.OwnerID, RunID: g.Binding.RunID, SessionKey: session, TaskID: g.Binding.TaskID, TaskRevision: g.Binding.TaskRevision, ConstraintsRevision: g.Binding.ConstraintsRevision, LeaseEpoch: g.LeaseEpoch, Deadline: deadline}
	ctx = ownership.WithToken(ctx, nil, ownership.Token{SessionKey: session, RunID: b.RunID, LeaseID: g.LeaseID, FenceEpoch: g.LeaseEpoch})
	if e := s.checkAuthority(ctx, tx, g, b); e != nil {
		return e
	}
	_, e := s.sessionTx(ctx, tx, b, sel.resource, sel.tool, false)
	return e
}

func (s *Store) ClaimActionTx(ctx context.Context, tx pgx.Tx, g actionauth.Grant, effect string) error {
	if !Protected(g.Binding.ToolName) {
		return nil
	}
	sel, ok := ctx.Value(selectionKey{}).(selection)
	if !ok || sel.authorization != g.ID || sel.profile != s.profile.ID || sel.target != s.profile.RuntimeTarget || sel.tool != g.Binding.ToolName || sel.contract != g.Binding.ContractHash || sel.args != g.Binding.ArgsHash || s.profile.ContractHashes[sel.tool] != sel.contract || g.Binding.TaskID == "" {
		return ErrDenied
	}
	var session string
	var expiry time.Time
	if e := tx.QueryRow(ctx, `SELECT session_key,expires_at FROM session_ownership WHERE run_id=$1 AND lease_id=$2 AND fence_epoch=$3 FOR SHARE`, g.Binding.RunID, g.LeaseID, g.LeaseEpoch).Scan(&session, &expiry); e != nil {
		return ErrDenied
	}
	deadline, e := s.budgets.ToolDeadlineTx(ctx, tx, g.Binding.OwnerID, effect)
	if e != nil {
		return e
	}
	var at time.Time
	if e = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); e != nil {
		return e
	}
	for _, d := range []time.Time{g.ExpiresAt, expiry, at.Add(resources.MaxLeaseTTL)} {
		if d.Before(deadline) {
			deadline = d
		}
	}
	b := resources.Binding{OwnerID: g.Binding.OwnerID, RunID: g.Binding.RunID, SessionKey: session, TaskID: g.Binding.TaskID, TaskRevision: g.Binding.TaskRevision, ConstraintsRevision: g.Binding.ConstraintsRevision, LeaseEpoch: g.LeaseEpoch, Deadline: deadline}
	ctx = ownership.WithToken(ctx, nil, ownership.Token{SessionKey: session, RunID: b.RunID, LeaseID: g.LeaseID, FenceEpoch: g.LeaseEpoch})
	if e = s.sourceGuard(ctx, tx, b.RunID); e != nil {
		return e
	}
	if sel.single != nil {
		actual, err := s.singleTab(ctx, tx, b, sel.single.Session.ID)
		if err != nil {
			return ErrDenied
		}
		sel.resource, err = singleResource(s.profile.ID, actual)
		if err != nil {
			return err
		}
		// Replace with an immutable per-transaction copy. Prepared contexts can be
		// reused concurrently without a mutable pointer being shared between claims.
		sel.single = &singleTabSelection{Session: actual, Observation: sel.single.Observation}
		ctx = context.WithValue(ctx, selectionKey{}, sel)
	}
	if expiry, e = s.sessionTx(ctx, tx, b, sel.resource, sel.tool, false); e != nil {
		return e
	}
	if !expiry.IsZero() && expiry.Before(b.Deadline) {
		b.Deadline = expiry
	}
	if e = s.resources.RegisterTx(ctx, tx, b, sel.resource); e != nil {
		return e
	}
	var version int64
	if e = tx.QueryRow(ctx, `SELECT version FROM execution_resources WHERE resource_id=$1`, sel.resource).Scan(&version); e != nil {
		return e
	}
	proofs, e := s.resources.AcquireTx(ctx, tx, b, []resources.Request{{ResourceID: sel.resource, Mode: resources.Write, ExpectedVersion: version}}, resources.MaxLeaseTTL)
	if e != nil {
		return e
	}
	p := proofs[0]
	if e = s.resources.ClaimTx(ctx, tx, b, p); e != nil {
		return e
	}
	if e = tx.QueryRow(ctx, `SELECT expires_at FROM execution_resource_leases WHERE lease_id=$1`, p.LeaseID).Scan(&b.Deadline); e != nil {
		return e
	}
	// Source/budget/resource row waits cannot extend the exact grant's deadline.
	if e = s.checkCurrent(ctx, tx, g, b, p, effect); e != nil {
		return e
	}
	var id [24]byte
	if _, e = rand.Read(id[:]); e != nil {
		return e
	}
	ticket := hex.EncodeToString(id[:])
	_, e = tx.Exec(ctx, `INSERT INTO execution_resource_dispatches(ticket_id,authorization_id,action_id,effect_id,owner_id,run_id,session_key,task_id,task_revision,constraints_revision,owner_lease_id,owner_epoch,resource_id,resource_lease_id,resource_version,resource_fence,profile_id,tool_name,contract_hash,args_hash,deadline)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`, ticket, g.ID, g.Binding.ActionID, effect, b.OwnerID, b.RunID, b.SessionKey, b.TaskID, b.TaskRevision, b.ConstraintsRevision, g.LeaseID, b.LeaseEpoch, p.ResourceID, p.LeaseID, p.ExpectedResourceVersion, p.FenceEpoch, s.profile.ID, sel.tool, sel.contract, sel.args, b.Deadline)
	if e == nil && sel.single != nil {
		raw, err := json.Marshal(sel.single)
		if err != nil {
			return err
		}
		_, e = tx.Exec(ctx, `UPDATE execution_resource_dispatches SET single_tab_binding=$2 WHERE ticket_id=$1`, ticket, raw)
	}
	return e
}

func (s *Store) checkAuthority(ctx context.Context, tx pgx.Tx, g actionauth.Grant, b resources.Binding) error {
	if e := (resources.WorkerGuard{}).CheckTx(ctx, tx, b); e != nil {
		return e
	}
	var at time.Time
	if e := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); e != nil {
		return e
	}
	if !g.ExpiresAt.After(at) || !b.Deadline.After(at) {
		return ErrDenied
	}
	if e := actionauth.CheckTaskActionTx(ctx, tx, g.Binding, at); e != nil {
		return e
	}
	var policy string
	if e := tx.QueryRow(ctx, `SELECT execution_policy FROM tasks WHERE task_id=$1 FOR SHARE`, b.TaskID).Scan(&policy); e != nil {
		return e
	}
	if e := actionauth.CheckExecutionPolicy(actionauth.ContextSnapshot{ExecutionPolicy: policy}, g.Binding.Effect); e != nil {
		return e
	}
	if e := s.sourceGuard(ctx, tx, b.RunID); e != nil {
		return e
	}
	return nil
}
func (s *Store) sessionTx(ctx context.Context, tx pgx.Tx, b resources.Binding, resource, tool string, closing bool) (time.Time, error) {
	if singleTabTool(tool) {
		return s.singleSessionTx(ctx, tx, b, resource)
	}
	if tool == "browser.open" || tool == "browser.read" || tool == "browser.open_pages" {
		return time.Time{}, nil
	}
	var expiry time.Time
	var live bool
	lock := " FOR SHARE"
	if closing {
		lock = " FOR UPDATE"
	}
	e := tx.QueryRow(ctx, `SELECT expires_at,owner_id=$3 AND run_id=$4 AND task_id=$5 AND task_revision=$6 AND closed_at IS NULL AND expires_at>clock_timestamp()
 FROM execution_browser_resources WHERE profile_id=$1 AND resource_id=$2`+lock, s.profile.ID, resource, b.OwnerID, b.RunID, b.TaskID, b.TaskRevision).Scan(&expiry, &live)
	if e != nil || !live {
		return time.Time{}, ErrDenied
	}
	return expiry, nil
}
func (s *Store) checkCurrent(ctx context.Context, tx pgx.Tx, g actionauth.Grant, b resources.Binding, p resources.Proof, effect string) error {
	if e := s.checkAuthority(ctx, tx, g, b); e != nil {
		return e
	}
	if expiry, e := s.sessionTx(ctx, tx, b, p.ResourceID, g.Binding.ToolName, false); e != nil || (!expiry.IsZero() && expiry.Before(b.Deadline)) {
		return ErrDenied
	}
	deadline, e := s.budgets.ToolDeadlineTx(ctx, tx, b.OwnerID, effect)
	if e != nil || deadline.Before(b.Deadline) {
		return ErrDenied
	}
	if e = s.resources.CheckTx(ctx, tx, b, p, resources.Write); e != nil {
		return e
	}
	// Evaluate a fresh clock AFTER the resource/lease locks have been obtained.
	var live bool
	e = tx.QueryRow(ctx, `SELECT $2::timestamptz>clock_timestamp() AND $3::timestamptz>clock_timestamp() AND expires_at>clock_timestamp() FROM execution_resource_leases WHERE lease_id=$1`, p.LeaseID, b.Deadline, g.ExpiresAt).Scan(&live)
	if e != nil || !live {
		return ErrDenied
	}
	return nil
}
