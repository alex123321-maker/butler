package semantic

import (
	"context"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

// CatalogGuard reads authoritative chunk/source links on the caller transaction.
// Optional Source.Catalog* citation metadata is deliberately never consulted.
type CatalogGuard struct {
	store   *catalog.Store
	purpose string
}

func NewCatalogGuard(store *catalog.Store, purpose string) (*CatalogGuard, error) {
	if store == nil || store.Pool() == nil || purpose == "" || len(purpose) > 512 {
		return nil, ErrAccess
	}
	return &CatalogGuard{store, purpose}, nil
}
func sourceObjects(sources []Source) ([]catalog.ObjectRef, error) {
	refs := make([]catalog.ObjectRef, 0, len(sources))
	seen := map[int64]bool{}
	for _, src := range sources {
		if src.ID <= 0 || src.MemoryOrigin().Sensitivity == content.Secret {
			return nil, ErrAccess
		}
		if !seen[src.ID] {
			refs = append(refs, catalog.ObjectRef{Type: "chunk", ID: src.ID})
			seen[src.ID] = true
		}
	}
	return refs, nil
}
func chunkObjects(ids []int64) ([]catalog.ObjectRef, error) {
	refs := make([]catalog.ObjectRef, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 {
			return nil, ErrAccess
		}
		if !seen[id] {
			refs = append(refs, catalog.ObjectRef{Type: "chunk", ID: id})
			seen[id] = true
		}
	}
	return refs, nil
}
func (g *CatalogGuard) CheckSources(ctx context.Context, sources []Source) error {
	if g == nil || g.store == nil {
		return ErrAccess
	}
	_, e := sourceObjects(sources)
	if e != nil {
		return e
	}
	stamps := make([]SourceStamp, len(sources))
	for i, s := range sources {
		stamps[i] = stamp(s)
	}
	return g.checkStamps(ctx, stamps)
}
func (g *CatalogGuard) CheckSourcesTx(ctx context.Context, tx pgx.Tx, sources []Source) error {
	if g == nil || g.store == nil || tx == nil {
		return ErrAccess
	}
	_, e := sourceObjects(sources)
	if e != nil {
		return e
	}
	stamps := make([]SourceStamp, len(sources))
	for i, s := range sources {
		stamps[i] = stamp(s)
	}
	return g.checkStampsTx(ctx, tx, stamps)
}
