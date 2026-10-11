package app

import (
	"context"
	"encoding/json"
	"reflect"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DelegationAdmission is an optional private Broker control capability. It is
// not an owner endpoint or runtime API; constructor availability enables nothing.
type DelegationAdmission struct {
	pool    *pgxpool.Pool
	leases  *session.PostgresLeaseManager
	reader  *delegationAdmissionReader
	Manager *task.DelegationManager
}
type admittedCapabilities struct {
	Version         int                      `json:"version"`
	Capabilities    DelegationCapabilities   `json:"capabilities"`
	Binding         actionauth.ActionBinding `json:"binding"`
	EffectID        string                   `json:"effect_id"`
	AuthorizationID string                   `json:"authorization_id"`
	Args            delegation.AdmissionArgs `json:"args"`
	ArgsJSON        string                   `json:"args_json"`
}
type delegationAdmissionProof struct {
	envelope delegation.AdmissionEnvelope
	grant    actionauth.Grant
	args     delegation.AdmissionArgs
}
type delegationAdmissionKey struct{}
type delegationAdmissionReader struct {
	pool    *pgxpool.Pool
	sources DelegationCatalog
	hash    string
}
type delegationAdmissionGuard struct {
	*DelegationGuards
	reader *delegationAdmissionReader
}

func NewDelegationAdmission(pool *pgxpool.Pool, store *task.Store, sessions task.DelegationSessions, inputs task.DelegationInputs, sources DelegationCatalog, limits task.DelegationLimits) (*DelegationAdmission, error) {
	if pool == nil || store == nil || nilDelegationAdapter(sources) || nilDelegationAdapter(sessions) || nilDelegationAdapter(inputs) {
		return nil, delegation.ErrDenied
	}
	hash, err := actionauth.ContractHash(delegation.AdmissionContract())
	if err != nil {
		return nil, err
	}
	reader := &delegationAdmissionReader{pool, sources, hash}
	guard, err := NewDelegationGuards(sources, reader)
	if err != nil {
		return nil, err
	}
	manager, err := task.NewDelegationManager(store, sessions, inputs, &delegationAdmissionGuard{guard, reader}, limits)
	if err != nil {
		return nil, err
	}
	return &DelegationAdmission{pool: pool, leases: session.NewPostgresLeaseManager(pool), reader: reader, Manager: manager}, nil
}

func (s *DelegationAdmission) AdmitDispatched(ctx context.Context, e delegation.AdmissionEnvelope) (delegation.AdmissionReceipt, error) {
	if s == nil || s.Manager == nil || e.RunID == "" || e.ToolCallID == "" || e.EffectID == "" || len(e.RunID) > 512 || len(e.ToolCallID) > 512 || len(e.EffectID) > 512 || e.ContractHash != s.reader.hash {
		return delegation.AdmissionReceipt{}, delegation.ErrDenied
	}
	if _, ok := owneridentity.FromContext(ctx); ok {
		return delegation.AdmissionReceipt{}, delegation.ErrDenied
	}
	args, err := delegation.DecodeAdmissionArgs(e.ArgsJSON)
	if err != nil {
		return delegation.AdmissionReceipt{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// This unlocked read derives private identity only. The manager's current
	// transaction repeats the actual claim and lease after taking its tree locks.
	proof, err := s.reader.readDispatch(ctx, s.pool, e, args, false)
	if err != nil {
		return delegation.AdmissionReceipt{}, err
	}
	var key string
	if err = s.pool.QueryRow(ctx, `SELECT session_key FROM runs WHERE run_id=$1`, e.RunID).Scan(&key); err != nil {
		return delegation.AdmissionReceipt{}, delegation.ErrDenied
	}
	token := ownership.Token{SessionKey: key, RunID: e.RunID, LeaseID: proof.grant.LeaseID, FenceEpoch: proof.grant.LeaseEpoch}
	owned := ownership.WithToken(ctx, s.leases.Authority(), token)
	owned = context.WithValue(owned, delegationAdmissionKey{}, proof)
	b := proof.grant.Binding
	value, err := s.Manager.Admit(owned, task.DelegationRequest{ParentTaskID: b.TaskID, ParentRevision: b.TaskRevision, ConstraintsRevision: b.ConstraintsRevision, ContractSource: args.ContractSource, Sources: args.Sources, Budget: args.Budget, Deadline: args.Deadline, Dependencies: args.Dependencies})
	if err != nil {
		return delegation.AdmissionReceipt{}, err
	}
	// Replays acknowledge only this actual dispatch's durable child. A new action
	// cannot claim a child's admission simply by reusing its contract source key.
	var effect, hash string
	if err = s.pool.QueryRow(ctx, `SELECT effect_id,args_hash FROM delegation_admission_snapshots WHERE task_id=$1`, value.Contract.TaskID).Scan(&effect, &hash); err != nil || effect != e.EffectID || hash != b.ArgsHash {
		return delegation.AdmissionReceipt{}, delegation.ErrDenied
	}
	return delegation.AdmissionReceipt{TaskID: value.Contract.TaskID, InputID: value.Contract.InputID, State: value.Contract.State, ContractRevision: value.Contract.Revision}, nil
}

func (r *delegationAdmissionReader) ReadDelegationCapabilitiesTx(ctx context.Context, tx pgx.Tx, parent task.Record, childID string) (DelegationCapabilities, error) {
	if childID == "" {
		p, ok := ctx.Value(delegationAdmissionKey{}).(delegationAdmissionProof)
		if !ok {
			return DelegationCapabilities{}, delegation.ErrDenied
		}
		// Current source/retention proof precedes claimed action rows. All reads
		// share the caller transaction; no service call or pool borrow occurs.
		if err := r.sources.CheckRunSourcesTx(ctx, tx, p.envelope.RunID); err != nil {
			return DelegationCapabilities{}, err
		}
		current, err := r.readDispatch(ctx, tx, p.envelope, p.args, true)
		if err != nil || current.grant.ID != p.grant.ID || current.grant.LeaseID != p.grant.LeaseID || current.grant.LeaseEpoch != p.grant.LeaseEpoch || !reflect.DeepEqual(current.grant.Binding, p.grant.Binding) {
			return DelegationCapabilities{}, delegation.ErrDenied
		}
		b := current.grant.Binding
		if err = r.currentBindingTx(ctx, tx, parent, b); err != nil {
			return DelegationCapabilities{}, err
		}
		return DelegationCapabilities{TaskID: parent.TaskID, OwnerID: parent.OwnerID, TaskRevision: parent.Revision, ConstraintsRevision: parent.ConstraintsRevision, Permissions: []delegation.Permission{}, Budget: p.args.Budget, Deadline: p.args.Deadline}, nil
	}
	snapshot, err := r.snapshotTx(ctx, tx, parent, childID)
	if err != nil {
		return DelegationCapabilities{}, err
	}
	if err = r.currentBindingTx(ctx, tx, parent, snapshot.Binding); err != nil {
		return DelegationCapabilities{}, err
	}
	return snapshot.Capabilities, nil
}

func (r *delegationAdmissionReader) currentBindingTx(ctx context.Context, tx pgx.Tx, parent task.Record, b actionauth.ActionBinding) error {
	if b.TaskID != parent.TaskID || b.OwnerID != parent.OwnerID || b.TaskRevision != parent.Revision || b.ConstraintsRevision != parent.ConstraintsRevision || b.ToolName != delegation.AdmissionTool || b.ContractHash != r.hash || b.Effect != actionauth.EffectPrepare || b.Resource != "tool:"+delegation.AdmissionTool || len(b.CredentialRefs) != 0 {
		return delegation.ErrDenied
	}
	if err := actionauth.CheckExecutionPolicy(actionauth.ContextSnapshot{ExecutionPolicy: parent.ExecutionPolicy}, b.Effect); err != nil {
		return err
	}
	at, err := delegationNow(ctx, tx)
	if err != nil {
		return err
	}
	return actionauth.CheckTaskActionTx(ctx, tx, b, at)
}

func (g *delegationAdmissionGuard) BindContractSourcesTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, c delegation.Contract) error {
	if err := g.DelegationGuards.BindContractSourcesTx(ctx, tx, parent, child, c); err != nil {
		return err
	}
	cap, err := g.reader.ReadDelegationCapabilitiesTx(ctx, tx, parent, "")
	if err != nil {
		return err
	}
	p, ok := ctx.Value(delegationAdmissionKey{}).(delegationAdmissionProof)
	if !ok || len(c.Permissions) != 0 || c.Budget != cap.Budget || !c.Deadline.Equal(cap.Deadline) || !reflect.DeepEqual(c.ContractSource, p.args.ContractSource) || !reflect.DeepEqual(c.Sources, p.args.Sources) {
		return delegation.ErrDenied
	}
	snapshot := admittedCapabilities{Version: 1, Capabilities: cap, Binding: p.grant.Binding, EffectID: p.envelope.EffectID, AuthorizationID: p.grant.ID, Args: p.args, ArgsJSON: p.envelope.ArgsJSON}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO delegation_admission_snapshots(task_id,owner_id,parent_task_id,parent_revision,parent_constraints_revision,execution_action_id,effect_id,authorization_id,action_hash,args_hash,contract_hash,accepted_json) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, child.TaskID, parent.OwnerID, parent.TaskID, parent.Revision, parent.ConstraintsRevision, p.grant.Binding.ActionID, p.envelope.EffectID, p.grant.ID, p.grant.Binding.ActionHash, p.grant.Binding.ArgsHash, g.reader.hash, raw)
	return err
}

func (r *delegationAdmissionReader) snapshotTx(ctx context.Context, tx pgx.Tx, parent task.Record, childID string) (admittedCapabilities, error) {
	var s admittedCapabilities
	var raw []byte
	var own, taskID, actionID, effectID, authID, hash, argsHash, contractHash string
	var revision, constraints int64
	err := tx.QueryRow(ctx, `SELECT owner_id,parent_task_id,parent_revision,parent_constraints_revision,execution_action_id,effect_id,authorization_id,action_hash,args_hash,contract_hash,accepted_json FROM delegation_admission_snapshots WHERE task_id=$1 FOR SHARE`, childID).Scan(&own, &taskID, &revision, &constraints, &actionID, &effectID, &authID, &hash, &argsHash, &contractHash, &raw)
	if err != nil || json.Unmarshal(raw, &s) != nil || s.Version != 1 || own != parent.OwnerID || taskID != parent.TaskID || revision != parent.Revision || constraints != parent.ConstraintsRevision || s.Binding.ActionID != actionID || s.Binding.ActionHash != hash || s.Binding.ArgsHash != argsHash || s.Binding.ContractHash != contractHash || contractHash != r.hash || s.EffectID != effectID || s.AuthorizationID != authID || len(s.Capabilities.Permissions) != 0 || s.Capabilities.TaskID != taskID || s.Capabilities.OwnerID != own || s.Capabilities.TaskRevision != revision || s.Capabilities.ConstraintsRevision != constraints || s.Capabilities.Budget != s.Args.Budget || !s.Capabilities.Deadline.Equal(s.Args.Deadline) {
		return admittedCapabilities{}, delegation.ErrDenied
	}
	args, decodeErr := delegation.DecodeAdmissionArgs(s.ArgsJSON)
	actualHash, hashErr := actionauth.ArgumentsHash(s.ArgsJSON)
	if decodeErr != nil || hashErr != nil || actualHash != argsHash || !reflect.DeepEqual(args, s.Args) || actionauth.BindCredentialMetadata(s.Binding, s.Binding.CredentialMetadataHash).ActionHash != s.Binding.ActionHash {
		return admittedCapabilities{}, delegation.ErrDenied
	}
	c, err := delegation.LoadTx(ctx, tx, childID)
	if err != nil || c == nil || c.ParentTaskID != parent.TaskID || c.ParentRevision != revision || c.ConstraintsRevision != constraints || len(c.Permissions) != 0 || c.Budget != s.Capabilities.Budget || !c.Deadline.Equal(s.Capabilities.Deadline) || c.ContractSource != s.Args.ContractSource || !reflect.DeepEqual(c.Sources, s.Args.Sources) {
		return admittedCapabilities{}, delegation.ErrDenied
	}
	return s, nil
}
