package generatedtools

import (
	"context"
	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/proto"
	"time"
)

func (s *Store) bindDeveloperAction(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, b actionauth.ActionBinding, c actionauth.ContextSnapshot) (actionauth.ActionBinding, error) {
	if !proto.Equal(contract, DeveloperAdmissionContract()) || len(call.GetCredentialRefs()) != 0 {
		return b, ErrAccess
	}
	args, err := DecodeDeveloperArgs(call.GetArgsJson())
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
	snapshot, err := ReadDeveloperPrerequisiteTx(ctx, tx, s.sources, b, args, s.profile)
	if err != nil {
		return b, err
	}
	refs, err := bindingRefs([]catalog.SourceRef{args.NeedSource, args.ContractSource})
	if err != nil {
		return b, err
	}
	b.ConverterID, b.ConverterNeedHash = snapshot.Permission.ConverterID, snapshot.NeedHash
	b, err = actionauth.BindDataSources(b, refs)
	if err != nil {
		return b, err
	}
	return b, tx.Commit(ctx)
}

func (s *Store) checkDeveloperActionTx(ctx context.Context, tx pgx.Tx, b actionauth.ActionBinding) error {
	if b.OwnerID != s.owner || len(b.DataSources) != 2 || b.ConverterPackageHash != "" || b.ConverterGeneration != 0 {
		return ErrAccess
	}
	// Args are intentionally absent from the action journal. Recover only the
	// two exact typed DATA sources below; never grant by caller-selected bodies.
	refs := []catalog.SourceRef{}
	for _, r := range b.DataSources {
		refs = append(refs, catalog.SourceRef{ID: r.ID, Revision: r.Revision, AccessRevision: r.AccessRevision, Hash: r.Hash})
	}
	var session, lease string
	var epoch int64
	if tx.QueryRow(ctx, `SELECT session_key,lease_id,fence_epoch FROM session_ownership WHERE run_id=$1 AND expires_at>clock_timestamp()`, b.RunID).Scan(&session, &lease, &epoch) != nil {
		return ErrAccess
	}
	ctx = runContext(ctx, actionauth.ContextSnapshot{RunID: b.RunID, SessionKey: session, LeaseID: lease, LeaseEpoch: epoch})
	sources, err := s.sources.AccessTx(ctx, tx, refs, "memory_read")
	if err != nil {
		return err
	}
	var n Need
	var need catalog.SourceRef
	var spec catalog.SourceRef
	for _, src := range sources {
		if parsed, e := needFrom(src); e == nil {
			n = parsed
			need = sourceRef(src)
		} else {
			spec = sourceRef(src)
		}
	}
	if need.ID == "" || spec.ID == "" || Hash(n) != b.ConverterNeedHash || n.ConverterID != b.ConverterID {
		return ErrAccess
	}
	// Claim needs current body/receipt/policy proof, not proposed allocation.
	var at time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&at) != nil {
		return ErrAccess
	}
	_, err = ReadDeveloperPrerequisiteTx(ctx, tx, s.sources, b, DeveloperArgs{NeedSource: need, ContractSource: spec, Deadline: at.Add(time.Second)}, s.profile)
	return err
}
