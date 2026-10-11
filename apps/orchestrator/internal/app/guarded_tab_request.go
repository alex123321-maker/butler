package app

import (
	"context"
	"strings"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/singletab"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type guardedRequestProof struct {
	envelope singletabcontrol.RequestEnvelope
	grant    actionauth.Grant
}
type guardedRequestKey struct{}
type GuardedTabRequests struct {
	pool        *pgxpool.Pool
	leases      *session.PostgresLeaseManager
	sources     resourceexec.SourceGuard
	checkpoints *singletab.SelectionCheckpoints
	inputs      *durableinput.Store
	controller  *singletabcontrol.Controller
	hash        string
}

func NewGuardedTabRequests(pool *pgxpool.Pool, metadata singletabcontrol.Metadata, sources resourceexec.SourceGuard, host string, checkpoints *singletab.SelectionCheckpoints) (*GuardedTabRequests, error) {
	if pool == nil || sources == nil || checkpoints == nil {
		return nil, singletabcontrol.ErrDenied
	}
	hash, err := actionauth.ContractHash(singletabcontrol.RequestContract())
	if err != nil {
		return nil, err
	}
	s := &GuardedTabRequests{pool: pool, leases: session.NewPostgresLeaseManager(pool), sources: sources, checkpoints: checkpoints, hash: hash, inputs: durableinput.NewStore(pool)}
	c, err := singletabcontrol.NewDispatchedRequests(pool, metadata, s, sources, host, s)
	if err != nil {
		return nil, err
	}
	// The request-only wrapper does not expose Select or Revoke to the Broker.
	// Controller's owning publication hook proves the real input and dispatch.
	s.controller = c
	return s, nil
}
func normalizeGuardedBody(body string) string { return strings.Join(strings.Fields(body), " ") }

func (s *GuardedTabRequests) CheckReady(ctx context.Context) error {
	if s == nil || singletabcontrol.CheckSchema(ctx, s.pool) != nil {
		return singletabcontrol.ErrDenied
	}
	return s.checkDispatchSchema(ctx, s.pool)
}

func (s *GuardedTabRequests) checkDispatchSchema(ctx context.Context, q admissionQuery) error {
	var body string
	err := q.QueryRow(ctx, `SELECT p.prosrc FROM pg_trigger t JOIN pg_proc p ON p.oid=t.tgfoid WHERE t.tgrelid=to_regclass('guarded_single_tab_dispatches') AND t.tgname='guarded_dispatch_immutable' AND t.tgenabled IN ('O','A') AND t.tgtype=27 AND NOT t.tgisinternal AND p.proname='guarded_tab_dispatch_immutable' AND (SELECT count(*) FROM pg_attribute WHERE attrelid=t.tgrelid AND attname IN ('request_id','run_id','tool_call_id','effect_id','authorization_id','contract_hash','created_at') AND NOT attisdropped)=7`).Scan(&body)
	if err != nil || normalizeGuardedBody(body) != normalizeGuardedBody(`BEGIN RAISE EXCEPTION 'guarded request dispatch history is immutable'; END`) {
		return singletabcontrol.ErrDenied
	}
	return nil
}

func (s *GuardedTabRequests) readDispatch(ctx context.Context, q admissionQuery, e singletabcontrol.RequestEnvelope, lock bool) (actionauth.Grant, error) {
	// Reuse the exact permanent dispatch reader; no delegation is performed.
	return readAdmissionDispatch(ctx, q, delegation.AdmissionEnvelope{RunID: e.RunID, ToolCallID: e.ToolCallID, EffectID: e.EffectID, ArgsJSON: e.ArgsJSON, ContractHash: e.ContractHash}, singletabcontrol.BindTool, s.hash, lock)
}
func (s *GuardedTabRequests) RequestDispatched(ctx context.Context, e singletabcontrol.RequestEnvelope) (singletabcontrol.RequestReceipt, error) {
	if s == nil || !singletabcontrol.ValidRequestArgs(e.ArgsJSON) || e.RunID == "" || e.ToolCallID == "" || e.EffectID == "" || e.ContractHash != s.hash || len(e.RunID) > 512 || len(e.ToolCallID) > 512 || len(e.EffectID) > 512 {
		return singletabcontrol.RequestReceipt{}, singletabcontrol.ErrDenied
	}
	if _, owner := owneridentity.FromContext(ctx); owner {
		return singletabcontrol.RequestReceipt{}, singletabcontrol.ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	grant, err := s.readDispatch(ctx, s.pool, e, false)
	if err != nil {
		return singletabcontrol.RequestReceipt{}, singletabcontrol.ErrDenied
	}
	var key, channel string
	var expiry time.Time
	if s.pool.QueryRow(ctx, `SELECT r.session_key,s.channel,o.expires_at FROM runs r JOIN sessions s USING(session_key) JOIN session_ownership o USING(session_key) WHERE r.run_id=$1 AND o.run_id=r.run_id AND o.lease_id=$2 AND o.fence_epoch=$3 AND o.expires_at>clock_timestamp()`, e.RunID, grant.LeaseID, grant.LeaseEpoch).Scan(&key, &channel, &expiry) != nil {
		return singletabcontrol.RequestReceipt{}, singletabcontrol.ErrDenied
	}
	if channel != "web" && channel != "telegram" {
		return singletabcontrol.RequestReceipt{}, singletabcontrol.ErrDenied
	}
	var claim durableinput.Record
	if s.pool.QueryRow(ctx, `SELECT input_id,claim_id FROM accepted_inputs WHERE run_id=$1 AND session_key=$2 AND owner_id=$3 AND task_id=$4 AND status='running' AND checkpoint='tool_dispatching' AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp()`, e.RunID, key, grant.Binding.OwnerID, grant.Binding.TaskID).Scan(&claim.InputID, &claim.ClaimID) != nil {
		return singletabcontrol.RequestReceipt{}, singletabcontrol.ErrDenied
	}
	if grant.InputClaimHash == "" || actionauth.InputClaimHash(claim.InputID, claim.ClaimID) != grant.InputClaimHash {
		return singletabcontrol.RequestReceipt{}, singletabcontrol.ErrDenied
	}
	if grant.ExpiresAt.Before(expiry) {
		expiry = grant.ExpiresAt
	}
	token := ownership.Token{SessionKey: key, RunID: e.RunID, LeaseID: grant.LeaseID, FenceEpoch: grant.LeaseEpoch}
	owned := ownership.WithToken(durableinput.WithClaim(ctx, claim), s.leases.Authority(), token)
	owned = context.WithValue(owned, guardedRequestKey{}, guardedRequestProof{e, grant})
	// This identifies the request's current owner/channel, never an approval or
	// browser permission. Worker/source/permanent dispatch checks remain required.
	owned = owneridentity.WithActor(owned, owneridentity.Actor{OwnerID: grant.Binding.OwnerID, SessionKey: key, Channel: channel, DeliveryID: "broker-request:" + e.EffectID})
	b := resources.Binding{OwnerID: grant.Binding.OwnerID, SessionKey: key, RunID: e.RunID, TaskID: grant.Binding.TaskID, TaskRevision: grant.Binding.TaskRevision, ConstraintsRevision: grant.Binding.ConstraintsRevision, LeaseEpoch: grant.LeaseEpoch, Deadline: expiry}
	selection, err := s.controller.Request(owned, b)
	if err != nil {
		return singletabcontrol.RequestReceipt{}, singletabcontrol.ErrDenied
	}
	return singletabcontrol.RequestReceipt{ApprovalID: selection.ApprovalID, State: "awaiting_owner_choice"}, nil
}
func (s *GuardedTabRequests) CheckTx(ctx context.Context, tx pgx.Tx, b resources.Binding) error {
	p, ok := ctx.Value(guardedRequestKey{}).(guardedRequestProof)
	if !ok || singletabcontrol.CheckSchemaTx(ctx, tx) != nil || s.checkDispatchSchema(ctx, tx) != nil || (resources.WorkerGuard{}).CheckTx(ctx, tx, b) != nil || s.sources(ctx, tx, b.RunID) != nil {
		return singletabcontrol.ErrDenied
	}
	grant, err := s.readDispatch(ctx, tx, p.envelope, true)
	if err != nil || grant.ID != p.grant.ID || grant.InputClaimHash != p.grant.InputClaimHash || grant.Binding.ActionHash != p.grant.Binding.ActionHash || b.RunID != grant.Binding.RunID || b.TaskID != grant.Binding.TaskID || b.OwnerID != grant.Binding.OwnerID || b.TaskRevision != grant.Binding.TaskRevision || b.ConstraintsRevision != grant.Binding.ConstraintsRevision || b.Deadline.After(p.grant.ExpiresAt) || s.inputs.CheckDispatchClaimTx(ctx, tx, b.RunID, p.envelope.ToolCallID, p.envelope.EffectID, p.grant.ID) != nil {
		return singletabcontrol.ErrDenied
	}
	return nil
}
func (s *GuardedTabRequests) SuspendSelectionTx(ctx context.Context, tx pgx.Tx, b resources.Binding, selection singletabcontrol.Selection) error {
	p, ok := ctx.Value(guardedRequestKey{}).(guardedRequestProof)
	if !ok {
		return singletabcontrol.ErrDenied
	}
	if _, err := tx.Exec(ctx, `INSERT INTO guarded_single_tab_dispatches(request_id,run_id,tool_call_id,effect_id,authorization_id,contract_hash) VALUES($1,$2,$3,$4,$5,$6)`, selection.ApprovalID, b.RunID, p.envelope.ToolCallID, p.envelope.EffectID, p.grant.ID, s.hash); err != nil {
		return err
	}
	return s.checkpoints.SuspendToolSelectionTx(ctx, tx, b, selection)
}
func (s *GuardedTabRequests) ResolveSelectionTx(context.Context, pgx.Tx, resources.Binding, string, bool) error {
	return singletabcontrol.ErrDenied
}

// PendingRequest reads the actual atomically suspended request, even when the
// private RPC ACK is lost. It does not replay the Broker call or settle its
// journal. Fresh recovery separately requires a confirmed request outcome.
func (s *GuardedTabRequests) PendingRequest(ctx context.Context, runID, callID, effectID string) (flow.ApprovalRequest, error) {
	token, ok := ownership.TokenFromContext(ctx)
	if s == nil || !ok || token.RunID != runID {
		return flow.ApprovalRequest{}, singletabcontrol.ErrDenied
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return flow.ApprovalRequest{}, err
	}
	defer tx.Rollback(context.Background())
	var valid bool
	if tx.QueryRow(ctx, `SELECT run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp() FROM session_ownership WHERE session_key=$1 FOR UPDATE`, token.SessionKey, runID, token.LeaseID, token.FenceEpoch).Scan(&valid) != nil || !valid {
		return flow.ApprovalRequest{}, singletabcontrol.ErrDenied
	}
	var request flow.ApprovalRequest
	err = tx.QueryRow(ctx, `SELECT q.approval_id FROM guarded_single_tab_dispatches d JOIN guarded_single_tab_requests q ON q.request_id=d.request_id JOIN runs r ON r.run_id=d.run_id JOIN tasks t ON t.task_id=q.task_id JOIN sessions s ON s.session_key=q.session_key JOIN accepted_inputs i ON i.run_id=r.run_id WHERE d.run_id=$1 AND d.tool_call_id=$2 AND d.effect_id=$3 AND d.contract_hash=$4 AND q.session_key=$5 AND s.user_id=q.owner_id AND t.owner_id=q.owner_id AND t.session_key=q.session_key AND t.last_run_id=r.run_id AND t.status='waiting_for_user' AND t.revision=q.task_revision AND t.constraints_revision=q.constraints_revision AND r.task_id=t.task_id AND r.task_revision=t.revision AND r.current_state='awaiting_approval' AND r.status='awaiting_approval' AND q.state IN ('pending','selected','revoked') AND q.expires_at>clock_timestamp() AND i.status='running' AND i.claim_expires_at>clock_timestamp() AND i.expires_at>clock_timestamp() AND i.checkpoint IN ('awaiting_approval','approval_ready') AND i.checkpoint_json->>'approval_id'=q.approval_id AND i.checkpoint_json->'continuation'->>'kind'='guarded_tab_selection' AND i.checkpoint_json->'continuation'->>'version'='1' AND i.checkpoint_json->'continuation'->>'approval_id'=q.approval_id FOR SHARE OF r,t,s`, runID, callID, effectID, s.hash, token.SessionKey).Scan(&request.ApprovalID)
	if err != nil {
		return flow.ApprovalRequest{}, singletabcontrol.ErrDenied
	}
	var taskID, ownerID string
	var at time.Time
	if tx.QueryRow(ctx, `SELECT r.task_id,s.user_id,clock_timestamp() FROM runs r JOIN sessions s USING(session_key) WHERE r.run_id=$1`, runID).Scan(&taskID, &ownerID, &at) != nil {
		return flow.ApprovalRequest{}, singletabcontrol.ErrDenied
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, ownerID, nil, at); err != nil {
		return flow.ApprovalRequest{}, singletabcontrol.ErrDenied
	}
	if s.sources(ctx, tx, runID) != nil || s.checkpoints.CheckPendingSelectionTx(ctx, tx, runID, request.ApprovalID) != nil {
		return flow.ApprovalRequest{}, singletabcontrol.ErrDenied
	}
	// Claim/source/authority locks may have waited beyond either finite expiry.
	if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM session_ownership o JOIN guarded_single_tab_requests q ON q.session_key=o.session_key WHERE q.approval_id=$1 AND o.run_id=$2 AND o.lease_id=$3 AND o.fence_epoch=$4 AND o.expires_at>clock_timestamp() AND q.expires_at>clock_timestamp())`, request.ApprovalID, runID, token.LeaseID, token.FenceEpoch).Scan(&valid) != nil || !valid || tx.Commit(ctx) != nil {
		return flow.ApprovalRequest{}, singletabcontrol.ErrDenied
	}
	request.RunID, request.SessionKey, request.ToolCallID = runID, token.SessionKey, callID
	request.ToolName, request.ApprovalType, request.ArgsJSON = singletabcontrol.BindTool, "browser_tab_selection", "{}"
	request.Summary = "Выберите вкладку в Butler → Подтверждения. Выбор не разрешает действие на странице."
	request.RiskLevel = "medium"
	return request, nil
}
