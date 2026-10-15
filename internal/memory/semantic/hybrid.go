package semantic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/butler/butler/internal/budget"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/security/content"
)

type Diagnostic string

const (
	BaseHybrid             Diagnostic = "hybrid_base"
	Ranked                 Diagnostic = "reranked"
	MissingIndex           Diagnostic = "semantic_index_unavailable"
	EmbeddingUnavailable   Diagnostic = "embedding_unavailable"
	RankingUnavailable     Diagnostic = "reranker_unavailable"
	RankingInputLimit      Diagnostic = "reranker_input_limit"
	ExactOnly              Diagnostic = "exact_identifier_only"
	ContextLimited         Diagnostic = "retrieval_context_limited"
	LocalBackendUnverified Diagnostic = "local_query_backend_unverified"
	QueryInputLimit        Diagnostic = "query_input_limit"
)

const MaxQueryBytes = 2048

// SearchReport contains retrieval evidence, never a verified answer. Byte units
// conservatively bound the retrieved JSON; they are not measured model usage.
type SearchReport struct {
	Hits                              []memoryservice.SemanticHit
	Diagnostics                       []Diagnostic
	ContextBytes, TokenAllowanceUnits int
	Stats                             []Stats
	candidates, exact, vector         []Hit
}

type localQueryKey struct{}

func (r *HybridReader) SearchWithPolicy(ctx context.Context, scopes []memoryservice.Scope, query string, limit int, policy memoryservice.QueryPolicy) (memoryservice.SearchResult, error) {
	if !policy.Local {
		return memoryservice.SearchResult{Diagnostics: []string{"query_search_disabled"}}, nil
	}
	ctx = context.WithValue(ctx, localQueryKey{}, !policy.External)
	report, err := r.SearchDetailed(ctx, scopes, query, limit)
	result := memoryservice.SearchResult{Hits: report.Hits}
	for _, d := range report.Diagnostics {
		result.Diagnostics = append(result.Diagnostics, string(d))
	}
	return result, err
}

func (r *HybridReader) localBackendProven() bool {
	if r == nil || r.Adapter == nil {
		return false
	}
	b, ok := r.Adapter.Backend.(interface{ localTarget() budget.LocalTarget })
	a, owned := r.Adapter.Admission.(*LiveAdmission)
	if !ok || !owned || a == nil {
		return false
	}
	t := b.localTarget()
	return t.Endpoint() == "local-models:9090" && t.Identity() != "" && t.Endpoint() == a.target.Endpoint() && t.Identity() == a.target.Identity()
}

type HybridReader struct {
	Store             *Store
	Adapter           *Adapter
	CodeQuery, Rerank bool
	ContextBytes      int
	Log               *slog.Logger
}

func (r *HybridReader) Search(ctx context.Context, scopes []memoryservice.Scope, query string, limit int) ([]memoryservice.SemanticHit, error) {
	report, err := r.SearchDetailed(ctx, scopes, query, limit)
	var log *slog.Logger
	if r != nil {
		log = r.Log
	}
	if log == nil {
		log = slog.Default()
	}
	for _, d := range report.Diagnostics {
		if d != BaseHybrid && d != Ranked && d != ExactOnly {
			log.Warn("memory retrieval degraded", "reason", string(d))
		}
	}
	return report.Hits, err
}

func (r *HybridReader) SearchDetailed(ctx context.Context, scopes []memoryservice.Scope, query string, limit int) (SearchReport, error) {
	var report SearchReport
	if r == nil || r.Store == nil || r.Store.Guard == nil {
		return report, ErrAccess
	}
	if strings.TrimSpace(query) == "" || len(scopes) == 0 || len(scopes) > 12 || limit < 1 || limit > 4 {
		return report, ErrInvalid
	}
	if operationOrigin(ctx).Sensitivity == content.Secret {
		return report, content.ErrSecret
	}
	budget := r.ContextBytes
	if budget == 0 {
		budget = 6144
	}
	if budget < 128 || budget > 8192 {
		return report, ErrInvalid
	}
	seenScopes := map[Scope]bool{}
	for _, scope := range scopes {
		s := Scope{scope.Type, scope.ID}
		if !s.valid() || seenScopes[s] {
			return report, ErrInvalid
		}
		seenScopes[s] = true
	}
	// The owner message is still complete in the orchestrator's model input.
	// Search size is an optional-retrieval limit, not a run failure or a reason
	// to silently truncate the question and change which evidence is sought.
	if len(query) > MaxQueryBytes {
		report.Diagnostics = []Diagnostic{QueryInputLimit}
		return report, nil
	}
	var exact, text, vector []Hit
	for _, scope := range scopes {
		s := Scope{scope.Type, scope.ID}
		lex, err := r.Store.Lexical(ctx, s, query)
		if err != nil {
			return report, err
		}
		exact = append(exact, lex.Exact...)
		if exactOnlyQuery(query, technicalTerms(query)) {
			report.Diagnostics = appendUnique(report.Diagnostics, ExactOnly)
			continue
		}
		text = append(text, lex.FullText...)
		g, err := r.Store.Active(ctx, s)
		if IsMissing(err) {
			report.Diagnostics = appendUnique(report.Diagnostics, MissingIndex)
			continue
		}
		if err != nil {
			return report, err
		}
		if r.Adapter == nil {
			report.Diagnostics = appendUnique(report.Diagnostics, EmbeddingUnavailable)
			continue
		}
		if localOnly, _ := ctx.Value(localQueryKey{}).(bool); localOnly && !r.localBackendProven() {
			report.Diagnostics = appendUnique(report.Diagnostics, LocalBackendUnverified)
			continue
		}
		a := *r.Adapter
		a.Space = g.Space
		task := pb.EmbeddingTask_EMBEDDING_TASK_QUERY
		if r.CodeQuery {
			task = pb.EmbeddingTask_EMBEDDING_TASK_CODE_QUERY
		}
		vectors, stats, err := a.Embeddings(ctx, s, []*pb.TextInput{{Id: "hybrid-query", Text: query}}, task)
		if errors.Is(err, ErrUnavailable) {
			report.Diagnostics = appendUnique(report.Diagnostics, EmbeddingUnavailable)
			continue
		}
		if err != nil {
			return report, err
		}
		report.Stats = append(report.Stats, stats)
		hits, err := r.Store.Search(ctx, g, vectors[0], maxCandidates)
		if err != nil {
			return report, err
		}
		vector = append(vector, hits...)
	}
	candidates := fuse(exact, text, vector, 4)
	report.Diagnostics = appendUnique(report.Diagnostics, BaseHybrid)
	report.candidates = candidates
	report.exact = exact
	report.vector = vector
	localOnly, _ := ctx.Value(localQueryKey{}).(bool)
	if r.Rerank && r.Adapter != nil && (!localOnly || r.localBackendProven()) && len(candidates) > 1 {
		if err := r.rank(ctx, scopes, query, &report); err != nil {
			return report, err
		}
	}
	return r.publish(ctx, query, limit, budget, report)
}

func (r *HybridReader) rank(ctx context.Context, scopes []memoryservice.Scope, query string, report *SearchReport) error {
	candidates := report.candidates
	// Keep cross-scope RRF positions. Rank only each authorized scope's slots;
	// do not add uncalibrated scores from separate requests/scopes.
	for _, scope := range scopes {
		s := Scope{scope.Type, scope.ID}
		var group []Hit
		var positions []int
		for i, h := range candidates {
			if h.Source.Scope == s {
				group = append(group, h)
				positions = append(positions, i)
			}
		}
		if len(group) < 2 {
			continue
		}
		currentSources := make([]Source, len(group))
		for i, h := range group {
			currentSources[i] = h.Source
		}
		if err := r.Store.check(ctx, currentSources); err != nil {
			return err
		}
		if rankBytes(query, group) > 8192 {
			report.Diagnostics = appendUnique(report.Diagnostics, RankingInputLimit)
			continue
		}
		scores, stats, err := r.Adapter.Rerank(ctx, s, query, group)
		if errors.Is(err, ErrUnavailable) {
			report.Diagnostics = appendUnique(report.Diagnostics, RankingUnavailable)
			continue
		}
		if err != nil {
			return err
		}
		report.Stats = append(report.Stats, stats)
		report.Diagnostics = appendUnique(report.Diagnostics, Ranked)
		order := make([]int, len(group))
		for i := range order {
			order[i] = i
		}
		exactIDs := map[int64]bool{}
		for _, h := range report.exact {
			exactIDs[h.Source.ID] = true
		}
		sort.SliceStable(order, func(i, j int) bool {
			if exactIDs[group[order[i]].Source.ID] != exactIDs[group[order[j]].Source.ID] {
				return exactIDs[group[order[i]].Source.ID]
			}
			return scores[order[i]] > scores[order[j]]
		})
		for i, p := range positions {
			candidates[p] = group[order[i]]
		}
	}
	return nil
}

func (r *HybridReader) publish(ctx context.Context, query string, limit, budget int, report SearchReport) (SearchReport, error) {
	report.Hits = nil
	report.ContextBytes = 0
	report.TokenAllowanceUnits = 0
	for _, h := range report.candidates {
		live, err := r.Store.LiveSource(ctx, h.Source.Scope, h.Source.ID)
		if IsMissing(err) {
			continue
		}
		if err != nil {
			return report, err
		}
		if live.Hash() != h.Source.Hash() || !live.Revision.Equal(h.Source.Revision) || live.MemoryOrigin().Sensitivity == content.Secret {
			continue
		}
		excerpt, clipped := evidenceExcerpt(live.Content, query, 1024)
		item := memoryservice.SemanticHit{Title: live.Title, Summary: excerpt, Distance: h.Distance, SourceRef: "memory_chunk:" + strconv.FormatInt(live.ID, 10), OriginType: live.SourceType, OriginID: live.SourceID, Revision: live.Revision.UTC().Format("2006-01-02T15:04:05.999999999Z"), GenerationID: h.GenerationID, SpaceID: h.SpaceID, Origin: live.MemoryOrigin(), ExcerptClipped: clipped}
		next := append(append([]memoryservice.SemanticHit(nil), report.Hits...), item)
		encoded, err := json.Marshal(next)
		if err != nil {
			return report, ErrInvalid
		}
		if len(encoded) > budget {
			report.Diagnostics = appendUnique(report.Diagnostics, ContextLimited)
			continue
		}
		report.Hits = next
		report.ContextBytes = len(encoded)
		report.TokenAllowanceUnits = len(encoded)
		if len(report.Hits) == limit {
			break
		}
	}
	return report, nil
}

func rankBytes(query string, hits []Hit) int {
	n := len(query)
	for _, h := range hits {
		n += len(h.Source.Title) + len(h.Source.Content)
	}
	return n
}
func appendUnique(values []Diagnostic, value Diagnostic) []Diagnostic {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}

// Reciprocal rank fusion uses positions only. Exact identifiers are protected;
// cosine distance and PostgreSQL/Qwen scores are never added to each other.
func fuse(exact, text, vector []Hit, limit int) []Hit {
	type candidate struct {
		hit   Hit
		score float64
		exact bool
	}
	byID := map[int64]*candidate{}
	for listIndex, list := range [][]Hit{exact, text, vector} {
		seen := map[int64]bool{}
		for rank, h := range list {
			if seen[h.Source.ID] {
				continue
			}
			seen[h.Source.ID] = true
			c := byID[h.Source.ID]
			if c == nil {
				c = &candidate{hit: h}
				byID[h.Source.ID] = c
			}
			if h.GenerationID != "" {
				c.hit = h
			}
			c.score += 1.0 / float64(60+rank+1)
			c.exact = c.exact || listIndex == 0
		}
	}
	var all []*candidate
	for _, c := range byID {
		all = append(all, c)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].exact != all[j].exact {
			return all[i].exact
		}
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		if !all[i].hit.Source.Revision.Equal(all[j].hit.Source.Revision) {
			return all[i].hit.Source.Revision.After(all[j].hit.Source.Revision)
		}
		return all[i].hit.Source.ID < all[j].hit.Source.ID
	})
	contentSeen := map[string]bool{}
	sourceCounts := map[string]int{}
	var hits []Hit
	for _, c := range all {
		src := c.hit.Source
		hash := sha256.Sum256([]byte(strings.Join(strings.Fields(src.Content), " ")))
		key := hex.EncodeToString(hash[:])
		sourceKey := src.Scope.Type + ":" + src.Scope.ID + ":" + src.SourceType + ":" + src.SourceID
		if src.CatalogSourceID != "" {
			sourceKey = src.Scope.Type + ":" + src.Scope.ID + ":" + src.CatalogSourceID
		}
		if src.SourceID == "" && src.CatalogSourceID == "" {
			sourceKey += ":" + strconv.FormatInt(src.ID, 10)
		}
		if contentSeen[key] || sourceCounts[sourceKey] >= 2 {
			continue
		}
		contentSeen[key] = true
		sourceCounts[sourceKey]++
		hits = append(hits, c.hit)
		if len(hits) == limit {
			break
		}
	}
	return hits
}

// Excerpts are explicitly clipped evidence windows, not altered indexing or
// reranker inputs. Technical identifiers can select a later UTF-8-safe window.
func evidenceExcerpt(text, query string, limit int) (string, bool) {
	if len(text) <= limit {
		return text, false
	}
	start := 0
	for _, term := range technicalTerms(query) {
		if at := strings.Index(text, term); at >= 0 {
			start = max(0, at-limit/4)
			break
		}
	}
	for start < len(text) && !utf8.RuneStart(text[start]) {
		start++
	}
	end := min(len(text), start+limit)
	for end > start && end < len(text) && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[start:end], true
}
