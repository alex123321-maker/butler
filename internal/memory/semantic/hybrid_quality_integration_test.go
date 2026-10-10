package semantic

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/chunks"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type hybridQualityMetrics struct{ Top1, MRR, Recall4 float64 }
type hybridQualityCase struct {
	QueryID, Expected, EmbeddingTop, HybridTop, RankedTop string
	HybridSources, RankedSources                          []string
	ContextBytes                                          int
}

func scoreSources(m *hybridQualityMetrics, found []string, expected string) {
	for i, id := range found {
		if id == expected {
			if i == 0 {
				m.Top1++
			}
			m.MRR += 1 / float64(i+1)
			m.Recall4++
			break
		}
	}
}
func fixtureID(source string) string {
	at := strings.LastIndex(source, "#")
	if at < 0 {
		return source
	}
	return source[at+1:]
}
func reportIDs(r SearchReport) []string {
	var out []string
	for _, h := range r.Hits {
		out = append(out, fixtureID(h.OriginID))
	}
	return out
}

func TestActualHybridRetrievalQuality(t *testing.T) {
	endpoint := os.Getenv("BUTLER_TEST_LOCAL_MODELS_ENDPOINT")
	if endpoint == "" {
		t.Skip("real local service not configured")
	}
	ctx, s, cs := database(t)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Minute)
	defer cancel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "local-models", "retrieval-quality.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus qualityCorpus
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Queries) != 24 {
		t.Fatal("fixed corpus")
	}
	scope := Scope{"session", "semantic:hybrid-quality"}
	scopes := []memoryservice.Scope{{Type: scope.Type, ID: scope.ID}}
	for _, d := range corpus.Documents {
		if _, err = cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: scope.ID, Title: d.Title, Content: d.Text, Summary: d.Text, SourceType: "fixture", SourceID: corpus.Version + "#" + d.ID, ProvenanceJSON: `{"kind":"document","source_refs":["fixture:retrieval-quality-v1"],"sensitivity":"private"}`}); err != nil {
			t.Fatal(err)
		}
	}
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	backend, err := NewAuthenticatedBackend(conn, os.Getenv("BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE"))
	if err != nil {
		t.Fatal("private source")
	}
	gate := &labAdmission{scope: scope, deadline: time.Now().Add(8 * time.Minute)}
	a := &Adapter{Backend: backend, Space: testSpace(768), Admission: gate, RerankerID: "qwen3-reranker-0.6b-bf16-v1"}
	g, err := s.Create(ctx, a.Space, scope)
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	indexStats, err := Rebuild(ctx, s, a, g.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	indexMS := time.Since(started).Milliseconds()
	// Use the previously measured same-corpus embedding artifact for fixture
	// activation, rather than inventing a quality score. Main pointers stay off.
	priorRaw, err := os.ReadFile(filepath.Join("..", "..", "..", "deploy", "local-models", "verification-retrieval-quality.json"))
	if err != nil {
		t.Fatal(err)
	}
	var prior struct {
		CorpusVersion string                    `json:"corpus_version"`
		CorpusSHA256  string                    `json:"corpus_sha256"`
		Baseline      float64                   `json:"lexical_bm25_top1"`
		Metrics       map[string]qualityMetrics `json:"metrics"`
	}
	if json.Unmarshal(priorRaw, &prior) != nil || prior.CorpusVersion != corpus.Version || prior.CorpusSHA256 != hexDigest(raw) {
		t.Fatal("embedding proof corpus mismatch")
	}
	proof := Proof{"fixture://embedding-quality", hexDigest(priorRaw), corpus.Version, 24, prior.Baseline, prior.Metrics["768"].Top1}
	if err = s.Verify(ctx, g.ID, proof); err != nil {
		t.Fatal(err)
	}
	if err = s.Activate(ctx, g.ID, ""); err != nil {
		t.Fatal(err)
	}
	r := &HybridReader{Store: s, Adapter: a}
	base, hybrid, ranked := &hybridQualityMetrics{}, &hybridQualityMetrics{}, &hybridQualityMetrics{}
	var reports []SearchReport
	var cases []hybridQualityCase
	var inference []Stats
	started = time.Now()
	for _, q := range corpus.Queries {
		r.CodeQuery = q.Code
		report, err := r.SearchDetailed(ctx, scopes, q.Text, 4)
		if err != nil {
			t.Fatal(q.ID, err)
		}
		inference = append(inference, report.Stats...)
		var vectorIDs []string
		for _, h := range report.vector[:min(4, len(report.vector))] {
			vectorIDs = append(vectorIDs, fixtureID(h.Source.SourceID))
		}
		hybridIDs := reportIDs(report)
		scoreSources(base, vectorIDs, q.Expected)
		scoreSources(hybrid, hybridIDs, q.Expected)
		c := hybridQualityCase{QueryID: q.ID, Expected: q.Expected, HybridSources: hybridIDs, ContextBytes: report.ContextBytes}
		if len(vectorIDs) > 0 {
			c.EmbeddingTop = vectorIDs[0]
		}
		if len(hybridIDs) > 0 {
			c.HybridTop = hybridIDs[0]
		}
		cases = append(cases, c)
		reports = append(reports, report)
	}
	queryMS := time.Since(started).Milliseconds()
	started = time.Now()
	for i, q := range corpus.Queries {
		report := reports[i]
		report.Stats = nil
		if err = r.rank(ctx, scopes, q.Text, &report); err != nil {
			t.Fatal("ranking", q.ID, err)
		}
		report, err = r.publish(ctx, q.Text, 4, 6144, report)
		if err != nil {
			t.Fatal("source recheck", err)
		}
		inference = append(inference, report.Stats...)
		ids := reportIDs(report)
		scoreSources(ranked, ids, q.Expected)
		cases[i].RankedSources = ids
		if len(ids) > 0 {
			cases[i].RankedTop = ids[0]
		}
	}
	rankMS := time.Since(started).Milliseconds()
	for _, m := range []*hybridQualityMetrics{base, hybrid, ranked} {
		m.Top1 /= 24
		m.MRR /= 24
		m.Recall4 /= 24
	}
	// One complete serial query/rerank runs the public method and includes both
	// GPU model switches. Grouped profiling above isolates candidate quality.
	r.Rerank = true
	r.CodeQuery = false
	started = time.Now()
	serial, err := r.SearchDetailed(ctx, scopes, corpus.Queries[0].Text, 4)
	if err != nil {
		t.Fatal("serial flow", err)
	}
	serialMS := time.Since(started).Milliseconds()
	inference = append(inference, serial.Stats...)
	if !hasDiagnostic(serial, Ranked) || len(serial.Hits) == 0 {
		t.Fatal("actual serial rerank unavailable")
	}
	r.Rerank = false
	exactChecks := map[string]bool{}
	for query, want := range map[string]string{"ClaimGrant": "code-claim", "PrepareWithLogicalRef": "code-prepare", "BudgetRoot": "code-root", "ReadRegular": "code-files", "normalizeVector": "code-vector"} {
		report, err := r.SearchDetailed(ctx, scopes, query, 4)
		if err != nil {
			t.Fatal(err)
		}
		ids := reportIDs(report)
		exactChecks[query] = len(ids) > 0 && ids[0] == want
		if !exactChecks[query] {
			t.Fatal("exact technical result missing", query)
		}
	}
	absent, err := r.SearchDetailed(ctx, scopes, "NonexistentButlerMethod", 4)
	if err != nil || len(absent.Hits) != 0 {
		t.Fatal("absent identifier did not abstain", err)
	}
	result := map[string]any{"corpus_version": corpus.Version, "corpus_sha256": hexDigest(raw), "scope": corpus.Scope, "cases": cases, "metrics": map[string]any{"embedding768": base, "hybrid_rrf": hybrid, "hybrid_reranked": ranked}, "exact_identifiers": exactChecks, "absent_identifier": "no source; no invented semantic nearest answer", "indexing_ms": indexMS, "grouped_query_ms_24": queryMS, "grouped_rerank_ms_24": rankMS, "actual_serial_query_rerank_ms": serialMS, "indexing": indexStats, "inference": inference, "admitted_calls": gate.calls, "admitted_input_bytes": gate.bytes, "rank_fusion": "RRF k=60 plus protected exact technical hits; no score addition", "context": "4 cited UTF8 excerpts <=6144 serialized bytes; conservative byte token allowance", "answer_grounding": "retrieval expected-source coverage only; no generative answer correctness claim", "production": "disabled pending source/job/current local-run admission from22/23", "rerank_default": false, "rerank_improves_top1": ranked.Top1 > hybrid.Top1}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')
	path := os.Getenv("BUTLER_TEST_SEMANTIC_REPORT")
	if path == "" {
		t.Fatal("report path")
	}
	if err = os.WriteFile(path, encoded, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("24 cases embedding/hybrid/rerank top1 %.3f/%.3f/%.3f; recall4 %.3f/%.3f/%.3f", base.Top1, hybrid.Top1, ranked.Top1, base.Recall4, hybrid.Recall4, ranked.Recall4)
}
