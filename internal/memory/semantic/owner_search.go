package semantic

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
)

// OwnerSearch is composed only with the operator-pinned local backend. The
// authenticated HTTP actor authorizes one bounded query, not a fabricated run.
// It never indexes, changes facts, starts jobs or falls back to cloud inference.
type OwnerSearch struct {
	catalog *catalog.Store
	store   *Store
	adapter *Adapter
}

type ownerQueryAdmission struct {
	budgets *budget.Controller
	catalog *catalog.Store
	target  budget.LocalTarget
}

func (a *ownerQueryAdmission) Admit(ctx context.Context, o Operation) (context.Context, func(), error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || validateOperation(o) != nil || o.Kind != "query" || o.Scope != (Scope{"user", actor.OwnerID}) {
		return nil, nil, ErrAdmission
	}
	return a.budgets.AdmitOwnerQuery(ctx, a.target, budget.LocalOperation{Kind: "query", Items: 1, InputBytes: int64(o.InputBytes), Timeout: 30 * time.Second, Origin: o.Origin}, a.catalog.CheckRestoreTx)
}
func (a *ownerQueryAdmission) CheckResult(ctx context.Context, _ Operation) error {
	return a.catalog.CheckRestore(ctx)
}

func NewOwnerSearch(c *catalog.Store, b *budget.Controller, backend Backend, target budget.LocalTarget, space Space) (*OwnerSearch, error) {
	local, ok := backend.(interface{ localTarget() budget.LocalTarget })
	if c == nil || c.Pool() == nil || b == nil || !ok || target.Identity() == "" || local.localTarget() != target || target.Endpoint() != "local-models:9090" || space.Validate() != nil {
		return nil, ErrAdmission
	}
	guard, err := NewCatalogGuard(c, "memory_read")
	if err != nil {
		return nil, err
	}
	store := NewStore(c.Pool())
	store.Guard = guard
	return &OwnerSearch{c, store, &Adapter{Backend: backend, Space: space, Admission: &ownerQueryAdmission{b, c, target}}}, nil
}

// Candidates searches up to twelve current indexed user/session scopes. A
// single query vector is shared only across the exact same configured space.
// Text search still covers nonindexed sources and other owner-visible scopes.
func (r *OwnerSearch) Candidates(ctx context.Context, query string) ([]catalog.BrowseItem, []string, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || actor.Channel != "web" {
		return nil, nil, catalog.ErrDenied
	}
	if strings.TrimSpace(query) == "" || len(query) > MaxQueryBytes || sanitize.Text(query) != query {
		return nil, nil, catalog.ErrInvalid
	}
	if r == nil {
		return nil, []string{"semantic_unavailable"}, nil
	}
	if err := r.catalog.CheckRestore(ctx); err != nil {
		return nil, nil, err
	}
	rows, err := r.store.pool.Query(ctx, `SELECT a.generation_id FROM semantic_index_active a JOIN semantic_index_generations g ON g.id=a.generation_id WHERE g.space_id=$2 AND g.state='ready' AND g.retired_at IS NULL AND ((a.scope_type='user' AND a.scope_id=$1) OR (a.scope_type='session' AND EXISTS(SELECT 1 FROM sessions s WHERE s.session_key=a.scope_id AND s.user_id=$1))) ORDER BY (a.scope_type='user') DESC,g.created_at DESC,a.generation_id LIMIT 13`, actor.OwnerID, r.adapter.Space.ID())
	if err != nil {
		return nil, nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if len(ids) == 0 {
		return nil, []string{"semantic_index_unavailable"}, nil
	}
	diagnostics := []string{"semantic_indexed_only"}
	if len(ids) > 12 {
		ids = ids[:12]
		diagnostics = append(diagnostics, "semantic_scope_limit")
	}
	vectors, _, err := r.adapter.Embeddings(WithOrigin(ctx, content.Origin{Kind: content.OwnerInput, Sensitivity: content.Private}), Scope{"user", actor.OwnerID}, []*pb.TextInput{{Id: "owner-query", Text: query}}, pb.EmbeddingTask_EMBEDDING_TASK_QUERY)
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, ctx.Err()
		}
		if errors.Is(err, ErrUnavailable) {
			return nil, []string{"embedding_unavailable"}, nil
		}
		var exhausted *budget.ExhaustedError
		if errors.As(err, &exhausted) {
			return nil, []string{"semantic_budget_exhausted"}, nil
		}
		return nil, nil, err
	}
	hits := []Hit{}
	for _, id := range ids {
		generation, e := r.store.Generation(ctx, id)
		if e != nil {
			return nil, nil, e
		}
		if generation.Space.ID() != r.adapter.Space.ID() {
			return nil, nil, ErrStale
		}
		found, e := r.store.Search(ctx, generation, vectors[0], 4)
		if e != nil {
			return nil, nil, e
		}
		current, e := r.store.Active(ctx, generation.Scope)
		if e != nil || current.ID != id {
			return nil, nil, ErrStale
		}
		hits = append(hits, found...)
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Distance != hits[j].Distance {
			return hits[i].Distance < hits[j].Distance
		}
		return hits[i].Source.ID < hits[j].Source.ID
	})
	refs := []catalog.ChunkPreviewRef{}
	seen := map[int64]bool{}
	for _, h := range hits {
		if !seen[h.Source.ID] {
			seen[h.Source.ID] = true
			refs = append(refs, catalog.ChunkPreviewRef{ID: h.Source.ID, Revision: h.Source.Revision})
		}
		if len(refs) == 4 {
			break
		}
	}
	items, err := r.catalog.BrowseChunks(ctx, refs)
	return items, diagnostics, err
}
