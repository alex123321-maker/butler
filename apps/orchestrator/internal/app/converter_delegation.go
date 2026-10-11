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
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A distinct optional admission component; context-only task.delegate and its
// empty-permission snapshot are unchanged. Root composition enables neither here.
type ConverterDelegation struct {
	pool    *pgxpool.Pool
	leases  *session.PostgresLeaseManager
	reader  *converterDelegationReader
	Manager *task.DelegationManager
}
type converterDelegationProof struct {
	envelope delegation.AdmissionEnvelope
	grant    actionauth.Grant
	args     generatedtools.DeveloperArgs
}
type converterDelegationKey struct{}
type converterDelegationReader struct {
	sources       DelegationCatalog
	profile       generatedtools.Profile
	hash          string
	continuations bool
}
type converterDelegationGuard struct {
	*DelegationGuards
	reader *converterDelegationReader
}

func NewConverterDelegation(pool *pgxpool.Pool, store *task.Store, sessions task.DelegationSessions, inputs task.DelegationInputs, sources DelegationCatalog, limits task.DelegationLimits, profile generatedtools.Profile) (*ConverterDelegation, error) {
	if pool == nil || store == nil || nilDelegationAdapter(sources) || nilDelegationAdapter(sessions) || nilDelegationAdapter(inputs) || profile.Validate() != nil {
		return nil, delegation.ErrDenied
	}
	hash, err := actionauth.ContractHash(generatedtools.DeveloperAdmissionContract())
	if err != nil {
		return nil, err
	}
	reader := &converterDelegationReader{sources: sources, profile: profile, hash: hash}
	guard, err := NewDelegationGuards(sources, reader)
	if err != nil {
		return nil, err
	}
	manager, err := task.NewDelegationManager(store, sessions, inputs, &converterDelegationGuard{guard, reader}, limits)
	if err != nil {
		return nil, err
	}
	return &ConverterDelegation{pool: pool, leases: session.NewPostgresLeaseManager(pool), reader: reader, Manager: manager}, nil
}

func (s *ConverterDelegation) AdmitDispatched(ctx context.Context, e delegation.AdmissionEnvelope) (delegation.AdmissionReceipt, error) {
	if s == nil || s.Manager == nil || e.RunID == "" || e.ToolCallID == "" || e.EffectID == "" || len(e.RunID) > 512 || len(e.ToolCallID) > 512 || len(e.EffectID) > 512 || e.ContractHash != s.reader.hash {
		return delegation.AdmissionReceipt{}, delegation.ErrDenied
	}
	if _, actor := owneridentity.FromContext(ctx); actor {
		return delegation.AdmissionReceipt{}, delegation.ErrDenied
	}
	args, err := generatedtools.DecodeDeveloperArgs(e.ArgsJSON)
	if err != nil {
		return delegation.AdmissionReceipt{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	grant, err := readAdmissionDispatch(ctx, s.pool, e, generatedtools.DeveloperAdmissionTool, s.reader.hash, false)
	if err != nil {
		return delegation.AdmissionReceipt{}, err
	}
	var key string
	if s.pool.QueryRow(ctx, `SELECT session_key FROM runs WHERE run_id=$1`, e.RunID).Scan(&key) != nil {
		return delegation.AdmissionReceipt{}, delegation.ErrDenied
	}
	owned := ownership.WithToken(ctx, s.leases.Authority(), ownership.Token{SessionKey: key, RunID: e.RunID, LeaseID: grant.LeaseID, FenceEpoch: grant.LeaseEpoch})
	owned = context.WithValue(owned, converterDelegationKey{}, converterDelegationProof{e, grant, args})
	permissions, err := generatedtools.DeveloperPermissions()
	if err != nil {
		return delegation.AdmissionReceipt{}, err
	}
	b := grant.Binding
	value, err := s.Manager.Admit(owned, task.DelegationRequest{ParentTaskID: b.TaskID, ParentRevision: b.TaskRevision, ConstraintsRevision: b.ConstraintsRevision, ContractSource: delegation.SourceRef(args.ContractSource), Sources: []delegation.SourceRef{delegation.SourceRef(args.NeedSource)}, Permissions: permissions, Budget: delegation.Limits(args.Budget), Deadline: args.Deadline, Dependencies: args.Dependencies})
	if err != nil {
		return delegation.AdmissionReceipt{}, err
	}
	var effect, hash string
	if s.pool.QueryRow(ctx, `SELECT effect_id,args_hash FROM converter_developer_snapshots WHERE task_id=$1`, value.Contract.TaskID).Scan(&effect, &hash) != nil || effect != e.EffectID || hash != b.ArgsHash {
		return delegation.AdmissionReceipt{}, delegation.ErrDenied
	}
	return delegation.AdmissionReceipt{TaskID: value.Contract.TaskID, InputID: value.Contract.InputID, State: value.Contract.State, ContractRevision: value.Contract.Revision}, nil
}

func (r *converterDelegationReader) proposalTx(ctx context.Context, tx pgx.Tx, parent task.Record) (generatedtools.DeveloperSnapshot, error) {
	p, ok := ctx.Value(converterDelegationKey{}).(converterDelegationProof)
	if !ok {
		return generatedtools.DeveloperSnapshot{}, delegation.ErrDenied
	}
	if err := r.sources.CheckRunSourcesTx(ctx, tx, p.envelope.RunID); err != nil {
		return generatedtools.DeveloperSnapshot{}, err
	}
	b := p.grant.Binding
	if b.TaskID != parent.TaskID || b.OwnerID != parent.OwnerID || b.TaskRevision != parent.Revision || b.ConstraintsRevision != parent.ConstraintsRevision {
		return generatedtools.DeveloperSnapshot{}, delegation.ErrDenied
	}
	snapshot, err := generatedtools.ReadDeveloperPrerequisiteTx(ctx, tx, r.sources, b, p.args, r.profile)
	if err != nil {
		return snapshot, err
	}
	if b.ConverterID != snapshot.Permission.ConverterID || b.ConverterNeedHash != snapshot.NeedHash || b.ConverterPackageHash != "" || b.ConverterGeneration != 0 || len(b.DataSources) != 2 {
		return snapshot, delegation.ErrDenied
	}
	for _, ref := range []catalog.SourceRef{p.args.NeedSource, p.args.ContractSource} {
		found := false
		for _, r := range b.DataSources {
			if r.ID == ref.ID && r.Revision == ref.Revision && r.AccessRevision == ref.AccessRevision && r.Hash == ref.Hash {
				found = true
			}
		}
		if !found {
			return snapshot, delegation.ErrDenied
		}
	}
	current, err := readAdmissionDispatch(ctx, tx, p.envelope, generatedtools.DeveloperAdmissionTool, r.hash, true)
	if err != nil || current.ID != p.grant.ID || !reflect.DeepEqual(current, p.grant) {
		return generatedtools.DeveloperSnapshot{}, delegation.ErrDenied
	}
	snapshot.EffectID, snapshot.AuthorizationID, snapshot.ArgsJSON = p.envelope.EffectID, p.grant.ID, p.envelope.ArgsJSON
	return snapshot, nil
}

func (r *converterDelegationReader) ReadDelegationCapabilitiesTx(ctx context.Context, tx pgx.Tx, parent task.Record, childID string) (DelegationCapabilities, error) {
	var snapshot *generatedtools.DeveloperSnapshot
	if childID == "" {
		value, err := r.proposalTx(ctx, tx, parent)
		if err != nil {
			return DelegationCapabilities{}, err
		}
		snapshot = &value
	} else {
		value, err := generatedtools.ReadDeveloperSnapshotTx(ctx, tx, r.sources, parent.OwnerID, childID)
		if err != nil || value == nil {
			return DelegationCapabilities{}, delegation.ErrDenied
		}
		snapshot = value
	}
	b := snapshot.Binding
	if b.TaskID != parent.TaskID || b.OwnerID != parent.OwnerID || b.TaskRevision != parent.Revision || b.ConstraintsRevision != parent.ConstraintsRevision {
		return DelegationCapabilities{}, delegation.ErrDenied
	}
	return DelegationCapabilities{TaskID: parent.TaskID, OwnerID: parent.OwnerID, TaskRevision: parent.Revision, ConstraintsRevision: parent.ConstraintsRevision, Permissions: snapshot.Permissions, Budget: delegation.Limits(snapshot.Args.Budget), Deadline: snapshot.Args.Deadline}, nil
}

func (g *converterDelegationGuard) BindContractSourcesTx(ctx context.Context, tx pgx.Tx, parent, child task.Record, c delegation.Contract) error {
	if err := g.DelegationGuards.BindContractSourcesTx(ctx, tx, parent, child, c); err != nil {
		return err
	}
	snapshot, err := g.reader.proposalTx(ctx, tx, parent)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(c.Permissions, snapshot.Permissions) || c.Budget != delegation.Limits(snapshot.Args.Budget) || !c.Deadline.Equal(snapshot.Args.Deadline) || c.ContractSource != delegation.SourceRef(snapshot.Args.ContractSource) || len(c.Sources) != 1 || c.Sources[0] != delegation.SourceRef(snapshot.Args.NeedSource) {
		return delegation.ErrDenied
	}
	snapshot.ChildTaskID = child.TaskID
	snapshot.ChildTaskRevision, snapshot.ChildConstraintsRevision = child.Revision, child.ConstraintsRevision
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	b := snapshot.Binding
	_, err = tx.Exec(ctx, `INSERT INTO converter_developer_snapshots(task_id,owner_id,parent_task_id,parent_revision,parent_constraints_revision,execution_action_id,effect_id,authorization_id,action_hash,args_hash,contract_hash,snapshot_hash,accepted_json) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, child.TaskID, parent.OwnerID, parent.TaskID, parent.Revision, parent.ConstraintsRevision, b.ActionID, snapshot.EffectID, snapshot.AuthorizationID, b.ActionHash, b.ArgsHash, g.reader.hash, generatedtools.Hash(snapshot), raw)
	if err == nil && g.reader.continuations {
		err = generatedtools.WriteContinuationIntentsTx(ctx, tx, snapshot)
	}
	return err
}
