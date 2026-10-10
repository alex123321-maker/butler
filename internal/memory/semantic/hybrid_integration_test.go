package semantic

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/chunks"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/security/content"
)

type hybridModel struct {
	fakeModel
	rankCalls            int
	unavailable, corrupt bool
	afterRank            func()
}

func (f *hybridModel) Describe(ctx context.Context, e *pb.Empty) (*pb.Catalog, error) {
	c, err := f.fakeModel.Describe(ctx, e)
	c.Models = append(c.Models, &pb.ModelProfile{Id: "qwen", Enabled: true, Model: "Qwen/Qwen3-Reranker-0.6B", Revision: "e61197ed45024b0ed8a2d74b80b4d909f1255473", Format: "safetensors", Precision: "BF16", PreprocessingVersion: "qwen3-rerank-yesno-v1", RuntimeVersion: "torch2.10.0+cu128/transformers5.19.0", SourceUrl: "https://huggingface.co/Qwen/Qwen3-Reranker-0.6B"})
	return c, err
}
func (f *hybridModel) Infer(ctx context.Context, r *pb.InferRequest) (*pb.InferResponse, error) {
	if r.GetEmbed() != nil {
		return f.fakeModel.Infer(ctx, r)
	}
	f.rankCalls++
	if f.unavailable {
		return nil, ErrUnavailable
	}
	out := &pb.RerankOutput{}
	for i, c := range r.GetRerank().Candidates {
		out.Scores = append(out.Scores, &pb.Score{Id: c.Id, Value: float64(i+1) / float64(len(r.GetRerank().Candidates)+1)})
	}
	if f.corrupt {
		out.Scores[0].Value = math.NaN()
	}
	if f.afterRank != nil {
		f.afterRank()
	}
	return &pb.InferResponse{RequestId: r.RequestId, ProfileId: r.ProfileId, Revision: "e61197ed45024b0ed8a2d74b80b4d909f1255473", Output: &pb.InferResponse_Rerank{Rerank: out}}, nil
}

func TestHybridExactFullTextFreshnessAndAbsentIdentifier(t *testing.T) {
	ctx, s, cs := database(t)
	scope := Scope{"session", "semantic:hybrid-exact"}
	want, err := cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: scope.ID, Title: "Methods", Content: "ClaimGrant(ctx) controls the external boundary in internal/actionauth/store.go at v1.27.2. Вкладку нельзя переключать.", SourceID: "file:exact"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: scope.ID, Title: "Similar", Content: "ClaimGrant2 is a different method.", SourceID: "file:other"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: "other", Title: "Unauthorized", Content: "ClaimGrant hidden"})
	if err != nil {
		t.Fatal(err)
	}
	r := &HybridReader{Store: s}
	scopes := []memoryservice.Scope{{Type: scope.Type, ID: scope.ID}}
	for _, q := range []string{"ClaimGrant", "internal/actionauth/store.go", "v1.27.2"} {
		out, err := r.SearchDetailed(ctx, scopes, q, 4)
		if err != nil || len(out.Hits) != 1 || out.Hits[0].OriginID != "file:exact" || out.Hits[0].Origin.IsInstruction() || out.Hits[0].Revision == "" {
			t.Fatal("exact match lost", q, out, err)
		}
	}
	lex, err := s.Lexical(ctx, scope, "вкладками")
	if err != nil || len(lex.FullText) != 1 || lex.FullText[0].Source.ID != want.ID {
		t.Fatal("Russian stemming lost", lex, err)
	}
	out, err := r.SearchDetailed(ctx, scopes, "MissingTechnicalFunction", 4)
	if err != nil || len(out.Hits) != 0 {
		t.Fatal("invented nearest answer for absent identifier", out, err)
	}
	if _, err = s.pool.Exec(ctx, `UPDATE memory_chunks SET suppressed=true WHERE id=$1`, want.ID); err != nil {
		t.Fatal(err)
	}
	out, err = r.SearchDetailed(ctx, scopes, "ClaimGrant", 4)
	if err != nil || len(out.Hits) != 0 {
		t.Fatal("withdrawn exact result visible", out, err)
	}
	s.Guard = labSourceGuard{deny: true}
	if _, err = r.SearchDetailed(ctx, scopes, "ClaimGrant2", 4); !errors.Is(err, ErrAccess) {
		t.Fatal("denied source was passed", err)
	}
}

func TestHybridRankerDegradesExplicitlyButNeverBypassesAdmissionOrCorruption(t *testing.T) {
	ctx, s, cs := database(t)
	scope := Scope{"session", "semantic:hybrid-rank"}
	var first int64
	for i, body := range []string{"Правило запрещает переключение вкладки", "Наблюдение читает вкладку"} {
		c, err := cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: scope.ID, Title: body, Content: body, Summary: body, SourceID: body})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = c.ID
		}
	}
	f := &hybridModel{fakeModel: fakeModel{space: testSpace(768)}}
	a := &Adapter{Space: f.space, Backend: f, Admission: &testAdmission{}, RerankerID: "qwen"}
	g, err := s.Create(ctx, a.Space, scope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Rebuild(ctx, s, a, g.ID, 10); err != nil {
		t.Fatal(err)
	}
	if err = s.Verify(ctx, g.ID, qualityProof()); err != nil {
		t.Fatal(err)
	}
	if err = s.Activate(ctx, g.ID, ""); err != nil {
		t.Fatal(err)
	}
	r := &HybridReader{Store: s, Adapter: a, Rerank: true}
	scopes := []memoryservice.Scope{{Type: scope.Type, ID: scope.ID}}
	f.unavailable = true
	out, err := r.SearchDetailed(ctx, scopes, "Причина ограничения вкладки", 4)
	if err != nil || len(out.Hits) != 2 || !hasDiagnostic(out, RankingUnavailable) {
		t.Fatal("ranker outage turned into false empty result", out, err)
	}
	f.unavailable = false
	f.corrupt = true
	if _, err = r.SearchDetailed(ctx, scopes, "Причина ограничения вкладки", 4); !errors.Is(err, ErrInvalid) {
		t.Fatal("corrupt rank partial accepted", err)
	}
	f.corrupt = false
	a.Admission = &testAdmission{deny: true}
	before := f.calls + f.rankCalls
	if _, err = r.SearchDetailed(ctx, scopes, "Причина ограничения вкладки", 4); !errors.Is(err, ErrAdmission) || f.calls+f.rankCalls != before {
		t.Fatal("budget denial became fallback", err)
	}
	a.Admission = &testAdmission{}
	f.afterRank = func() {
		if _, err := s.pool.Exec(ctx, `UPDATE memory_chunks SET suppressed=true WHERE id=$1`, first); err != nil {
			t.Fatal(err)
		}
	}
	out, err = r.SearchDetailed(ctx, scopes, "Причина ограничения вкладки", 4)
	if err != nil || len(out.Hits) != 1 {
		t.Fatal("postranking source recheck failed", out, err)
	}
	for _, h := range out.Hits {
		if h.Origin.Sensitivity == content.Secret || strings.Contains(h.Summary, "запрещает") {
			t.Fatal("withdrawn source reappeared")
		}
	}
}

func hasDiagnostic(r SearchReport, want Diagnostic) bool {
	for _, d := range r.Diagnostics {
		if d == want {
			return true
		}
	}
	return false
}

func TestHybridOversizeRankFallsBackAndCitedContextIsBounded(t *testing.T) {
	ctx, s, cs := database(t)
	scope := Scope{"session", "semantic:hybrid-size"}
	for _, prefix := range []string{"Первый", "Второй"} {
		if _, err := cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: scope.ID, Title: prefix, Content: prefix + strings.Repeat(" текст", 1000), SourceID: prefix}); err != nil {
			t.Fatal(err)
		}
	}
	sources, err := s.Sources(ctx, scope, 10)
	if err != nil {
		t.Fatal(err)
	}
	f := &hybridModel{fakeModel: fakeModel{space: testSpace(768)}}
	a := &Adapter{Space: f.space, Backend: f, Admission: &testAdmission{}, RerankerID: "qwen"}
	g, err := s.Create(ctx, a.Space, scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, src := range sources {
		if err = s.Put(ctx, g.ID, src, unitVector(768)); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Verify(ctx, g.ID, qualityProof()); err != nil {
		t.Fatal(err)
	}
	if err = s.Activate(ctx, g.ID, ""); err != nil {
		t.Fatal(err)
	}
	r := &HybridReader{Store: s, Adapter: a, Rerank: true, ContextBytes: 1600}
	out, err := r.SearchDetailed(ctx, []memoryservice.Scope{{Type: scope.Type, ID: scope.ID}}, "Искомый текст", 4)
	if err != nil || !hasDiagnostic(out, RankingInputLimit) || f.rankCalls != 0 || len(out.Hits) != 1 || !out.Hits[0].ExcerptClipped || out.ContextBytes > 1600 || out.TokenAllowanceUnits != out.ContextBytes {
		t.Fatal("oversize rank/context contract", out, err)
	}
}
