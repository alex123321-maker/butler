package actionauth

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/credentialcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool               *pgxpool.Pool
	budgets            *budget.Controller
	contractGuard      ContractGuard
	runDataGuard       func(context.Context, pgx.Tx, string) error
	dispatchClaimGuard DispatchClaimGuard
}

// ContractGuard joins current capability admission to the irreversible dispatch
// transaction after task/owner authority and before effect/budget writes.
type ContractGuard interface {
	CheckActionTx(context.Context, pgx.Tx, ActionBinding) error
}

func (s *PostgresStore) SetContractGuard(guard ContractGuard) { s.contractGuard = guard }

// SetBudgets is trusted deployment wiring, never a model-selectable option.
func (s *PostgresStore) SetBudgets(c *budget.Controller) { s.budgets = c }

// SetRunDataGuard is a trusted owning-memory adapter; the implementation uses
// the same caller transaction and locks current source rows after task authority.
func (s *PostgresStore) SetRunDataGuard(guard func(context.Context, pgx.Tx, string) error) {
	s.runDataGuard = guard
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }
func (s *PostgresStore) PutIntent(ctx context.Context, i Intent) error {
	encoded, err := json.Marshal(i.Binding)
	if err != nil {
		return err
	}
	if i.EffectID == "" || i.EffectFingerprint == "" {
		return fmt.Errorf("durable effect identity is required")
	}
	policy, err := json.Marshal(i.EffectPolicy)
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	key := ""
	if i.EffectPolicy.RetrySafety == "external_idempotency" {
		key = "butler_" + digest(i.EffectID+"\x00"+i.EffectFingerprint)
	}
	var fingerprint string
	err = tx.QueryRow(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state,idempotency_key)
 VALUES($1,$2,$3::jsonb,$4::jsonb,'prepared',$5) ON CONFLICT(action_id) DO UPDATE SET action_id=EXCLUDED.action_id RETURNING fingerprint`, i.EffectID, i.EffectFingerprint, string(encoded), string(policy), key).Scan(&fingerprint)
	if err != nil {
		return err
	}
	// Upgrade only untouched legacy intents whose exact binding still matches.
	if fingerprint != i.EffectFingerprint {
		tag, e := tx.Exec(ctx, `UPDATE action_effects e SET fingerprint=$2,policy=$3::jsonb,idempotency_key=$4 WHERE action_id=$1 AND fingerprint=$5 AND state IN ('prepared','allowed') AND EXISTS(SELECT 1 FROM action_intents i WHERE i.action_id=$1 AND i.action_hash=$5 AND i.effect_id=$1)`, i.EffectID, i.EffectFingerprint, string(policy), key, i.Binding.ActionHash)
		if e != nil || tag.RowsAffected() != 1 {
			return fmt.Errorf("logical action identity already binds a different effect")
		}
	}
	var hash, effectID string
	err = tx.QueryRow(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id,effect_fingerprint,effect_policy)
 VALUES($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9,$10::jsonb) ON CONFLICT(action_id) DO UPDATE SET effect_fingerprint=EXCLUDED.effect_fingerprint,effect_policy=EXCLUDED.effect_policy WHERE action_intents.action_hash=EXCLUDED.action_hash AND action_intents.effect_id=EXCLUDED.effect_id RETURNING action_hash,effect_id`, i.Binding.ActionID, i.Binding.RunID, i.Binding.ToolCallID, i.Binding.ActionHash, string(encoded), i.RequiresApproval, i.ExpiresAt, i.EffectID, i.EffectFingerprint, string(policy)).Scan(&hash, &effectID)
	if err != nil {
		return err
	}
	if hash != i.Binding.ActionHash {
		return fmt.Errorf("tool call identity already binds a different action")
	}
	return tx.Commit(ctx)
}
func (s *PostgresStore) GetIntent(ctx context.Context, id string) (Intent, error) {
	var i Intent
	var encoded string
	var policy string
	err := s.pool.QueryRow(ctx, `SELECT binding::text,requires_approval,expires_at,effect_id,effect_fingerprint,effect_policy::text FROM action_intents WHERE action_id=$1`, id).Scan(&encoded, &i.RequiresApproval, &i.ExpiresAt, &i.EffectID, &i.EffectFingerprint, &policy)
	if err != nil {
		return i, err
	}
	err = json.Unmarshal([]byte(encoded), &i.Binding)
	if err == nil {
		err = json.Unmarshal([]byte(policy), &i.EffectPolicy)
	}
	return i, err
}
func (s *PostgresStore) PutGrant(ctx context.Context, g Grant) (Grant, error) {
	encoded, err := json.Marshal(g)
	if err != nil {
		return Grant{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Grant{}, err
	}
	defer tx.Rollback(ctx)
	var state string
	var effectID string
	err = tx.QueryRow(ctx, `SELECT e.action_id,e.state FROM action_effects e JOIN action_intents i ON i.effect_id=e.action_id WHERE i.action_id=$1 AND i.action_hash=$2 FOR UPDATE OF e`, g.Binding.ActionID, g.Binding.ActionHash).Scan(&effectID, &state)
	if err != nil || (state != string(StatePrepared) && state != string(StateAllowed)) {
		return Grant{}, fmt.Errorf("external effect already dispatched; reconcile before any new attempt")
	}
	var stored string
	// Only an unused grant may be refreshed after worker takeover. Once dispatch
	// begins, even a timeout cannot turn this action into a reusable permission.
	err = tx.QueryRow(ctx, `INSERT INTO action_authorizations(authorization_id,action_id,action_hash,grant_data,mandate_id,mandate_revision,expires_at,state)
 VALUES($1,$2,$3,$4::jsonb,NULLIF($5,''),$6,$7,'authorized') ON CONFLICT(action_id) DO UPDATE
 SET authorization_id=EXCLUDED.authorization_id,grant_data=EXCLUDED.grant_data,mandate_id=EXCLUDED.mandate_id,mandate_revision=EXCLUDED.mandate_revision,expires_at=EXCLUDED.expires_at
 WHERE action_authorizations.state='authorized' AND action_authorizations.action_hash=EXCLUDED.action_hash RETURNING grant_data::text`, g.ID, g.Binding.ActionID, g.Binding.ActionHash, string(encoded), g.MandateID, g.MandateRevision, g.ExpiresAt).Scan(&stored)
	if err != nil {
		return Grant{}, fmt.Errorf("action authorization cannot be refreshed after dispatch")
	}
	err = json.Unmarshal([]byte(stored), &g)
	if err != nil {
		return Grant{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE action_effects SET state='allowed',authorization_id=$2,updated_at=NOW() WHERE action_id=$1`, effectID, g.ID)
	if err != nil {
		return Grant{}, err
	}
	return g, tx.Commit(ctx)
}
func (s *PostgresStore) GetGrant(ctx context.Context, id string) (Grant, error) {
	var g Grant
	var encoded string
	var state string
	err := s.pool.QueryRow(ctx, `SELECT grant_data::text,state FROM action_authorizations WHERE authorization_id=$1`, id).Scan(&encoded, &state)
	if err != nil {
		return g, err
	}
	err = json.Unmarshal([]byte(encoded), &g)
	g.State = state
	return g, err
}
func (s *PostgresStore) ClaimGrant(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	g, err := grantInTx(ctx, tx, id)
	if err != nil {
		return err
	}
	if s.dispatchClaimGuard != nil {
		if err = s.dispatchClaimGuard.CheckAdmissionTx(ctx, tx, g); err != nil {
			return err
		}
	}
	if err = s.guardDispatch(ctx, tx, g); err != nil {
		return err
	}
	var effectID string
	err = tx.QueryRow(ctx, `SELECT e.action_id FROM action_effects e JOIN action_intents i ON i.effect_id=e.action_id WHERE i.action_id=$1 AND e.state='allowed' AND e.authorization_id=$2 FOR UPDATE OF e`, g.Binding.ActionID, id).Scan(&effectID)
	if err != nil {
		return fmt.Errorf("external effect is already executing or uncertain; do not replay")
	}
	if s.budgets != nil {
		if err = s.budgets.ReserveToolTx(ctx, tx, toolBudgetIdentity(g, effectID)); err != nil {
			return err
		}
	}
	// Budget row contention can cross a policy deadline or active-hours boundary.
	// Reuse the authority locks already held; never acquire a new authority set.
	if g.Binding.TaskID != "" {
		var at time.Time
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
			return err
		}
		if err = CheckTaskActionTx(ctx, tx, g.Binding, at); err != nil {
			return err
		}
	}
	if s.dispatchClaimGuard != nil {
		if err = s.dispatchClaimGuard.ClaimActionTx(ctx, tx, g, effectID); err != nil {
			return err
		}
	}
	// Retain the Input row lock acquired by guardDispatch and recheck time after
	// budget contention. A fresh claim cannot borrow an unused old permission.
	if credentialcontrol.IsTool(g.Binding.ToolName) {
		if err = checkCredentialInputTx(ctx, tx, g); err != nil {
			return err
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE action_authorizations a SET state='inflight',dispatched_at=NOW()
 WHERE authorization_id=$1 AND state='authorized' AND expires_at>clock_timestamp()
 AND (mandate_id IS NULL OR EXISTS(SELECT 1 FROM action_mandates m WHERE m.mandate_id=a.mandate_id AND m.revision=a.mandate_revision AND m.revoked_at IS NULL AND m.expires_at>clock_timestamp()))`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("authorization already dispatched, expired or revoked; do not replay the action")
	}
	if _, err = tx.Exec(ctx, `UPDATE action_effects SET state='executing',dispatch_outcome='executing',updated_at=NOW() WHERE action_id=$1`, effectID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO action_effect_dispatches(authorization_id,action_id,execution_action_id,outcome) VALUES($1,$2,$3,'executing')`, id, effectID, g.Binding.ActionID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func grantInTx(ctx context.Context, tx pgx.Tx, id string) (Grant, error) {
	var g Grant
	var data, state string
	err := tx.QueryRow(ctx, `SELECT grant_data::text,state FROM action_authorizations WHERE authorization_id=$1`, id).Scan(&data, &state)
	if err != nil {
		return g, err
	}
	err = json.Unmarshal([]byte(data), &g)
	g.State = state
	return g, err
}

// Live ownership and task locks serialize cancellation/amendment with the
// irreversible journal commit. They are released before contacting the runtime.
func (s *PostgresStore) guardDispatch(ctx context.Context, tx pgx.Tx, g Grant) error {
	var session string
	err := tx.QueryRow(ctx, `SELECT session_key FROM session_ownership WHERE run_id=$1 AND lease_id=$2 AND fence_epoch=$3 AND expires_at>clock_timestamp() FOR SHARE`, g.Binding.RunID, g.LeaseID, g.LeaseEpoch).Scan(&session)
	if err != nil {
		return fmt.Errorf("live execution ownership is required at dispatch commit")
	}
	var state, status string
	if err = tx.QueryRow(ctx, `SELECT current_state,status FROM runs WHERE run_id=$1 FOR SHARE`, g.Binding.RunID).Scan(&state, &status); err != nil {
		return fmt.Errorf("live run is required at dispatch commit")
	}
	for _, value := range []string{state, status} {
		switch value {
		case "", "completed", "failed", "cancelled", "finalizing":
			return fmt.Errorf("run is terminal or finalizing before dispatch commit")
		}
	}
	if g.Binding.TaskID != "" {
		var status, owner, policy string
		var revision, constraints, pinned int64
		err = tx.QueryRow(ctx, `SELECT task_revision FROM runs WHERE run_id=$1 AND task_id=$2 FOR SHARE`, g.Binding.RunID, g.Binding.TaskID).Scan(&pinned)
		if err != nil {
			return fmt.Errorf("pinned task run is unavailable")
		}
		err = tx.QueryRow(ctx, `SELECT status,revision,constraints_revision,owner_id,execution_policy FROM tasks WHERE task_id=$1 FOR SHARE`, g.Binding.TaskID).Scan(&status, &revision, &constraints, &owner, &policy)
		if err != nil || status != "active" || revision != g.Binding.TaskRevision || pinned != revision || constraints != g.Binding.ConstraintsRevision || owner != g.Binding.OwnerID {
			return fmt.Errorf("task authority changed before dispatch commit")
		}
		if err = CheckExecutionPolicy(ContextSnapshot{ExecutionPolicy: policy}, g.Binding.Effect); err != nil {
			return err
		}
		if err = CheckTaskActionTx(ctx, tx, g.Binding, time.Now().UTC()); err != nil {
			return err
		}
	}
	if s.runDataGuard != nil {
		// Broker RPC callers do not carry a core worker context. The owning
		// source verifier receives read proof from the exact PG lease already
		// locked above, never caller Actor/token metadata. Nil Authority prevents
		// borrowing another fence/connection or using this proof as write power;
		// it is confined to this guard's existing transaction and does not escape.
		proofCtx := owneridentity.WithActor(ctx, owneridentity.Actor{})
		proofCtx = ownership.WithToken(proofCtx, nil, ownership.Token{SessionKey: session, RunID: g.Binding.RunID, LeaseID: g.LeaseID, FenceEpoch: g.LeaseEpoch})
		if err = s.runDataGuard(proofCtx, tx, g.Binding.RunID); err != nil {
			return err
		}
	}
	if (g.Binding.ExtensionID != "" || g.Binding.ConverterID != "" || strings.HasPrefix(g.Binding.ToolName, "skills.") || strings.HasPrefix(g.Binding.ToolName, "converters.") || strings.HasPrefix(g.Binding.ToolName, "converter.")) && s.contractGuard == nil {
		return fmt.Errorf("reviewed capability admission is unavailable at dispatch")
	}
	if s.contractGuard != nil {
		if err = s.contractGuard.CheckActionTx(ctx, tx, g.Binding); err != nil {
			return err
		}
	}
	if credentialcontrol.IsTool(g.Binding.ToolName) {
		return checkCredentialInputTx(ctx, tx, g)
	}
	return nil
}

func checkCredentialInputTx(ctx context.Context, tx pgx.Tx, g Grant) error {
	var input, claim string
	var claimExpiry, expiry, at time.Time
	err := tx.QueryRow(ctx, `SELECT n.input_id,n.claim_id,n.claim_expires_at,n.expires_at
FROM accepted_inputs n JOIN runs r ON r.run_id=n.run_id AND r.session_key=n.session_key
JOIN sessions s ON s.session_key=n.session_key AND s.user_id=n.owner_id
WHERE n.run_id=$1 AND n.owner_id=$2 AND COALESCE(n.task_id,'')=$3
AND COALESCE(r.task_id,'')=$3 AND n.status='running' FOR SHARE OF n`, g.Binding.RunID, g.Binding.OwnerID, g.Binding.TaskID).Scan(&input, &claim, &claimExpiry, &expiry)
	if err != nil || len(g.InputClaimHash) != 64 || InputClaimHash(input, claim) != g.InputClaimHash {
		return fmt.Errorf("credential request owning Input claim changed before dispatch commit")
	}
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at) != nil || !claimExpiry.After(at) || !expiry.After(at) {
		return fmt.Errorf("credential request owning Input claim expired before dispatch commit")
	}
	return nil
}
func (s *PostgresStore) FinishGrant(ctx context.Context, id, state string) error {
	if state != "dispatch_completed" && state != "unknown" {
		return fmt.Errorf("unsupported dispatch outcome")
	}
	g, err := s.GetGrant(ctx, id)
	if err != nil {
		return err
	}
	i, err := s.GetIntent(ctx, g.Binding.ActionID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = s.RecordDispatchOutcome(ctx, id, state, EffectEvidence{EvidenceID: "evidence_" + randomID(), ActionID: i.EffectID, SourceActionID: g.Binding.ActionID, SourceRunID: g.Binding.RunID, SourceToolCallID: g.Binding.ToolCallID, Source: "runtime_response", Method: i.EffectPolicy.VerificationMethod, Verdict: StateUnknown, ObservedAt: now, RecordedAt: now})
	return err
}
func (s *PostgresStore) GetMandate(ctx context.Context, id string) (Mandate, error) {
	var m Mandate
	var encoded string
	err := s.pool.QueryRow(ctx, `SELECT mandate_data::text,revision,revoked_at,status,expires_at FROM action_mandates WHERE mandate_id=$1`, id).Scan(&encoded, &m.Revision, &m.RevokedAt, &m.Status, &m.ExpiresAt)
	if err != nil {
		return m, err
	}
	revision, revoked := m.Revision, m.RevokedAt
	status, expires := m.Status, m.ExpiresAt
	err = json.Unmarshal([]byte(encoded), &m)
	m.Revision, m.RevokedAt = revision, revoked
	m.Status, m.ExpiresAt = status, expires
	return m, err
}
func (s *PostgresStore) PutMandate(ctx context.Context, m Mandate) error {
	if m.ScopeKind == "owner_goal" {
		return fmt.Errorf("broad owner authority requires authenticated owner goal service")
	}
	if err := ValidateMandate(m, time.Now().UTC()); err != nil {
		return err
	}
	encoded, err := json.Marshal(m)
	if err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at) VALUES($1,$2,$3,$4,$5::jsonb,$6)
 ON CONFLICT(mandate_id) DO UPDATE SET revision=EXCLUDED.revision,mandate_data=EXCLUDED.mandate_data,expires_at=EXCLUDED.expires_at
 WHERE action_mandates.scope_kind='task' AND action_mandates.owner_id=EXCLUDED.owner_id AND action_mandates.task_id=EXCLUDED.task_id AND action_mandates.revision+1=EXCLUDED.revision AND action_mandates.revoked_at IS NULL`, m.ID, m.OwnerID, m.TaskID, m.Revision, string(encoded), m.ExpiresAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("mandate owner or revision is incompatible")
	}
	return nil
}
func (s *PostgresStore) RevokeMandate(ctx context.Context, id, owner string, revision int64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE action_mandates SET revoked_at=NOW(),status='revoked',revision=revision+1 WHERE mandate_id=$1 AND owner_id=$2 AND revision=$3 AND scope_kind='task' AND revoked_at IS NULL`, id, owner, revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("mandate owner or revision is incompatible")
	}
	return nil
}
