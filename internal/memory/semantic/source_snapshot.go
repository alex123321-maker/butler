package semantic

import (
	"context"
	"time"

	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

// SourceStamp binds one IPC item to current chunk content and citation revision.
// It carries no credential material and grants no source permission.
type SourceStamp struct {
	ID                  int64
	Scope               Scope
	Revision            time.Time
	Hash, EmbeddingHash string
}

func stamp(s Source) SourceStamp {
	return SourceStamp{s.ID, s.Scope, s.Revision, s.Hash(), s.EmbeddingHash()}
}

type sourceStampsKey struct{}

func withSourceStamps(ctx context.Context, sources []Source) context.Context {
	stamps := make([]SourceStamp, len(sources))
	for i, s := range sources {
		stamps[i] = stamp(s)
	}
	return context.WithValue(ctx, sourceStampsKey{}, stamps)
}
func (g *CatalogGuard) checkStampsTx(ctx context.Context, tx pgx.Tx, stamps []SourceStamp) error {
	ids := make([]int64, len(stamps))
	for i, s := range stamps {
		if s.ID <= 0 || !s.Scope.valid() || s.Revision.IsZero() || len(s.Hash) != 64 || len(s.EmbeddingHash) != 64 {
			return ErrAccess
		}
		ids[i] = s.ID
	}
	refs, e := chunkObjects(ids)
	if e != nil {
		return e
	}
	if e = g.store.CheckObjectsTx(ctx, tx, refs, g.purpose); e != nil {
		return e
	}
	for _, s := range stamps {
		current, e := scanSource(tx.QueryRow(ctx, `SELECT `+sourceColumns+` FROM memory_chunks WHERE id=$1 AND `+liveSource+` FOR SHARE`, s.ID))
		if e != nil || current.Scope != s.Scope || !current.Revision.Equal(s.Revision) || current.Hash() != s.Hash || current.EmbeddingHash() != s.EmbeddingHash || current.MemoryOrigin().Sensitivity == content.Secret {
			return ErrStale
		}
	}
	return nil
}
func (g *CatalogGuard) checkStamps(ctx context.Context, stamps []SourceStamp) error {
	tx, e := g.store.Pool().Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = g.checkStampsTx(ctx, tx, stamps); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
