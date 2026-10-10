package indexing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/ingestion"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/semantic"
	"github.com/butler/butler/internal/owneridentity"
)

type model struct {
	space semantic.Space
	calls atomic.Int64
	fail  int64
	after func()
}

func TestActualPinnedOwnedIndexRuntimeIntegration(t *testing.T) {
	tokenFile := os.Getenv("BUTLER_TEST_OWNED_INDEX_REAL_TOKEN_FILE")
	if tokenFile == "" {
		t.Skip("actual private pinned local model service required")
	}
	r, ctx, src, _ := fixture(t)
	backend, closeBackend, e := semantic.NewPinnedBackend(r.Target, tokenFile)
	if e != nil {
		t.Fatal("private service unavailable")
	}
	defer closeBackend()
	r.Backend = backend
	q := request(src)
	receipt, e := r.Enqueue(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	bounded, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	if ok, e := r.ProcessOne(bounded); !ok || e != nil {
		t.Fatal("actual owned indexing", ok, e)
	}
	var count int
	if e = r.Catalog.Pool().QueryRow(ctx, `SELECT count(*) FROM semantic_index_entries WHERE generation_id=$1`, receipt.GenerationID).Scan(&count); e != nil || count != 5 {
		t.Fatal("actual vectors", count, e)
	}
	var state string
	if e = r.Catalog.Pool().QueryRow(ctx, `SELECT state FROM semantic_index_generations WHERE id=$1`, receipt.GenerationID).Scan(&state); e != nil || state != "building" {
		t.Fatal("unreviewed index activated", state, e)
	}
	if _, e = r.Catalog.Delete(ctx, src.ID, "delete", src.Revision); e != nil {
		t.Fatal(e)
	}
	if e = r.Catalog.Pool().QueryRow(ctx, `SELECT count(*) FROM semantic_index_entries WHERE generation_id=$1`, receipt.GenerationID).Scan(&count); e != nil || count != 0 {
		t.Fatal("actual vectors retained after deletion", count, e)
	}
}

func (m *model) Describe(context.Context, *pb.Empty) (*pb.Catalog, error) {
	return &pb.Catalog{Models: []*pb.ModelProfile{{Id: m.space.ProfileID, Model: m.space.Model, Revision: m.space.Revision, PreprocessingVersion: m.space.Preprocessing, RuntimeVersion: m.space.RuntimeVersion, Dimensions: 768, Format: "safetensors", Precision: "BF16", Enabled: true, SourceUrl: "https://huggingface.co/" + m.space.Model, Capabilities: []pb.Capability{pb.Capability_CAPABILITY_EMBED}}}}, nil
}
func (m *model) Infer(ctx context.Context, r *pb.InferRequest) (*pb.InferResponse, error) {
	n := m.calls.Add(1)
	if m.after != nil {
		m.after()
	}
	if n == m.fail {
		return nil, errors.New("unavailable")
	}
	out := &pb.EmbedOutput{IndexSpace: m.space.WireID()}
	for _, t := range r.GetEmbed().Texts {
		v := make([]float32, m.space.Dimensions)
		v[0] = 1
		out.Vectors = append(out.Vectors, &pb.Vector{Id: t.Id, Values: v})
	}
	return &pb.InferResponse{RequestId: r.RequestId, ProfileId: r.ProfileId, Revision: m.space.Revision, Output: &pb.InferResponse_Embed{Embed: out}}, nil
}
func fixture(t *testing.T) (*Runtime, context.Context, catalog.Source, *model) {
	t.Helper()
	pool := memorytest.Database(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if e := manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	c := catalog.New(pool, manifest)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "index-owner"})
	src, e := c.Put(ctx, catalog.PutSource{ExternalKey: "source", Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "Документ", Body: "Проверяемый русский исходник"})
	if e != nil {
		t.Fatal(e)
	}
	var fragments []catalog.Fragment
	for i := 0; i < 5; i++ {
		fragments = append(fragments, catalog.Fragment{Key: strconv.Itoa(i), Title: "Русский раздел", Text: "Данные Service.Restart " + strconv.Itoa(i), Locator: []byte(`{"path":"fixture.md"}`)})
	}
	if _, e = c.PublishFragments(ctx, catalog.SourceRef{ID: src.ID}, fragments); e != nil {
		t.Fatal(e)
	}
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	target, e := budget.NewLocalTarget("local-models:9090", "operator-pinned-service")
	if e != nil {
		t.Fatal(e)
	}
	space := semantic.Space{ProfileID: "embeddinggemma2-text-bf16-v1", Model: "google/embeddinggemma-2", Revision: "914f7f89142e33e77833254d9c9b90c3cef7303b", Preprocessing: "embeddinggemma2-retrieval-v1", RuntimeVersion: "torch2.10.0+cu128/transformers5.19.0/sentence-transformers6.1.0", ChunkVersion: "structured-go-v1", Dimensions: 768}
	m := &model{space: space}
	return &Runtime{Catalog: c, Jobs: jobs.New(pool, c, b, &jobs.LocalTarget{Endpoint: target.Endpoint(), Identity: target.Identity()}), Backend: m, Target: target, Space: space, Price: budget.ModelPrice{Basis: "local_no_billing"}}, ctx, src, m
}
func request(src catalog.Source) Request {
	return Request{Job: jobs.Request{ID: "index-job", Kind: "index", Purpose: "memory_read", Sources: []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}, Allowance: budget.Units{ModelTurns: 5, Tokens: 50000, ActiveMillis: 600000, ProviderInputBytes: 1000000}, Deadline: time.Now().Add(time.Hour)}, ScopeType: "user", ScopeID: "owner", MaxChunks: 100}
}
func TestAtomicIndexJobPublicationAndBoundedRestartIntegration(t *testing.T) {
	r, ctx, src, m := fixture(t)
	q := request(src)
	r.Space.Dimensions = 0
	if _, e := r.Enqueue(ctx, q); e == nil {
		t.Fatal("invalid projection committed")
	}
	var n int
	if e := r.Catalog.Pool().QueryRow(ctx, `SELECT count(*) FROM memory_aux_jobs`).Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	r.Space = m.space
	a, e := r.Enqueue(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	same, e := r.Enqueue(ctx, q)
	if e != nil || same.GenerationID != a.GenerationID {
		t.Fatal(same, e)
	}
	// Other owned workers cannot be silently consumed by this indexing loop.
	extract := q.Job
	extract.ID = "aaa-extraction"
	extract.Kind = "extract"
	if _, e = r.Jobs.Enqueue(ctx, extract); e != nil {
		t.Fatal(e)
	}
	m.fail = 2
	if ok, e := r.ProcessOne(context.Background()); !ok || !errors.Is(e, semantic.ErrUnavailable) {
		t.Fatal(ok, e)
	}
	if e = r.Catalog.Pool().QueryRow(ctx, `SELECT count(*) FROM semantic_index_entries WHERE generation_id=$1`, a.GenerationID).Scan(&n); e != nil || n != 4 {
		t.Fatal("partial durable prefix", n, e)
	}
	if _, e = r.Catalog.Pool().Exec(ctx, `UPDATE memory_aux_jobs SET next_attempt_at=clock_timestamp() WHERE job_id='index-job'`); e != nil {
		t.Fatal(e)
	}
	restarted := *r
	restarted.Jobs = jobs.New(r.Catalog.Pool(), r.Catalog, budget.NewController(r.Catalog.Pool(), budget.DefaultPolicy()), &jobs.LocalTarget{Endpoint: r.Target.Endpoint(), Identity: r.Target.Identity()})
	if ok, e := restarted.ProcessOne(context.Background()); !ok || e != nil {
		t.Fatal(ok, e)
	}
	if m.calls.Load() != 3 {
		t.Fatal("persisted vectors reinferred", m.calls.Load())
	}
	if e = r.Catalog.Pool().QueryRow(ctx, `SELECT count(*) FROM semantic_index_entries WHERE generation_id=$1`, a.GenerationID).Scan(&n); e != nil || n != 5 {
		t.Fatal(n, e)
	}
	var state string
	if e = r.Catalog.Pool().QueryRow(ctx, `SELECT status FROM memory_aux_jobs WHERE job_id='aaa-extraction'`).Scan(&state); e != nil || state != "pending" {
		t.Fatal(state, e)
	}
	proof := semantic.Proof{ArtifactRef: "fixture:reviewed-index-quality", ArtifactSHA256: strings.Repeat("a", 64), CorpusVersion: "fixture-v1", Cases: 12, BaselineTop1: .5, CandidateTop1: .75}
	if e = r.Review(ctx, a.GenerationID, "", proof); e != nil {
		t.Fatal("owner review", e)
	}
	if _, e = r.Catalog.Delete(ctx, src.ID, "delete", src.Revision); e != nil {
		t.Fatal(e)
	}
	if e = r.Catalog.Pool().QueryRow(ctx, `SELECT count(*) FROM semantic_index_entries`).Scan(&n); e != nil || n != 0 {
		t.Fatal("deleted vectors survived", n, e)
	}
}
func TestLateIndexResultAfterSourceDeletionCannotPublishIntegration(t *testing.T) {
	r, ctx, src, m := fixture(t)
	q := request(src)
	a, e := r.Enqueue(ctx, q)
	if e != nil {
		t.Fatal(e)
	}
	m.after = func() {
		if _, e := r.Catalog.Delete(ctx, src.ID, "delete", src.Revision); e != nil {
			t.Error(e)
		}
	}
	bounded, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if ok, e := r.ProcessOne(bounded); !ok || e == nil {
		t.Fatal("late index result accepted", ok, e)
	}
	var n int
	if e = r.Catalog.Pool().QueryRow(ctx, `SELECT count(*) FROM semantic_index_entries WHERE generation_id=$1`, a.GenerationID).Scan(&n); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	var state string
	if e = r.Catalog.Pool().QueryRow(ctx, `SELECT status FROM memory_aux_jobs WHERE job_id='index-job'`).Scan(&state); e != nil || state != "cancelled" {
		t.Fatal(state, e)
	}
}

func TestProjectStructuredIndexRenameReusesVectorsAndRefreshesLocatorIntegration(t *testing.T) {
	r, ctx, _, m := fixture(t)
	u := ingestion.Upload{ExternalKey: "project-source", ScopeType: "project", ScopeID: "project-a", Format: "go", Path: "old.go", RepositoryRevision: "rev1", Raw: []byte("package service\n// Restart сохраняет русский контекст.\nfunc Restart() {}\n")}
	ingest := &ingestion.Store{Catalog: r.Catalog}
	a, err := ingest.Ingest(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	q := request(a.Source)
	q.ScopeType, q.ScopeID = "project", "project-a"
	first, err := r.Enqueue(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := r.ProcessOne(ctx); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if m.calls.Load() != 1 {
		t.Fatal(m.calls.Load())
	}
	u.ExpectedRevision, u.Path = a.Source.Revision, "new.go"
	b, err := ingest.Ingest(ctx, u)
	if err != nil || b.Fragments.Unchanged != 1 {
		t.Fatal(b, err)
	}
	q = request(b.Source)
	q.Job.ID, q.ScopeType, q.ScopeID = "renamed-index", "project", "project-a"
	second, err := r.Enqueue(ctx, q)
	if err != nil || second.GenerationID == first.GenerationID {
		t.Fatal(second, err)
	}
	if ok, err := r.ProcessOne(ctx); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if m.calls.Load() != 1 {
		t.Fatal("citation-only rename repeated model inference", m.calls.Load())
	}
	guard, err := semantic.NewCatalogGuard(r.Catalog, "memory_read")
	if err != nil {
		t.Fatal(err)
	}
	store := semantic.NewStore(r.Catalog.Pool())
	store.Guard = guard
	hits, err := store.Lexical(ctx, semantic.Scope{Type: "project", ID: "project-a"}, "new.go")
	if err != nil || len(hits.Exact) != 1 || !strings.Contains(hits.Exact[0].Source.Content, "русский") || !strings.Contains(hits.Exact[0].Source.ProvenanceJSON, "new.go") {
		t.Fatal(hits, err)
	}
	old, err := store.Lexical(ctx, semantic.Scope{Type: "project", ID: "project-a"}, "old.go")
	if err != nil || len(old.Exact) != 0 {
		t.Fatal("stale path indexed", old, err)
	}
	foreign, err := store.Lexical(ctx, semantic.Scope{Type: "project", ID: "project-b"}, "Restart")
	if err != nil || len(foreign.Exact) != 0 {
		t.Fatal("project isolation", foreign, err)
	}
}
