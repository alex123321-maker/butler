package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/indexing"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/semantic"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
)

// One prospective contention slice, not a p95/quality/native-model claim. The
// first generation uses100 real embeddings, the second reuses those100 and
// embeds100 new chunks. A real ordinary foreground query overlaps that job.
func TestMemoryCapacityActualForegroundDuringOwnedIndex(t *testing.T) {
	if os.Getenv("BUTLER_MEMORY_CAPACITY_INTERACTIVE") != "1" {
		t.Skip("explicit actual local contention slice required")
	}
	pool := memorytest.Database(t)
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Minute)
	defer stop()
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "capacity-owner", Channel: "web", DeliveryID: "capacity-interactive"})
	report := os.Getenv("BUTLER_MEMORY_CAPACITY_REPORT")
	if !filepath.IsAbs(report) {
		t.Fatal("absolute proof directory required")
	}
	manifest := catalog.NewFileManifest(filepath.Join(report, "interactive-anchor"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(pool, manifest)
	policy := budget.DefaultPolicy()
	policy.Models["local:google/embeddinggemma-2"] = budget.ModelPrice{Basis: "local_no_billing"}
	policy.Models["antigravity:capacity-control"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	b := budget.NewController(pool, policy)
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	index, closeIndex := configureOwnedSemantic(log, cat, b)
	if index == nil {
		t.Fatal("pinned actual local composition required")
	}
	defer closeIndex()
	actual, ok := index.Backend.(*semantic.AuthenticatedBackend)
	if !ok {
		t.Fatal("actual authenticated backend required")
	}
	observed := &capacityLocalBackend{AuthenticatedBackend: actual}
	observation := map[string]any{"phase": "setup", "cloud_native_calls": 0, "quality": "not_measured", "foreground_slo": "one query <=5000ms; not p95"}
	defer func() {
		stop()
		observed.mu.Lock()
		observation["local_calls"] = append([]capacityLocalObservation{}, observed.Observations...)
		observed.mu.Unlock()
		retained, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		var ledger json.RawMessage
		if err := pool.QueryRow(retained, `SELECT jsonb_build_object('jobs',(SELECT jsonb_agg(to_jsonb(j))FROM memory_aux_jobs j),'reservations',(SELECT jsonb_agg(to_jsonb(r))FROM execution_budget_reservations r),'scopes',(SELECT jsonb_agg(to_jsonb(s))FROM execution_budget_scopes s),'foreground',(SELECT jsonb_agg(to_jsonb(m))FROM execution_model_attempts m))`).Scan(&ledger); err != nil {
			t.Error("complete local contention ledger unavailable", err)
		}
		observation["ledger"] = ledger
		raw, _ := json.MarshalIndent(observation, "", "  ")
		if err := os.WriteFile(filepath.Join(report, "actual-interactive-index.json"), raw, 0644); err != nil {
			t.Error(err)
		}
	}()
	index.Backend = observed
	index.Reader.Adapter.Backend = observed
	refs := []catalog.SourceRef{}
	makeSource := func(key string) {
		source, err := cat.Put(owner, catalog.PutSource{ExternalKey: key, Kind: "document", ScopeType: "user", ScopeID: "capacity-owner", Body: "Synthetic local concurrency source"})
		if err != nil {
			t.Fatal(err)
		}
		ref := catalog.SourceRef{ID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash}
		fragments := make([]catalog.Fragment, 100)
		for i := range fragments {
			fragments[i] = catalog.Fragment{Key: time.Unix(int64(i), 0).UTC().Format(time.RFC3339), Text: "Synthetic indexed observation for local concurrency; no user fact or authority."}
		}
		if _, err = cat.PublishFragments(owner, ref, fragments); err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	enqueue := func(id string, max int) indexing.Receipt {
		r, err := index.Enqueue(owner, indexing.Request{Job: jobs.Request{ID: id, Kind: "index", Purpose: "memory_read", Sources: refs, Allowance: budget.Units{ModelTurns: 64, Tokens: 800000, ActiveMillis: 3900000, ProviderInputBytes: 524288}, Deadline: time.Now().Add(4 * time.Minute)}, ScopeType: "user", ScopeID: "capacity-owner", MaxChunks: max})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	makeSource("capacity-active")
	first := enqueue("capacity-contention-initial", 100)
	observation["phase"] = "initial_index"
	if worked, err := index.ProcessOne(ctx); !worked || err != nil {
		t.Fatal("initial actual index failed", err)
	}
	quality, err := os.ReadFile("../../../../deploy/local-models/verification-retrieval-quality.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = index.Review(owner, first.GenerationID, "", semantic.Proof{ArtifactRef: "deploy/local-models/verification-retrieval-quality.json", ArtifactSHA256: acceptance.MemoryBytesSHA256(quality), CorpusVersion: "retrieval-quality-v1", Cases: 24, BaselineTop1: 11.0 / 24, CandidateTop1: 22.0 / 24}); err != nil {
		t.Fatal(err)
	}
	makeSource("capacity-background")
	second := enqueue("capacity-contention-background", 200)
	observation["generation_id"] = second.GenerationID
	if _, err = pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel)VALUES('capacity-foreground','capacity-owner','web');INSERT INTO tasks(task_id,session_key,owner_id,goal,working_json)VALUES('capacity-task','capacity-foreground','capacity-owner','Наблюдать доступную память без внешних действий','{"scratch":{"tool_effect":"unknown"}}')`); err != nil {
		t.Fatal(err)
	}
	observed.Started = make(chan string, 8)
	type indexOutcome struct {
		worked bool
		err    error
	}
	background := make(chan indexOutcome, 1)
	go func() { worked, err := index.ProcessOne(ctx); background <- indexOutcome{worked, err} }()
	select {
	case caller := <-observed.Started:
		if caller != "memory-index" {
			t.Fatal("unexpected first local request")
		}
	case <-ctx.Done():
		t.Fatal("background IPC not entered")
	}
	p := &scopedCandidateProvider{}
	finals := completion.New(pool)
	finals.DataGuard = cat.CheckRunSourcesTx
	executor := flow.NewService(session.NewPostgresRepository(pool), session.NewPostgresLeaseManager(pool), runservice.NewService(runservice.NewPostgresRepository(pool), nil), transcript.NewStore(pool), p, flow.Config{ProviderName: p.Name(), ModelName: "capacity-control", OwnerID: "capacity-worker", LeaseTTL: 120, Tasks: task.NewStore(pool), Budgets: b, MemorySources: cat, SemanticMemory: index.Reader, Completion: finals, ContextBudgetTokens: 65536}, log)
	payload, _ := json.Marshal(map[string]string{"text": "Какие наблюдения доступны в памяти?", "user_id": "capacity-owner", "task_id": "capacity-task"})
	started := time.Now()
	observation["phase"] = "foreground_with_background_index"
	result, foregroundErr := executor.Execute(owner, ingress.InputEvent{EventID: "capacity-foreground-input", IdempotencyKey: "capacity-foreground-input", SessionKey: "capacity-foreground", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload), CreatedAt: time.Now()})
	foregroundMillis := time.Since(started).Milliseconds()
	bg := <-background
	observed.mu.Lock()
	calls := append([]capacityLocalObservation{}, observed.Observations...)
	observed.mu.Unlock()
	observation["foreground_millis"] = foregroundMillis
	observation["foreground_completed"] = foregroundErr == nil
	observation["background_completed"] = bg.worked && bg.err == nil
	if foregroundErr != nil || result == nil || foregroundMillis > 5000 {
		t.Fatal("foreground contention gate failed", foregroundMillis, foregroundErr)
	}
	if !bg.worked || bg.err != nil {
		t.Fatal("background index failed during foreground", bg.err)
	}
	input, _ := json.Marshal(p.last.InputItems)
	if !strings.Contains(string(input), "Synthetic indexed observation") || !strings.Contains(string(input), "tool_effect") || !strings.Contains(string(input), "unknown") {
		t.Fatal("actual local evidence or typed current state absent from foreground")
	}
	queries, overlap := 0, 0
	for _, query := range calls {
		if query.Caller != "memory-query" {
			continue
		}
		queries++
		for _, batch := range calls {
			if batch.Caller == "memory-index" && batch.StartedNanos < query.FinishedNanos && query.StartedNanos < batch.FinishedNanos {
				overlap++
			}
		}
	}
	if queries != 1 || overlap < 1 {
		t.Fatal("missing actual query/index request overlap", queries, overlap)
	}
	observation["actual_query_calls"] = queries
	observation["overlapping_index_requests"] = overlap
	observation["phase"] = "completed"
}
