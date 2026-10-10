package semantic

import (
	"context"
	"sort"
	"strconv"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/security/content"
)

type Reader struct {
	Store             *Store
	Adapter           *Adapter
	CodeQuery, Rerank bool
}

func (r *Reader) Search(ctx context.Context, scopes []memoryservice.Scope, query string, limit int) ([]memoryservice.SemanticHit, error) {
	if r == nil || r.Store == nil || r.Store.Guard == nil || r.Adapter == nil || r.Adapter.Admission == nil {
		return nil, ErrAdmission
	}
	if query == "" || limit < 1 || limit > 4 {
		return nil, ErrInvalid
	}
	var output []memoryservice.SemanticHit
	for _, scope := range scopes {
		g, err := r.Store.Active(ctx, Scope{scope.Type, scope.ID})
		if IsMissing(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		a := *r.Adapter
		a.Space = g.Space
		task := pb.EmbeddingTask_EMBEDDING_TASK_QUERY
		if r.CodeQuery {
			task = pb.EmbeddingTask_EMBEDDING_TASK_CODE_QUERY
		}
		vectors, _, err := a.Embeddings(ctx, g.Scope, []*pb.TextInput{{Id: "query", Text: query}}, task)
		if err != nil {
			return nil, err
		}
		hits, err := r.Store.Search(ctx, g, vectors[0], limit)
		if err != nil {
			return nil, err
		}
		if r.Rerank && len(hits) > 0 {
			currentSources := make([]Source, len(hits))
			for i, h := range hits {
				currentSources[i] = h.Source
			}
			if err := r.Store.check(ctx, currentSources); err != nil {
				return nil, err
			}
			scores, _, err := a.Rerank(ctx, g.Scope, query, hits)
			if err != nil {
				return nil, err
			}
			type pair struct {
				hit   Hit
				score float64
			}
			pairs := make([]pair, len(hits))
			for i, h := range hits {
				pairs[i] = pair{h, scores[i]}
			}
			sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].score > pairs[j].score })
			for i, p := range pairs {
				hits[i] = p.hit
			}
		}
		// A reranker can be slow. Recheck authoritative sources after its response;
		// a withdrawn or edited record cannot reappear from the transient candidate.
		for _, h := range hits {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			live, err := r.Store.LiveSource(ctx, g.Scope, h.Source.ID)
			if IsMissing(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if !live.Revision.Equal(h.Source.Revision) || live.Hash() != h.Source.Hash() || live.MemoryOrigin().Sensitivity == content.Secret {
				continue
			}
			output = append(output, memoryservice.SemanticHit{Title: h.Source.Title, Summary: h.Source.Summary, Distance: h.Distance, SourceRef: "memory_chunk:" + strconv.FormatInt(h.Source.ID, 10), OriginType: h.Source.SourceType, OriginID: h.Source.SourceID, Revision: h.Source.Revision.UTC().Format("2006-01-02T15:04:05.999999999Z"), GenerationID: g.ID, SpaceID: g.Space.ID(), Origin: h.Source.MemoryOrigin()})
			if len(output) == limit {
				return output, nil
			}
		}
	}
	return output, nil
}
