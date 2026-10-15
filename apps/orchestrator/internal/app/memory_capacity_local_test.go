package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/indexing"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/semantic"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
)

type capacityLocalObservation struct {
	Items           int    `json:"items"`
	Millis          int64  `json:"millis"`
	QueueMillis     uint64 `json:"queue_millis"`
	ExecutionMillis uint64 `json:"execution_millis"`
	PeakRAMMiB      uint64 `json:"peak_ram_mib"`
	PeakDeviceMiB   uint64 `json:"peak_device_mib"`
	Completed       bool   `json:"completed"`
}
type capacityLocalBackend struct {
	*semantic.AuthenticatedBackend
	mu           sync.Mutex
	Observations []capacityLocalObservation
}

func (b *capacityLocalBackend) Infer(ctx context.Context, r *pb.InferRequest) (*pb.InferResponse, error) {
	start := time.Now()
	out, err := b.AuthenticatedBackend.Infer(ctx, r)
	o := capacityLocalObservation{Items: len(r.GetEmbed().GetTexts()), Millis: time.Since(start).Milliseconds(), Completed: err == nil && out != nil}
	if out != nil {
		o.QueueMillis = out.QueueMs
		o.ExecutionMillis = out.ExecutionMs
		o.PeakRAMMiB = out.PeakRamMib
		o.PeakDeviceMiB = out.PeakDeviceMib
	}
	b.mu.Lock()
	b.Observations = append(b.Observations, o)
	b.mu.Unlock()
	return out, err
}

// Actual pinned local embedding/indexing, with no cloud credentials. This is
// separate from the bulk synthetic vector screen and from semantic QA quality.
func TestMemoryCapacityActualLocalIndexAndReuse(t *testing.T) {
	if os.Getenv("BUTLER_MEMORY_CAPACITY_ACTUAL_LOCAL") != "1" {
		t.Skip("explicit actual local capacity slice required")
	}
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "capacity-index-owner", Channel: "web", DeliveryID: "actual-local-index"})
	report := os.Getenv("BUTLER_MEMORY_CAPACITY_REPORT")
	if !filepath.IsAbs(report) {
		t.Fatal("absolute report required")
	}
	manifest := catalog.NewFileManifest(filepath.Join(report, "local-anchor"))
	if manifest.Initialize() != nil {
		t.Fatal("fresh anchor required")
	}
	cat := catalog.New(pool, manifest)
	policy := budget.DefaultPolicy()
	policy.Models["local:google/embeddinggemma-2"] = budget.ModelPrice{Basis: "local_no_billing"}
	b := budget.NewController(pool, policy)
	index, closeIndex := configureOwnedSemantic(slog.New(slog.NewTextHandler(io.Discard, nil)), cat, b)
	if index == nil {
		t.Fatal("actual pinned local composition unavailable")
	}
	defer closeIndex()
	actual, ok := index.Backend.(*semantic.AuthenticatedBackend)
	if !ok {
		t.Fatal("actual local backend required")
	}
	observed := &capacityLocalBackend{AuthenticatedBackend: actual}
	index.Backend = observed
	fragments := make([]catalog.Fragment, 100)
	for i := range fragments {
		fragments[i] = catalog.Fragment{Key: time.Unix(int64(i), 0).UTC().Format(time.RFC3339), Text: "Synthetic indexed observation for resource measurement, not a user assertion.", Locator: json.RawMessage(`{}`)}
	}
	source, err := cat.Put(owner, catalog.PutSource{ExternalKey: "actual-index-capacity", Kind: "document", ScopeType: "user", ScopeID: "capacity-index-owner", Title: "Synthetic local-index source", Body: "One hundred explicitly synthetic fragments for indexing throughput.", Origin: content.Origin{Kind: content.Document, Sensitivity: content.Public}})
	if err != nil {
		t.Fatal(err)
	}
	ref := catalog.SourceRef{ID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash}
	if _, err = cat.PublishFragments(owner, ref, fragments); err != nil {
		t.Fatal(err)
	}
	results := []map[string]any{}
	defer func() {
		raw, _ := json.MarshalIndent(map[string]any{"generations": results, "actual_local_calls": observed.Observations, "cloud_calls": 0, "quality_claim": "none; existing profile proof only"}, "", "  ")
		if os.WriteFile(filepath.Join(report, "actual-local-index.json"), append(raw, '\n'), 0644) != nil {
			t.Error("local report unavailable")
		}
	}()
	previous := ""
	for i := 0; i < 2; i++ {
		id := []string{"capacity-actual-initial", "capacity-actual-reuse"}[i]
		start := time.Now()
		r, err := index.Enqueue(owner, indexing.Request{Job: jobs.Request{ID: id, Kind: "index", Purpose: "memory_read", Sources: []catalog.SourceRef{ref}, Allowance: budget.Units{ModelTurns: 32, Tokens: 400000, ActiveMillis: 1920000, ProviderInputBytes: 262144}, Deadline: time.Now().Add(10 * time.Minute)}, ScopeType: "user", ScopeID: "capacity-index-owner", MaxChunks: 100})
		if err != nil {
			t.Fatal("actual index enqueue", err)
		}
		observed.mu.Lock()
		before := len(observed.Observations)
		observed.mu.Unlock()
		worked, err := index.ProcessOne(ctx)
		elapsed := time.Since(start).Milliseconds()
		observed.mu.Lock()
		after := len(observed.Observations)
		observed.mu.Unlock()
		results = append(results, map[string]any{"generation": r.GenerationID, "job_id": id, "milliseconds": elapsed, "additional_inference_calls": after - before, "completed": worked && err == nil})
		if err != nil || !worked {
			t.Fatal("actual owned indexing failed")
		}
		if i == 0 && after-before != 25 {
			t.Fatal("unexpected initial actual batch count", after-before)
		}
		if i == 1 && after != before {
			t.Fatal("exact-space vector reuse made a model call")
		}
		raw, err := os.ReadFile("../../../../deploy/local-models/verification-retrieval-quality.json")
		if err != nil {
			t.Fatal(err)
		}
		if err = index.Review(owner, r.GenerationID, previous, semantic.Proof{ArtifactRef: "deploy/local-models/verification-retrieval-quality.json", ArtifactSHA256: acceptance.MemoryBytesSHA256(raw), CorpusVersion: "retrieval-quality-v1", Cases: 24, BaselineTop1: 11.0 / 24, CandidateTop1: 22.0 / 24}); err != nil {
			t.Fatal("actual index review/CAS", err)
		}
		previous = r.GenerationID
	}
	var ledger json.RawMessage
	if err = pool.QueryRow(ctx, `SELECT jsonb_build_object('scopes',(SELECT jsonb_agg(to_jsonb(s))FROM execution_budget_scopes s),'reservations',(SELECT jsonb_agg(to_jsonb(r))FROM execution_budget_reservations r),'jobs',(SELECT jsonb_agg(to_jsonb(j)-ARRAY['sources','authority','claim_id'])FROM memory_aux_jobs j))`).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if os.WriteFile(filepath.Join(report, "actual-local-ledger.json"), append(ledger, '\n'), 0644) != nil {
		t.Fatal("local ledger unavailable")
	}
}
