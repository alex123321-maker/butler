package workspaceexec

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/workspace"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Profile struct{ ID, Target string }
type DispatchStore struct {
	pool       *pgxpool.Pool
	resources  *resources.Store
	budgets    *budget.Controller
	provenance *Provenance
	profile    Profile
	hashes     map[string]string
}
type preparedCall struct {
	authorization, tool, contract, args, workspace string
	data                                           Args
}
type callKey struct{}
type prepared struct {
	authorization, profile, tool, contract, args, input, path, bodyHash string
	bytes                                                               int64
	source                                                              SourceContext
	expected                                                            workspace.Snapshot
}

func NewDispatchStore(pool *pgxpool.Pool, r *resources.Store, b *budget.Controller, p *Provenance, profile Profile) (*DispatchStore, error) {
	if pool == nil || r == nil || b == nil || p == nil || p.pool != pool || profile.ID == "" || len(profile.ID) > 128 || profile.Target == "" || len(profile.Target) > 512 {
		return nil, ErrDenied
	}
	hashes := map[string]string{}
	for _, c := range Contracts(profile.Target) {
		h, err := actionauth.ContractHash(c)
		if err != nil {
			return nil, err
		}
		hashes[c.ToolName] = h
	}
	return &DispatchStore{pool, r, b, p, profile, hashes}, nil
}
func (s *DispatchStore) CheckReady(ctx context.Context) error {
	if s == nil || s.provenance.CheckReady(ctx) != nil {
		return ErrDenied
	}
	_, err := s.pool.Exec(ctx, `SELECT a.authorization_id,a.source_context,a.input_claim_hash,d.ticket_id,d.resource_lease_id,e.observation_hash,i.claim_id,i.checkpoint_json FROM code_workspace_authorizations a,code_workspace_dispatches d,code_workspace_effect_evidence e,accepted_inputs i WHERE FALSE`)
	return err
}
func (s *DispatchStore) PrepareContext(ctx context.Context, call *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract, g actionauth.Grant) (context.Context, error) {
	if !Protected(call.GetToolName()) {
		return ctx, nil
	}
	if s == nil || !ExactContract(c) || c.GetRuntimeTarget() != s.profile.Target || call.GetToolName() != c.GetToolName() || call.GetRunId() != g.Binding.RunID || call.GetToolCallId() != g.Binding.ToolCallID || call.GetToolName() != g.Binding.ToolName || len(call.GetCredentialRefs()) != 0 || len(g.Binding.CredentialRefs) != 0 || !sha256Digest(g.InputClaimHash) {
		return ctx, ErrDenied
	}
	ch, err := actionauth.ContractHash(c)
	if err != nil || ch != g.Binding.ContractHash || s.hashes[call.GetToolName()] != ch {
		return ctx, ErrDenied
	}
	ah, err := actionauth.ArgumentsHash(call.GetArgsJson())
	if err != nil || ah != g.Binding.ArgsHash {
		return ctx, ErrDenied
	}
	a, err := DecodeArgs(call.GetToolName(), call.GetArgsJson())
	if err != nil {
		return ctx, err
	}
	id := a.WorkspaceID
	if call.GetToolName() == CreateTool {
		var random [16]byte
		if _, err = rand.Read(random[:]); err != nil {
			return ctx, err
		}
		id = hex.EncodeToString(random[:])
	}
	return context.WithValue(ctx, callKey{}, preparedCall{g.ID, call.GetToolName(), ch, ah, id, a}), nil
}
func (s *DispatchStore) ownedTx(ctx context.Context, tx pgx.Tx, g actionauth.Grant) (context.Context, resources.Binding, time.Time, func(), error) {
	var key string
	var expiry time.Time
	if tx.QueryRow(ctx, `SELECT session_key,expires_at FROM session_ownership WHERE run_id=$1 AND lease_id=$2 AND fence_epoch=$3 FOR SHARE`, g.Binding.RunID, g.LeaseID, g.LeaseEpoch).Scan(&key, &expiry) != nil {
		return nil, resources.Binding{}, time.Time{}, nil, ErrDenied
	}
	if g.ExpiresAt.Before(expiry) {
		expiry = g.ExpiresAt
	}
	ctx = ownership.WithToken(ctx, nil, ownership.Token{SessionKey: key, RunID: g.Binding.RunID, LeaseID: g.LeaseID, FenceEpoch: g.LeaseEpoch})
	owned, stop, err := ownedContext(ctx)
	if err != nil {
		return nil, resources.Binding{}, time.Time{}, nil, err
	}
	b := resources.Binding{OwnerID: g.Binding.OwnerID, RunID: g.Binding.RunID, SessionKey: key, TaskID: g.Binding.TaskID, TaskRevision: g.Binding.TaskRevision, ConstraintsRevision: g.Binding.ConstraintsRevision, LeaseEpoch: g.LeaseEpoch, Deadline: expiry}
	if (resources.WorkerGuard{}).CheckTx(owned, tx, b) != nil {
		stop()
		return nil, b, time.Time{}, nil, ErrDenied
	}
	var at time.Time
	if tx.QueryRow(owned, `SELECT clock_timestamp()`).Scan(&at) != nil || !g.ExpiresAt.After(at) || actionauth.CheckTaskActionTx(owned, tx, g.Binding, at) != nil {
		stop()
		return nil, b, time.Time{}, nil, ErrDenied
	}
	return owned, b, expiry, stop, nil
}

// Exact original checkpoint is checked before source/effect/budget/resource
// locks and again after waits. No reusable claim ID enters an argument/ticket.
func checkInputTx(ctx context.Context, tx pgx.Tx, g actionauth.Grant, b resources.Binding, effect string) (time.Time, error) {
	var input, claim string
	var deadline time.Time
	err := tx.QueryRow(ctx, `SELECT input_id,claim_id,LEAST(expires_at,claim_expires_at) FROM accepted_inputs WHERE run_id=$1 AND session_key=$2 AND owner_id=$3 AND task_id=$4 AND status='running' AND checkpoint='tool_dispatching' AND checkpoint_json->'checkpoint_data'->>'tool_call_id'=$5 AND checkpoint_json->'checkpoint_data'->>'effect_id'=$6 AND checkpoint_json->'checkpoint_data'->>'authorization_id'=$7 AND claim_expires_at>clock_timestamp() AND expires_at>clock_timestamp() FOR UPDATE`, b.RunID, b.SessionKey, b.OwnerID, b.TaskID, g.Binding.ToolCallID, effect, g.ID).Scan(&input, &claim, &deadline)
	if err != nil || !sha256Digest(g.InputClaimHash) || actionauth.InputClaimHash(input, claim) != g.InputClaimHash {
		return time.Time{}, ErrDenied
	}
	return deadline, nil
}
func bodyDigest(data []byte) string {
	value := sha256.Sum256(data)
	return hex.EncodeToString(value[:])
}

func (s *DispatchStore) CheckAdmissionTx(ctx context.Context, tx pgx.Tx, g actionauth.Grant) error {
	if !Protected(g.Binding.ToolName) {
		return nil
	}
	c, ok := ctx.Value(callKey{}).(preparedCall)
	if s == nil || !ok || c.authorization != g.ID || c.tool != g.Binding.ToolName || c.contract != g.Binding.ContractHash || c.args != g.Binding.ArgsHash || s.hashes[c.tool] != c.contract {
		return ErrDenied
	}
	owned, b, _, stop, err := s.ownedTx(ctx, tx, g)
	if err != nil {
		return err
	}
	defer stop()
	var effect string
	if tx.QueryRow(owned, `SELECT effect_id FROM action_intents WHERE action_id=$1 AND action_hash=$2`, g.Binding.ActionID, g.Binding.ActionHash).Scan(&effect) != nil {
		return ErrDenied
	}
	if _, err = checkInputTx(owned, tx, g, b, effect); err != nil {
		return err
	}
	ref := workspace.Ref{ID: c.workspace, Binding: workspace.Binding{OwnerID: b.OwnerID, SessionKey: b.SessionKey, TaskID: b.TaskID, TaskRevision: b.TaskRevision, ConstraintsRevision: b.ConstraintsRevision}}
	sources, err := s.provenance.PrepareTx(owned, tx, ref, c.tool == CreateTool)
	if err != nil {
		return err
	}
	raw, err := encodeContext(sources)
	if err != nil {
		return err
	}
	expected, _ := json.Marshal(c.data.Expected)
	body := ""
	if c.tool == WriteTool {
		body = bodyDigest(c.data.Content)
	}
	_, err = tx.Exec(owned, `INSERT INTO code_workspace_authorizations(authorization_id,profile_id,workspace_id,tool_name,contract_hash,args_hash,source_context,path,body_sha256,body_bytes,expected_snapshot,input_claim_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT(authorization_id) DO NOTHING`, g.ID, s.profile.ID, ref.ID, c.tool, c.contract, c.args, raw, c.data.Path, body, len(c.data.Content), expected, g.InputClaimHash)
	if err != nil {
		return err
	}
	p, err := s.loadPrepared(owned, tx, g.ID)
	if err != nil || p.source.ref != ref || p.tool != c.tool || p.contract != c.contract || p.args != c.args || p.path != c.data.Path || p.bodyHash != body || p.bytes != int64(len(c.data.Content)) || p.expected != c.data.Expected || p.input != g.InputClaimHash {
		return ErrDenied
	}
	if _, err = checkInputTx(owned, tx, g, b, effect); err != nil {
		return err
	}
	return s.provenance.CheckTx(owned, tx, p.source, "")
}

func (s *DispatchStore) loadPrepared(ctx context.Context, tx pgx.Tx, id string) (prepared, error) {
	var p prepared
	var raw, snapshot []byte
	var workspaceID string
	err := tx.QueryRow(ctx, `SELECT authorization_id,profile_id,workspace_id,tool_name,contract_hash,args_hash,source_context,path,body_sha256,body_bytes,expected_snapshot,input_claim_hash FROM code_workspace_authorizations WHERE authorization_id=$1`, id).Scan(&p.authorization, &p.profile, &workspaceID, &p.tool, &p.contract, &p.args, &raw, &p.path, &p.bodyHash, &p.bytes, &snapshot, &p.input)
	if err != nil || p.profile != s.profile.ID || s.hashes[p.tool] != p.contract || json.Unmarshal(snapshot, &p.expected) != nil {
		return p, ErrDenied
	}
	p.source, err = decodeContext(raw)
	if err != nil || p.source.ref.ID != workspaceID {
		return p, ErrDenied
	}
	return p, nil
}

// The resource row is already locked here. Sanctioned writers cannot change
// this metadata until admission commits; the storage kernel still compares
// actual physical bytes before writing. Reject stale patches before spending
// any permission or reservation from the outer admission transaction.
func checkSnapshotTx(ctx context.Context, tx pgx.Tx, p prepared) error {
	if p.tool != WriteTool {
		return nil
	}
	actual := workspace.Snapshot{Path: p.path}
	err := tx.QueryRow(ctx, `SELECT version,content_sha256,byte_length FROM code_workspace_files WHERE workspace_id=$1 AND path=$2`, p.source.ref.ID, p.path).Scan(&actual.Version, &actual.SHA256, &actual.Bytes)
	if err == pgx.ErrNoRows && p.expected.Version == 0 {
		return nil
	}
	if err != nil || actual != p.expected {
		return workspace.ErrConflict
	}
	return nil
}

func (s *DispatchStore) ClaimActionTx(ctx context.Context, tx pgx.Tx, g actionauth.Grant, effect string) error {
	if !Protected(g.Binding.ToolName) {
		return nil
	}
	if s == nil {
		return ErrDenied
	}
	p, err := s.loadPrepared(ctx, tx, g.ID)
	if err != nil || p.tool != g.Binding.ToolName || p.contract != g.Binding.ContractHash || p.args != g.Binding.ArgsHash || p.input != g.InputClaimHash {
		return ErrDenied
	}
	owned, b, deadline, stop, err := s.ownedTx(ctx, tx, g)
	if err != nil {
		return err
	}
	defer stop()
	inputDeadline, err := checkInputTx(owned, tx, g, b, effect)
	if err != nil {
		return err
	}
	if err = s.provenance.CheckTx(owned, tx, p.source, ""); err != nil {
		return err
	}
	reserved, err := s.budgets.ToolDeadlineTx(owned, tx, b.OwnerID, effect)
	if err != nil {
		return err
	}
	var at time.Time
	if tx.QueryRow(owned, `SELECT clock_timestamp()`).Scan(&at) != nil {
		return ErrDenied
	}
	for _, bound := range []time.Time{inputDeadline, reserved, at.Add(resources.MaxLeaseTTL)} {
		if bound.Before(deadline) {
			deadline = bound
		}
	}
	b.Deadline = deadline
	id := workspace.ResourceID(p.source.ref.ID)
	if err = s.resources.RegisterTx(owned, tx, b, id); err != nil {
		return err
	}
	var version int64
	if tx.QueryRow(owned, `SELECT version FROM execution_resources WHERE resource_id=$1`, id).Scan(&version) != nil {
		return ErrDenied
	}
	mode := resources.Write
	if p.tool == ReadTool {
		mode = resources.Read
	}
	proofs, err := s.resources.AcquireTx(owned, tx, b, []resources.Request{{ResourceID: id, Mode: mode, ExpectedVersion: version}}, resources.MaxLeaseTTL)
	if err != nil {
		return err
	}
	proof := proofs[0]
	if mode == resources.Write {
		if err = s.resources.ClaimTx(owned, tx, b, proof); err != nil {
			return err
		}
	}
	if tx.QueryRow(owned, `SELECT expires_at FROM execution_resource_leases WHERE lease_id=$1`, proof.LeaseID).Scan(&deadline) != nil {
		return ErrDenied
	}
	b.Deadline = deadline
	if _, err = checkInputTx(owned, tx, g, b, effect); err != nil {
		return err
	}
	if err = s.provenance.CheckTx(owned, tx, p.source, ""); err != nil {
		return err
	}
	if err = s.resources.CheckTx(owned, tx, b, proof, mode); err != nil {
		return err
	}
	if err = checkSnapshotTx(owned, tx, p); err != nil {
		return err
	}
	if reserved, err = s.budgets.ToolDeadlineTx(owned, tx, b.OwnerID, effect); err != nil || reserved.Before(deadline) {
		return ErrDenied
	}
	if _, err = checkInputTx(owned, tx, g, b, effect); err != nil {
		return err
	}
	if err = s.provenance.CheckTx(owned, tx, p.source, ""); err != nil {
		return err
	}
	var ticket [24]byte
	if _, err = rand.Read(ticket[:]); err != nil {
		return err
	}
	_, err = tx.Exec(owned, `INSERT INTO code_workspace_dispatches(ticket_id,authorization_id,effect_id,resource_id,resource_lease_id,resource_version,resource_fence,deadline) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, hex.EncodeToString(ticket[:]), g.ID, effect, id, proof.LeaseID, proof.ExpectedResourceVersion, proof.FenceEpoch, deadline)
	return err
}
