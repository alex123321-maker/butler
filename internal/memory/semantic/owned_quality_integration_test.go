package semantic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	memoryservice "github.com/butler/butler/internal/memory/service"
)

// This real smoke applies ALL migrations, actual source authority and durable
// shared allowances. It calls the single already-deployed internal service.
func TestActualOwnedSemanticAuthority(t *testing.T) {
	tokenFile := os.Getenv("BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE")
	if tokenFile == "" {
		t.Skip("same deployed local service private file required")
	}
	f := ownedMemory(t, 8)
	assignLocalMandate(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	backend, closeBackend, e := NewPinnedBackend(f.target, tokenFile)
	if e != nil {
		t.Fatal("pinned authenticated connection unavailable")
	}
	defer closeBackend()
	js := jobs.New(f.pool, f.catalog, f.budgets, &jobs.LocalTarget{Endpoint: f.target.Endpoint(), Identity: f.target.Identity()})
	r := jobs.Request{ID: "actual-semantic-index", Kind: "index", Purpose: "private-retrieval", TaskID: "root", OriginalRunID: "run", Sources: []catalog.SourceRef{{ID: f.source.ID, Revision: f.source.Revision, AccessRevision: f.source.AccessRevision, Hash: f.source.Hash}}, Allowance: budget.Units{ModelTurns: 1, Tokens: 5000, ActiveMillis: 60000, ProviderInputBytes: 8192}, Deadline: time.Now().Add(time.Hour)}
	if _, e = js.Enqueue(f.owner, r); e != nil {
		t.Fatal(e)
	}
	job, ok, e := js.Claim(ctx)
	if e != nil || !ok {
		t.Fatal(e)
	}
	gate, e := NewJobAdmission(js, job, f.guard, f.target, budget.ModelPrice{Basis: "local_no_billing"})
	if e != nil {
		t.Fatal(e)
	}
	jobCtx := gate.Context(ctx)
	s := NewStore(f.pool)
	s.Guard = f.guard
	a := &Adapter{Backend: backend, Space: testSpace(768), Admission: gate, RerankerID: "qwen3-reranker-0.6b-bf16-v1"}
	g, e := s.Create(jobCtx, a.Space, Scope{"user", "owner"})
	if e != nil {
		t.Fatal(e)
	}
	indexed, e := Rebuild(jobCtx, s, a, g.ID, 10)
	if e != nil || len(indexed) != 1 {
		t.Fatal("owned actual indexing", e)
	}
	artifact := filepath.Join("..", "..", "..", "deploy", "local-models", "verification-retrieval-quality.json")
	raw, e := os.ReadFile(artifact)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	proof := Proof{ArtifactRef: "deploy/local-models/verification-retrieval-quality.json", ArtifactSHA256: hex.EncodeToString(sum[:]), CorpusVersion: "retrieval-quality-v1", Cases: 24, BaselineTop1: 11.0 / 24, CandidateTop1: 22.0 / 24}
	if e = s.Verify(jobCtx, g.ID, proof); e != nil {
		t.Fatal(e)
	}
	if e = s.Activate(jobCtx, g.ID, ""); e != nil {
		t.Fatal(e)
	}
	live, e := NewLiveAdmission(f.budgets, f.guard, f.target)
	if e != nil {
		t.Fatal(e)
	}
	a.Admission = live
	reader := &HybridReader{Store: s, Adapter: a, Rerank: false}
	queryCtx, stop := context.WithTimeout(f.live, time.Minute)
	defer stop()
	result, e := reader.SearchDetailed(queryCtx, []memoryservice.Scope{{Type: "user", ID: "owner"}}, "Какие данные содержит документ?", 4)
	if e != nil || len(result.Hits) != 1 || result.Hits[0].SourceRef == "" || result.Hits[0].Revision == "" {
		t.Fatal("owned current citation", e)
	}
	src, e := s.LiveSource(queryCtx, Scope{"user", "owner"}, f.chunk)
	if e != nil {
		t.Fatal(e)
	}
	_, ranked, e := a.Rerank(queryCtx, Scope{"user", "owner"}, "Какие данные содержит документ?", []Hit{{Source: src}})
	if e != nil {
		t.Fatal("owned optional rank", e)
	}
	var attempts int
	if e = f.pool.QueryRow(ctx, `SELECT count(*) FROM execution_budget_reservations`).Scan(&attempts); e != nil || attempts != 3 {
		t.Fatal("missing durable actual reservations", attempts, e)
	}
	late, e := s.Create(jobCtx, a.Space, Scope{"user", "owner"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.catalog.Delete(f.owner, f.source.ID, "delete", f.source.Revision); e != nil {
		t.Fatal(e)
	}
	var entries int
	if e = f.pool.QueryRow(ctx, `SELECT count(*) FROM semantic_index_entries`).Scan(&entries); e != nil || entries != 0 {
		t.Fatal("deleted derivative survived", e)
	}
	// No late vector publication or deleted source response.
	if e = s.Put(jobCtx, late.ID, src, unitVector(768)); e != ErrAccess {
		t.Fatal("deleted late publication", e)
	}
	// No needless inference is required to prove deletion from retrieval.
	reader.Adapter = nil
	deleted, e := reader.Search(queryCtx, []memoryservice.Scope{{Type: "user", ID: "owner"}}, "Какие данные содержит документ?", 4)
	if len(deleted) != 0 {
		t.Fatal("deleted source silently served", e)
	}
	report := struct {
		Generated                                                                                                    string
		Contract                                                                                                     string
		InferenceCalls                                                                                               int
		Index, Rank                                                                                                  Stats
		SourceAuthority, LocalOnly, JobIndependent, SharedAllowance, Citation, DeletedVectors, LatePublicationDenied bool
	}{time.Now().UTC().Format(time.RFC3339), "all-migrations/current-PG-source+local-only-mandate/owned-job+live-lease/pinned-internal-gRPC", attempts, indexed[0], ranked, true, true, true, true, true, true, true}
	if path := os.Getenv("BUTLER_TEST_OWNED_SEMANTIC_REPORT"); path != "" {
		raw, e = json.MarshalIndent(report, "", "  ")
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, append(raw, '\n'), 0600); e != nil {
			t.Fatal(e)
		}
	}
}
