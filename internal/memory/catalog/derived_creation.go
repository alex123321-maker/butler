package catalog

import (
	"context"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

// Shared constructor receipt and validator extracted from #88 / 0db6694d.
// Top transaction identity deliberately survives legitimate savepoints; row
// xmin would refer to a subtransaction and is not a substitute for this proof.
type derivedCreation struct {
	xid                       string
	source                    SourceRef
	owner, scopeType, scopeID string
}

func (s *Store) validateNewDerivedTx(ctx context.Context, tx pgx.Tx, derived Source, parents []SourceRef, purpose string) error {
	if tx == nil || len(parents) == 0 || len(parents) > 64 || derived.Kind != "model_candidate" || derived.Revision != 1 || derived.AccessRevision != 1 || (derived.Status != "active" && derived.Status != "quarantined") {
		return ErrInvalid
	}
	for _, p := range parents {
		if p.ID == "" || p.Revision < 1 || p.AccessRevision < 1 || len(p.Hash) != 64 {
			return ErrInvalid
		}
	}
	if derived.creation == nil || derived.creation.source != (SourceRef{derived.ID, derived.Revision, derived.AccessRevision, derived.Hash}) || derived.creation.owner != derived.OwnerID || derived.creation.scopeType != derived.ScopeType || derived.creation.scopeID != derived.ScopeID {
		return ErrDenied
	}
	current, err := s.AccessTx(ctx, tx, parents, purpose)
	if err != nil {
		return err
	}
	byID := map[string]Source{}
	for _, src := range current {
		byID[src.ID] = src
	}
	anchor, ok := byID[parents[0].ID]
	if !ok || derived.OwnerID != anchor.OwnerID || derived.ScopeType != anchor.ScopeType || derived.ScopeID != anchor.ScopeID {
		return ErrDenied
	}
	var fresh bool
	err = tx.QueryRow(ctx, `SELECT pg_current_xact_id()::text=$7 AND s.created_at>=transaction_timestamp() AND v.extracted_at>=transaction_timestamp() AND s.source_kind='model_candidate' AND s.external_key='derived:'||s.source_id AND s.head_revision=1 AND s.access_revision=1 AND s.status=$2 AND s.owner_id=$3 AND s.scope_type=$4 AND s.scope_id=$5 AND v.content_hash=$6 FROM memory_sources s JOIN memory_source_revisions v ON v.source_id=s.source_id AND v.revision=1 WHERE s.source_id=$1`, derived.ID, derived.Status, derived.OwnerID, derived.ScopeType, derived.ScopeID, derived.Hash, derived.creation.xid).Scan(&fresh)
	if err != nil {
		return err
	}
	if !fresh {
		return ErrDenied
	}
	var exact bool
	err = tx.QueryRow(ctx, `WITH p AS(SELECT * FROM jsonb_to_recordset($2::jsonb) AS x(source_id text,revision bigint,access_revision bigint,content_hash text)),d AS(SELECT parent_source_id,parent_revision FROM memory_source_dependencies WHERE child_source_id=$1) SELECT (SELECT count(*) FROM d)=(SELECT count(*) FROM p) AND NOT EXISTS(SELECT 1 FROM d LEFT JOIN p ON p.source_id=d.parent_source_id AND p.revision=d.parent_revision WHERE p.source_id IS NULL)`, derived.ID, mustJSON(parents)).Scan(&exact)
	if err != nil {
		return err
	}
	if !exact {
		return ErrStale
	}
	return nil
}

type freshPublicationChecker struct {
	store   *Store
	base    AccessChecker
	source  Source
	parents []SourceRef
	purpose string
}

func (c *freshPublicationChecker) PinnedMemorySources() []SourceRef {
	return c.base.PinnedMemorySources()
}
func (c *freshPublicationChecker) originalRefs(refs []SourceRef) ([]SourceRef, error) {
	out := []SourceRef{}
	for _, r := range refs {
		if r.ID != c.source.ID {
			out = append(out, r)
			continue
		}
		if r.Revision != 0 && r.Revision != c.source.Revision || r.AccessRevision != 0 && r.AccessRevision != c.source.AccessRevision || r.Hash != "" && r.Hash != c.source.Hash {
			return nil, ErrStale
		}
	}
	return out, nil
}
func (c *freshPublicationChecker) CheckMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []SourceRef, purpose string) (string, error) {
	refs, err := c.originalRefs(refs)
	if err != nil {
		return "", err
	}
	original := WithAccessChecker(ctx, c.base)
	if err = c.store.validateNewDerivedTx(original, tx, c.source, c.parents, c.purpose); err != nil {
		return "", err
	}
	return c.base.CheckMemoryAccessTx(original, tx, refs, purpose)
}
func (c *freshPublicationChecker) FinalizeMemoryAccessTx(ctx context.Context, tx pgx.Tx, refs []SourceRef, purpose string) error {
	refs, err := c.originalRefs(refs)
	if err != nil {
		return err
	}
	original := WithAccessChecker(ctx, c.base)
	if err = c.store.validateNewDerivedTx(original, tx, c.source, c.parents, c.purpose); err != nil {
		return err
	}
	return c.base.FinalizeMemoryAccessTx(original, tx, refs, purpose)
}

// WithFreshPublicationSourceTx permits the constructor-issued output only inside
// its trusted publication transaction. It cannot bless a ref, reloaded source or
// another transaction. All original job Check/Finalize/Pinned proof remains
// mandatory. No owner actor, external read authority or durable capability is
// minted; the caller still commits its ordinary result/claim CAS atomically.
func (s *Store) WithFreshPublicationSourceTx(ctx context.Context, tx pgx.Tx, fresh Source, parents []SourceRef, purpose string) (context.Context, error) {
	if _, owner := owneridentity.FromContext(ctx); owner {
		return nil, ErrDenied
	}
	base, ok := ctx.Value(checkerKey{}).(AccessChecker)
	if !ok || base == nil {
		return nil, ErrDenied
	}
	if _, nested := base.(*freshPublicationChecker); nested {
		return nil, ErrDenied
	}
	if err := s.validateNewDerivedTx(ctx, tx, fresh, parents, purpose); err != nil {
		return nil, err
	}
	return WithAccessChecker(ctx, &freshPublicationChecker{store: s, base: base, source: fresh, parents: append([]SourceRef{}, parents...), purpose: purpose}), nil
}
