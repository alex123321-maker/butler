package semantic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/chunks"
	"github.com/butler/butler/internal/security/content"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type qualityDoc struct {
	ID, Title, Text string
	Code            bool
}
type qualityQuery struct {
	ID, Text, Expected string
	Code               bool
}
type qualityCorpus struct {
	Version, Scope string
	Documents      []qualityDoc
	Queries        []qualityQuery
}

// This admission is deliberately only in a lab test. The fixture scope, calls,
// bytes and total deadline are finite; it cannot be enabled by production flags.
type labAdmission struct {
	scope        Scope
	deadline     time.Time
	calls, bytes int
}

func (a *labAdmission) Admit(ctx context.Context, o Operation) (context.Context, func(), error) {
	if ctx.Err() != nil || o.Scope != a.scope || time.Now().After(a.deadline) || a.calls >= 160 || a.bytes+o.InputBytes > 2*1024*1024 || o.Origin.Sensitivity == content.Secret {
		return nil, nil, ErrAdmission
	}
	a.calls++
	a.bytes += o.InputBytes
	return ctx, func() {}, nil
}

type qualityCase struct {
	QueryID, Expected, Lexical, Keyword string
	Top768, Top256, Rank768, Rank256    string
	Recall4_768, Recall4_256            bool
}
type qualityMetrics struct{ Top1, MRR, Recall4, RerankedTop1 float64 }

func TestActualLocalRetrievalQuality(t *testing.T) {
	endpoint := os.Getenv("BUTLER_TEST_LOCAL_MODELS_ENDPOINT")
	if endpoint == "" {
		t.Skip("real local service not configured")
	}
	ctx, s, cs := database(t)
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Minute)
	defer cancel()
	root := filepath.Join("..", "..", "..", "deploy", "local-models")
	raw, err := os.ReadFile(filepath.Join(root, "retrieval-quality.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus qualityCorpus
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Queries) != 24 {
		t.Fatal("corpus")
	}
	scope := Scope{"session", "semantic:quality"}
	ids := map[int64]string{}
	for _, d := range corpus.Documents {
		src, err := cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: scope.ID, Title: d.Title, Content: d.Text, Summary: d.Text, SourceType: "fixture", SourceID: corpus.Version + "#" + d.ID, ProvenanceJSON: `{"kind":"document","source_refs":["fixture:retrieval-quality-v1"],"sensitivity":"private"}`})
		if err != nil {
			t.Fatal(err)
		}
		ids[src.ID] = d.ID
	}
	conn, err := grpc.NewClient(endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal("connect")
	}
	defer conn.Close()
	backend, err := NewAuthenticatedBackend(conn, os.Getenv("BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE"))
	if err != nil {
		t.Fatal("private source")
	}
	gate := &labAdmission{scope: scope, deadline: time.Now().Add(8 * time.Minute)}
	adapters := map[int]*Adapter{}
	generations := map[int]Generation{}
	var indexing []Stats
	var measured []Stats
	indexStarted := time.Now()
	for _, dims := range []int{768, 256} {
		a := &Adapter{Backend: backend, Space: testSpace(dims), Admission: gate, RerankerID: "qwen3-reranker-0.6b-bf16-v1"}
		g, err := s.Create(ctx, a.Space, scope)
		if err != nil {
			t.Fatal(err)
		}
		stats, err := Rebuild(ctx, s, a, g.ID, 100)
		if err != nil {
			t.Fatal("indexing", err)
		}
		indexing = append(indexing, stats...)
		adapters[dims] = a
		generations[dims] = g
	}
	indexMS := time.Since(indexStarted).Milliseconds()
	var cases []qualityCase
	metrics := map[int]*qualityMetrics{768: {}, 256: {}}
	var bm25Hits, keywordHits int
	staged := map[int][][]Hit{768: {}, 256: {}}
	queryStarted := time.Now()
	for _, q := range corpus.Queries {
		c := qualityCase{QueryID: q.ID, Expected: q.Expected}
		lex := bm25(corpus.Documents, q.Text)
		c.Lexical = corpus.Documents[lex[0]].ID
		if c.Lexical == q.Expected {
			bm25Hits++
		}
		// Exercise the current store's exact title fallback, not an unavailable
		// cloud baseline. A stronger BM25 control is reported separately.
		for _, term := range currentKeywordTerms(q.Text) {
			matches, err := cs.FindByTitle(ctx, scope.Type, scope.ID, term, 2)
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) > 0 {
				c.Keyword = ids[matches[0].ID]
				break
			}
		}
		if c.Keyword == q.Expected {
			keywordHits++
		}
		for _, dims := range []int{768, 256} {
			task := pb.EmbeddingTask_EMBEDDING_TASK_QUERY
			if q.Code {
				task = pb.EmbeddingTask_EMBEDDING_TASK_CODE_QUERY
			}
			v, stats, err := adapters[dims].Embeddings(ctx, scope, []*pb.TextInput{{Id: q.ID, Text: q.Text}}, task)
			if err != nil {
				t.Fatal("query", err)
			}
			measured = append(measured, stats)
			hits, err := s.Preview(ctx, generations[dims], v[0], 4)
			if err != nil || len(hits) != 4 {
				t.Fatal("preview", err)
			}
			staged[dims] = append(staged[dims], hits)
			m := metrics[dims]
			if ids[hits[0].Source.ID] == q.Expected {
				m.Top1++
			}
			for rank, h := range hits {
				if ids[h.Source.ID] == q.Expected {
					m.MRR += 1 / float64(rank+1)
					m.Recall4++
					if dims == 768 {
						c.Recall4_768 = true
					} else {
						c.Recall4_256 = true
					}
					break
				}
			}
			if dims == 768 {
				c.Top768 = ids[hits[0].Source.ID]
			} else {
				c.Top256 = ids[hits[0].Source.ID]
			}
		}
		cases = append(cases, c)
	}
	queryMS := time.Since(queryStarted).Milliseconds()
	rankStarted := time.Now()
	for i, q := range corpus.Queries {
		for _, dims := range []int{768, 256} {
			hits := staged[dims][i]
			scores, stats, err := adapters[dims].Rerank(ctx, scope, q.Text, hits)
			if err != nil {
				t.Fatal("rerank", err)
			}
			measured = append(measured, stats)
			best := 0
			for j := range scores {
				if scores[j] > scores[best] {
					best = j
				}
			}
			top := ids[hits[best].Source.ID]
			if top == q.Expected {
				metrics[dims].RerankedTop1++
			}
			if dims == 768 {
				cases[i].Rank768 = top
			} else {
				cases[i].Rank256 = top
			}
		}
	}
	rankMS := time.Since(rankStarted).Milliseconds()
	n := float64(len(corpus.Queries))
	for _, m := range metrics {
		m.Top1 /= n
		m.MRR /= n
		m.Recall4 /= n
		m.RerankedTop1 /= n
	}
	bm25Top1 := float64(bm25Hits) / n
	gain := metrics[768].Top1 > bm25Top1
	report := map[string]any{"corpus_version": corpus.Version, "corpus_sha256": hexDigest(raw), "scope": corpus.Scope, "cases": cases, "metrics": metrics, "current_exact_title_keyword_top1": float64(keywordHits) / n, "lexical_bm25_top1": bm25Top1, "current_cloud_semantic_baseline": "unavailable; no claimed comparison", "indexing_ms_both_spaces": indexMS, "query_ms_48_requests": queryMS, "rerank_ms_48_requests": rankMS, "indexing": indexing, "inference": measured, "admitted_calls": gate.calls, "admitted_input_bytes": gate.bytes, "embedding_gain_over_bm25": gain, "production": "disabled; trusted run/job admission and #22/#23 integration pending", "precision": "BF16 GPU or FP32 CPU, never FP16"}
	encoded, _ := json.MarshalIndent(report, "", "  ")
	reportPath := os.Getenv("BUTLER_TEST_SEMANTIC_REPORT")
	if reportPath == "" {
		t.Fatal("report path required")
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(reportPath, encoded, 0644); err != nil {
		t.Fatal(err)
	}
	// Only activate an objectively winning staged embedding generation in this
	// disposable fixture. No main pointer or owner memory is changed.
	if gain {
		p := Proof{"fixture://" + corpus.Version, hexDigest(encoded), corpus.Version, len(corpus.Queries), bm25Top1, metrics[768].Top1}
		if err := s.Verify(ctx, generations[768].ID, p); err != nil {
			t.Fatal("verify", err)
		}
		if err := s.Activate(ctx, generations[768].ID, ""); err != nil {
			t.Fatal("activate", err)
		}
	}
	t.Logf("24 cases: BM25 %.3f; embedding768 %.3f/256 %.3f; reranked768 %.3f/256 %.3f; gain=%t", bm25Top1, metrics[768].Top1, metrics[256].Top1, metrics[768].RerankedTop1, metrics[256].RerankedTop1, gain)
}
func hexDigest(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func currentKeywordTerms(text string) []string {
	seen := map[string]bool{}
	var terms []string
	for _, term := range strings.Fields(strings.NewReplacer(",", " ", ".", " ", ":", " ", ";", " ", "-", " ", "_", " ", "\n", " ").Replace(strings.ToLower(strings.TrimSpace(text)))) {
		if len(term) >= 4 && !seen[term] {
			seen[term] = true
			terms = append(terms, term)
		}
	}
	return terms
}
func terms(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// A finite standard BM25 lexical reference, no semantic synonyms or tuning
// against model answers. The corpus and qrels are fixed before the first run.
func bm25(docs []qualityDoc, query string) []int {
	df := map[string]int{}
	tf := make([]map[string]int, len(docs))
	length := make([]int, len(docs))
	avg := 0.0
	for i, d := range docs {
		tf[i] = map[string]int{}
		for _, w := range terms(d.Title + " " + d.Text) {
			tf[i][w]++
			length[i]++
		}
		avg += float64(length[i])
		for w := range tf[i] {
			df[w]++
		}
	}
	avg /= float64(len(docs))
	values := make([]float64, len(docs))
	for i := range docs {
		for _, w := range terms(query) {
			f := float64(tf[i][w])
			if f == 0 {
				continue
			}
			idf := math.Log(1 + (float64(len(docs)-df[w])+0.5)/(float64(df[w])+0.5))
			values[i] += idf * f * 2.2 / (f + 1.2*(0.25+0.75*float64(length[i])/avg))
		}
	}
	order := make([]int, len(docs))
	for i := range docs {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return values[order[i]] > values[order[j]] })
	return order
}
