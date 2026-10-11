package generatedtools

import (
	"context"
	"fmt"
	"reflect"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
)

type collectionArgs struct {
	ChildTaskID string `json:"child_task_id"`
	Digest      string `json:"digest"`
}

func decodeCollectionArgs(raw string) (collectionArgs, error) {
	var a collectionArgs
	if len(raw) > 4096 || Decode([]byte(raw), &a) != nil || a.ChildTaskID == "" || len(a.ChildTaskID) > 512 || a.Digest != "" && !digestName.MatchString(a.Digest) {
		return a, ErrPackage
	}
	return a, nil
}

type collectionAccess struct {
	store      *Store
	current    actionauth.ContextSnapshot
	snapshot   DeveloperSnapshot
	projection DeveloperReturn
}

func (c collectionAccess) CheckMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []catalog.SourceRef, purpose string) (string, error) {
	if purpose != "memory_read" || !reflect.DeepEqual(refs, developerReturnSources(c.projection)) {
		return "", ErrAccess
	}
	if _, _, err := c.store.guardRunTx(ctx, tx, c.current); err != nil {
		return "", err
	}
	s, err := ReadDeveloperSnapshotTx(ctx, tx, c.store.sources, c.current.OwnerID, c.snapshot.ChildTaskID)
	if err != nil || s == nil || Hash(*s) != Hash(c.snapshot) {
		return "", ErrAccess
	}
	p, err := ReadDeveloperReturnTx(ctx, tx, *s)
	if err != nil || Hash(p) != Hash(c.projection) {
		return "", ErrAccess
	}
	return c.current.OwnerID, nil
}
func (c collectionAccess) FinalizeMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []catalog.SourceRef, purpose string) error {
	_, err := c.CheckMemoryAccessTx(ctx, tx, refs, purpose)
	return err
}
func (c collectionAccess) PinnedMemorySources() []catalog.SourceRef {
	return []catalog.SourceRef{c.projection.ReportSource, c.projection.FinalSource}
}

func (s *Store) collectionTx(ctx context.Context, tx pgx.Tx, current actionauth.ContextSnapshot, childID, digest string) (DeveloperReturn, collectionAccess, error) {
	snapshot, err := ReadDeveloperSnapshotTx(ctx, tx, s.sources, current.OwnerID, childID)
	if err != nil || snapshot == nil || snapshot.Binding.TaskID != current.TaskID || snapshot.Binding.TaskRevision != current.TaskRevision || snapshot.Binding.ConstraintsRevision != current.ConstraintsRevision || snapshot.Permission.RuntimePin != Hash(s.profile) {
		return DeveloperReturn{}, collectionAccess{}, ErrAccess
	}
	p, err := ReadDeveloperReturnTx(ctx, tx, *snapshot)
	if err != nil || digest != "" && p.Candidate.Digest != digest {
		return p, collectionAccess{}, ErrAccess
	}
	currentPackage, err := s.metadataTx(ctx, tx, p.Candidate.Digest)
	if err != nil || Hash(currentPackage) != Hash(p.Candidate) {
		return p, collectionAccess{}, ErrAccess
	}
	checker := collectionAccess{s, current, *snapshot, p}
	rows, err := s.sources.AccessTx(catalog.WithAccessChecker(ctx, checker), tx, developerReturnSources(p), "memory_read")
	if err != nil || len(rows) != 3 {
		return p, checker, ErrAccess
	}
	byID := sourcesByID(rows)
	n, err := needFrom(byID[p.Candidate.NeedSource.ID])
	if err != nil || Hash(n) != snapshot.NeedHash {
		return p, checker, ErrAccess
	}
	code, err := Parse([]byte(byID[p.Candidate.BodySource.ID].Body))
	if err != nil || code.Digest != p.Candidate.Digest || code.Manifest.Version != snapshot.Spec.Version || code.Manifest.Dependencies[0] != s.profile.Dependency || byID[p.Candidate.BodySource.ID].ScopeType != "task" || byID[p.Candidate.BodySource.ID].ScopeID != childID {
		return p, checker, ErrAccess
	}
	return p, checker, nil
}

func (s *Store) bindCollectionAction(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, b actionauth.ActionBinding, c actionauth.ContextSnapshot) (actionauth.ActionBinding, error) {
	if !staticContract(contract) || contract.ToolName != "converters.collect" || len(call.GetCredentialRefs()) != 0 {
		return b, ErrAccess
	}
	args, err := decodeCollectionArgs(call.GetArgsJson())
	if err != nil {
		return b, err
	}
	current, err := s.run(ctx, call.GetRunId())
	if err != nil || current.TaskID != c.TaskID || current.TaskRevision != c.TaskRevision || current.ConstraintsRevision != c.ConstraintsRevision || current.LeaseID != c.LeaseID || current.LeaseEpoch != c.LeaseEpoch {
		return b, ErrAccess
	}
	ctx = runContext(ctx, current)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return b, err
	}
	defer tx.Rollback(ctx)
	if _, _, err = s.guardRunTx(ctx, tx, current); err != nil {
		return b, err
	}
	p, _, err := s.collectionTx(ctx, tx, current, args.ChildTaskID, args.Digest)
	if err != nil {
		return b, err
	}
	refs, err := bindingRefs(developerReturnSources(p))
	if err != nil {
		return b, err
	}
	b.ConverterID, b.ConverterPackageHash, b.ConverterNeedHash, b.ConverterGeneration = p.Candidate.ID, p.Candidate.Digest, p.Candidate.NeedHash, p.Candidate.Generation
	b, err = actionauth.BindDataSources(b, refs)
	if err != nil {
		return b, err
	}
	return b, tx.Commit(ctx)
}

func (s *Store) checkCollectionActionTx(ctx context.Context, tx pgx.Tx, b actionauth.ActionBinding) error {
	var childID string
	if b.OwnerID != s.owner || len(b.DataSources) != 3 || b.Effect != actionauth.EffectRead || b.Resource != "tool:converters.collect" || len(b.CredentialRefs) != 0 || tx.QueryRow(ctx, `SELECT child_task_id FROM converter_developer_returns WHERE owner_id=$1 AND parent_task_id=$2 AND digest=$3`, s.owner, b.TaskID, b.ConverterPackageHash).Scan(&childID) != nil {
		return ErrAccess
	}
	var c actionauth.ContextSnapshot
	c.RunID, c.OwnerID, c.TaskID, c.TaskRevision, c.ConstraintsRevision = b.RunID, b.OwnerID, b.TaskID, b.TaskRevision, b.ConstraintsRevision
	if tx.QueryRow(ctx, `SELECT o.session_key,o.lease_id,o.fence_epoch,r.current_state,t.execution_policy FROM session_ownership o JOIN runs r ON r.run_id=o.run_id JOIN tasks t ON t.task_id=r.task_id WHERE o.run_id=$1 AND o.expires_at>clock_timestamp()`, b.RunID).Scan(&c.SessionKey, &c.LeaseID, &c.LeaseEpoch, &c.RunState, &c.ExecutionPolicy) != nil {
		return ErrAccess
	}
	ctx = runContext(ctx, c)
	if _, _, err := s.guardRunTx(ctx, tx, c); err != nil {
		return err
	}
	p, _, err := s.collectionTx(ctx, tx, c, childID, b.ConverterPackageHash)
	if err != nil || p.Candidate.ID != b.ConverterID || p.Candidate.NeedHash != b.ConverterNeedHash || p.Candidate.Generation != b.ConverterGeneration {
		return ErrAccess
	}
	refs, err := bindingRefs(developerReturnSources(p))
	canonical, bindErr := actionauth.BindDataSources(actionauth.ActionBinding{}, refs)
	if err != nil || bindErr != nil || !reflect.DeepEqual(canonical.DataSources, b.DataSources) {
		return ErrAccess
	}
	return nil
}

// Collect is an ordinary claimed parent tool action. A technical return grants
// only the candidate graph; checker/trial/activation require their own actions.
func (s *Store) Collect(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, childID, digest string) (Record, error) {
	args, err := decodeCollectionArgs(call.GetArgsJson())
	if err != nil || args.ChildTaskID != childID || args.Digest != digest || contract == nil || contract.ToolName != "converters.collect" || !staticContract(contract) {
		return Record{}, ErrAccess
	}
	c, err := s.run(ctx, call.GetRunId())
	if err != nil {
		return Record{}, err
	}
	ctx = runContext(ctx, c)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(ctx)
	if _, _, err = s.guardRunTx(ctx, tx, c); err != nil {
		return Record{}, err
	}
	p, checker, err := s.collectionTx(ctx, tx, c, childID, digest)
	if err != nil {
		return Record{}, err
	}
	if err = s.dispatchTx(ctx, tx, c, call, contract); err != nil {
		return Record{}, fmt.Errorf("current collection dispatch: %w", err)
	}
	if _, err = s.sources.GrantDelegatedCandidateTx(ctx, tx, childID, p.Candidate.BodySource, []catalog.SourceRef{p.Candidate.NeedSource, p.ContractSource}, checker); err != nil {
		return Record{}, fmt.Errorf("exact candidate grant: %w", err)
	}
	if err = checker.FinalizeMemoryAccessTx(ctx, tx, developerReturnSources(p), "memory_read"); err != nil {
		return Record{}, err
	}
	return p.Candidate, tx.Commit(ctx)
}
