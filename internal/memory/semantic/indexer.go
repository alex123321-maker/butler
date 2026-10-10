package semantic

import (
	"context"
	"strconv"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/security/content"
)

// Rebuild resumes a caller-owned durable generation after interruption. Each
// complete source snapshot is persisted only if its authoritative revision still
// matches. This is not a task scheduler and never claims a completed run lease.
func Rebuild(ctx context.Context, store *Store, adapter *Adapter, id string, maxSources int) ([]Stats, error) {
	if store == nil || adapter == nil || adapter.Admission == nil {
		return nil, ErrAdmission
	}
	g, err := store.Generation(ctx, id)
	if err != nil {
		return nil, err
	}
	if g.State != "building" || g.Space.ID() != adapter.Space.ID() {
		return nil, ErrStale
	}
	sources, err := store.Sources(ctx, g.Scope, maxSources)
	if err != nil {
		return nil, err
	}
	var measures []Stats
	var pending []Source
	bytes := 0
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		texts := make([]*pb.TextInput, len(pending))
		for i, src := range pending {
			texts[i] = &pb.TextInput{Id: strconv.FormatInt(src.ID, 10), Title: src.Title, Text: src.Content}
		}
		origins := make([]content.Origin, len(pending))
		for i, src := range pending {
			origins[i] = src.MemoryOrigin()
		}
		vectors, stats, err := adapter.Embeddings(withSourceStamps(WithOrigin(ctx, content.Derive(content.MemoryDerived, origins...)), pending), g.Scope, texts, pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT)
		if err != nil {
			return err
		}
		for i, src := range pending {
			if err := store.Put(ctx, id, src, vectors[i]); err != nil {
				return err
			}
		}
		measures = append(measures, stats)
		pending = nil
		bytes = 0
		return nil
	}
	for _, src := range sources {
		if ctx.Err() != nil {
			return measures, ctx.Err()
		}
		found, err := store.HasEntry(ctx, id, src)
		if err != nil {
			return measures, err
		}
		if found {
			continue
		}
		reused, err := store.Reuse(ctx, g, src)
		if err != nil {
			return measures, err
		}
		if reused {
			continue
		}
		n := len(src.Title) + len(src.Content)
		if n > 8192 {
			return measures, ErrInvalid
		}
		if len(pending) == 4 || bytes+n > 8192 {
			if err := flush(); err != nil {
				return measures, err
			}
		}
		pending = append(pending, src)
		bytes += n
	}
	if err := flush(); err != nil {
		return measures, err
	}
	return measures, nil
}
