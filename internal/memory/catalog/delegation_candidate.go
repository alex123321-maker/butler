package catalog

import (
	"context"
	"fmt"

	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// GrantDelegatedCandidateTx grants one exact child-task DATA body to the actual
// current parent worker. The trusted technical-return checker must validate the
// accepted role, immutable final/report and tested package on this transaction.
// No child session/final/report body is granted, and selected parent roots keep
// their existing independent rights. It cannot run as owner or auxiliary job.
func (s *Store) GrantDelegatedCandidateTx(ctx context.Context, tx pgx.Tx, childID string, candidate SourceRef, selected []SourceRef, checker AccessChecker) ([]Source, error) {
	token, owned := ownership.TokenFromContext(ctx)
	_, actor := owneridentity.FromContext(ctx)
	_, aux := ctx.Value(checkerKey{}).(AccessChecker)
	if tx == nil || !owned || actor || aux || checker == nil || len(selected) != 2 || childID == "" || !delegation.ValidSource(delegation.SourceRef(candidate)) {
		return nil, ErrDenied
	}
	publication, err := tx.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer publication.Rollback(ctx)
	if _, err = s.AccessTx(ctx, publication, nil, "memory_read"); err != nil {
		return nil, fmt.Errorf("current parent access: %w", err)
	}
	var parentID, own string
	var revision int64
	if publication.QueryRow(ctx, `SELECT t.task_id,t.owner_id,t.revision FROM runs r JOIN tasks t ON t.task_id=r.task_id JOIN sessions s ON s.session_key=r.session_key WHERE r.run_id=$1 AND r.session_key=$2 AND t.owner_id=s.user_id AND t.revision=r.task_revision AND t.status='active'`, token.RunID, token.SessionKey).Scan(&parentID, &own, &revision) != nil {
		return nil, fmt.Errorf("current parent identity: %w", ErrDenied)
	}
	c, err := delegation.LoadTx(ctx, publication, childID)
	if err != nil || c == nil || c.ParentTaskID != parentID || c.State != "result_ready" || c.ParentRevision != revision || !sameDelegationRoots(c, selected) {
		return nil, fmt.Errorf("exact accepted child roots: %w", ErrDenied)
	}
	if err = s.CheckDelegationSourcesTx(ctx, publication, childID, selected, "memory_read"); err != nil {
		return nil, fmt.Errorf("current selected child roots: %w", err)
	}
	requested := append([]SourceRef{candidate}, selected...)
	rows, err := s.AccessTx(WithAccessChecker(ctx, checker), publication, requested, "memory_read")
	if err != nil || len(rows) != 3 {
		return nil, fmt.Errorf("technical return access: %w", ErrDenied)
	}
	var body Source
	for _, src := range rows {
		if src.ID == candidate.ID {
			body = src
		}
	}
	if body.ScopeType != "task" || body.ScopeID != childID || body.OwnerID != own {
		return nil, fmt.Errorf("candidate child scope: %w", ErrDenied)
	}
	graph, err := s.CanonicalSourcesTx(ctx, publication, []SourceRef{candidate})
	if err != nil {
		return nil, err
	}
	roots, err := s.CanonicalSourcesTx(ctx, publication, selected)
	if err != nil || len(graph) != len(roots)+1 {
		return nil, fmt.Errorf("candidate graph width: %w", ErrDenied)
	}
	want := map[string]SourceRef{candidate.ID: candidate}
	for _, r := range roots {
		want[r.ID] = r
	}
	for _, r := range graph {
		if expected, ok := want[r.ID]; !ok || expected.Revision != r.Revision {
			return nil, fmt.Errorf("candidate graph identity: %w", ErrDenied)
		}
		if r == candidate {
			continue
		}
		src, e := getSourceTx(ctx, publication, r.ID, false)
		if e != nil || checkInheritedPermissionTx(ctx, publication, parentID, own, src, "memory_read", 0) != nil {
			return nil, fmt.Errorf("retained parent rights: %w", ErrDenied)
		}
	}
	if _, err = publication.Exec(ctx, `INSERT INTO memory_scope_grants(task_id,source_id,task_revision,access_revision,purpose,owner_id) VALUES($1,$2,$3,$4,'memory_read',$5) ON CONFLICT(task_id,source_id) DO NOTHING`, parentID, candidate.ID, revision, candidate.AccessRevision, own); err != nil {
		return nil, err
	}
	if err = checkExactGrantTx(ctx, publication, parentID, own, body, "memory_read"); err != nil {
		return nil, fmt.Errorf("current candidate grant: %w", err)
	}
	// Normal worker access repeats the live fence and pins the complete graph.
	// The privileged checker is deliberately absent from this final access.
	rows, err = s.AccessTx(ctx, publication, requested, "memory_read")
	if err != nil {
		return nil, fmt.Errorf("ordinary parent access: %w", err)
	}
	if err = publication.Commit(ctx); err != nil {
		return nil, err
	}
	return rows, nil
}
