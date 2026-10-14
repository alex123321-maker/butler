package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/indexing"
	"github.com/butler/butler/internal/memory/ingestion"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/semantic"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

const memoryEvalPurpose = "memory-eval-78"

// The additive collector uses the same source catalog, owned indexing jobs,
// hybrid reader, bundle builder, model budget controller and transport as Butler.
// It runs only in a fresh explicit test DB; it cannot become a production service.
type memoryEvalCollector struct {
	pool                *pgxpool.Pool
	catalog             *catalog.Store
	budgets             *budget.Controller
	backend             semantic.Backend
	factory             func(json.RawMessage) (transport.ModelProvider, error)
	suite               acceptance.MemorySuite
	plan                acceptance.MemoryPlan
	private             string
	frozen              acceptance.MemoryFrozen
	measurements        acceptance.MemoryMeasurements
	captures            []acceptance.MemoryCapture
	sourceRefs          map[string]map[string]catalog.SourceRef
	started             time.Time
	closed              bool
	captureMilliseconds map[string]int64
	retainedBytes       int64
	prior               *acceptance.MemoryPrior
	priorPins           *acceptance.MemoryPriorPins
	priorDirectory      string
}

type memoryEvalBackend struct {
	collector *memoryEvalCollector
	inner     semantic.Backend
	caseID    string
}

func (b *memoryEvalBackend) Describe(ctx context.Context, e *pb.Empty) (*pb.Catalog, error) {
	return b.inner.Describe(ctx, e)
}
func (b *memoryEvalBackend) Infer(ctx context.Context, r *pb.InferRequest) (*pb.InferResponse, error) {
	// The collector is deliberately serial. Bind the single unbound owning
	// reservation to the exact actual IPC before it can spend resources.
	var reservation string
	var count int
	if err := b.collector.pool.QueryRow(ctx, `SELECT count(*),COALESCE(min(reservation_id),'') FROM execution_budget_reservations e WHERE owner_id=$1 AND (operation_id LIKE 'aux:memory-index:%' OR operation_id LIKE 'local:%') AND NOT EXISTS(SELECT 1 FROM fixture_memory_eval_aux a WHERE a.reservation_id=e.reservation_id)`, "memory-eval-owner:"+b.caseID).Scan(&count, &reservation); err != nil || count != 1 {
		return nil, errors.New("exact owning auxiliary reservation required")
	}
	if err := b.collector.reserveCall(ctx, max(int64(len(r.String()))+8192, 2048*int64(len(r.GetEmbed().GetTexts())+1))); err != nil {
		return nil, err
	}
	requestRaw, _ := json.Marshal(r)
	if _, err := b.collector.pool.Exec(ctx, `INSERT INTO fixture_memory_eval_aux(request_id,case_id,reservation_id,request_hash) VALUES($1,$2,$3,$4)`, r.RequestId, b.caseID, reservation, hashMemoryBytes(requestRaw)); err != nil {
		return nil, err
	}
	start := time.Now()
	result, err := b.inner.Infer(ctx, r)
	elapsed := time.Since(start).Milliseconds()
	zero := int64(0)
	outcome := "success"
	if err != nil {
		outcome = "failed"
	}
	// The current worker exposes execution/queue/RAM but no tokenizer usage.
	// That limitation remains NULL, never an invented zero token count.
	e := acceptance.MemoryExpense{ID: "aux:" + r.RequestId, CaseID: b.caseID, Arm: "retrieval", Stage: "embedding", Status: "unknown", Outcome: outcome, EvidenceSHA256: acceptance.MemoryDigest(struct {
		RequestID, Profile, Revision string
		Elapsed                      int64
	}{r.RequestId, r.ProfileId, result.GetRevision(), elapsed}), Calls: 1, OutputTokens: &zero, Milliseconds: &elapsed, Microunits: &zero, StorageBytes: &zero}
	retain, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	responseRaw, _ := json.Marshal(result)
	if _, recordErr := b.collector.pool.Exec(retain, `UPDATE fixture_memory_eval_aux SET response_hash=$2,outcome=$3 WHERE request_id=$1 AND outcome='reserved'`, r.RequestId, hashMemoryBytes(responseRaw), outcome); recordErr != nil {
		return nil, recordErr
	}
	if recordErr := b.collector.recordExpense(retain, e); recordErr != nil {
		return nil, recordErr
	}
	if recordErr := b.collector.writeEvidence("aux-"+r.RequestId+".json", struct {
		Request, Response json.RawMessage
		Outcome           string
	}{requestRaw, responseRaw, outcome}); recordErr != nil {
		return nil, recordErr
	}
	return result, err
}

// IDs/revisions are namespace aliases only. Selection, excerpts, ordering and
// generation/space identity come from actual current HybridReader/BuildBundle.
type memoryEvalReader struct {
	collector *memoryEvalCollector
	reader    *semantic.HybridReader
	caseID    string
	originals map[int64]acceptance.MemorySource
}

func (r *memoryEvalReader) Search(ctx context.Context, scopes []memoryservice.Scope, q string, limit int) ([]memoryservice.SemanticHit, error) {
	report, err := r.reader.SearchDetailed(ctx, scopes, q, limit)
	if err != nil {
		return nil, err
	}
	if err = r.collector.writeEvidence("search-"+strings.TrimPrefix(r.caseID, "memory:")+".json", report); err != nil {
		return nil, err
	}
	for n, h := range report.Hits {
		id, parseErr := strconv.ParseInt(strings.TrimPrefix(h.SourceRef, "memory_chunk:"), 10, 64)
		original, ok := r.originals[id]
		if parseErr != nil || !ok {
			return nil, errors.New("retrieval original binding missing")
		}
		report.Hits[n].SourceRef = original.ID
		report.Hits[n].Revision = strconv.FormatInt(original.Revision, 10)
	}
	return report.Hits, nil
}

func memoryEvalSpace() semantic.Space {
	return semantic.Space{ProfileID: "embeddinggemma2-text-bf16-v1", Model: "google/embeddinggemma-2", Revision: "914f7f89142e33e77833254d9c9b90c3cef7303b", Preprocessing: "embeddinggemma2-retrieval-v1", RuntimeVersion: "torch2.10.0+cu128/transformers5.19.0/sentence-transformers6.1.0", ChunkVersion: "structured-go-v1", Dimensions: 768}
}
func memoryEvalConfigDigest() string {
	return acceptance.MemoryDigest(struct {
		Space                                 semantic.Space
		ChunkLimit, BundleBudget              int
		Rerank, Extract, Summary, Consolidate bool
	}{memoryEvalSpace(), 2, 12, false, false, false, false})
}

func (c *memoryEvalCollector) init(ctx context.Context) error {
	ddl := `CREATE TABLE fixture_memory_eval_window(singleton boolean PRIMARY KEY CHECK(singleton),plan_hash text NOT NULL,calls bigint NOT NULL DEFAULT 0,reserved_units bigint NOT NULL DEFAULT 0,max_calls bigint NOT NULL,expires_at timestamptz NOT NULL,closed boolean NOT NULL DEFAULT false,prior_calls bigint NOT NULL DEFAULT 0,prior_reserved_units bigint NOT NULL DEFAULT 0);
CREATE TABLE fixture_memory_eval_expenses(id text PRIMARY KEY,case_id text NOT NULL,arm text NOT NULL,stage text NOT NULL,record jsonb NOT NULL);
CREATE TABLE fixture_memory_eval_attempts(case_id text NOT NULL,arm text NOT NULL,run_id text NOT NULL UNIQUE,input_hash text NOT NULL,operation_id text UNIQUE,output_hash text,outcome text NOT NULL DEFAULT 'reserved',input_tokens bigint,output_tokens bigint,total_tokens bigint,model_version text,milliseconds bigint,PRIMARY KEY(case_id,arm));
CREATE TABLE fixture_memory_eval_aux(request_id text PRIMARY KEY,case_id text NOT NULL,reservation_id text UNIQUE NOT NULL,request_hash text NOT NULL,response_hash text,outcome text NOT NULL DEFAULT 'reserved');
CREATE TABLE fixture_memory_eval_guards(id bigserial PRIMARY KEY,run_id text NOT NULL,case_id text NOT NULL,arm text NOT NULL,operation_id text NOT NULL,phase text NOT NULL,checked_at timestamptz NOT NULL,task_revision bigint NOT NULL,constraints_revision bigint NOT NULL,lease_id text NOT NULL,lease_expires timestamptz NOT NULL,sources jsonb NOT NULL);
CREATE TABLE fixture_memory_eval_results(case_id text NOT NULL,arm text NOT NULL,input_hash text NOT NULL,output_hash text NOT NULL,assessment jsonb NOT NULL,task_scope text NOT NULL,task_revision bigint NOT NULL,PRIMARY KEY(case_id,arm));`
	ddl += `CREATE FUNCTION fixture_memory_eval_append_only() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'immutable original evidence'; END $$;
CREATE TRIGGER memory_expenses_immutable BEFORE UPDATE OR DELETE ON fixture_memory_eval_expenses FOR EACH ROW EXECUTE FUNCTION fixture_memory_eval_append_only();
CREATE TRIGGER memory_results_immutable BEFORE UPDATE OR DELETE ON fixture_memory_eval_results FOR EACH ROW EXECUTE FUNCTION fixture_memory_eval_append_only();
CREATE TRIGGER memory_guards_immutable BEFORE UPDATE OR DELETE ON fixture_memory_eval_guards FOR EACH ROW EXECUTE FUNCTION fixture_memory_eval_append_only();
CREATE FUNCTION fixture_memory_eval_window_guard() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN
IF OLD.closed OR NEW.plan_hash<>OLD.plan_hash OR NEW.max_calls<>OLD.max_calls OR NEW.expires_at<>OLD.expires_at OR NEW.prior_calls<>OLD.prior_calls OR NEW.prior_reserved_units<>OLD.prior_reserved_units OR NEW.calls<OLD.calls OR NEW.reserved_units<OLD.reserved_units THEN RAISE EXCEPTION 'sealed window'; END IF; RETURN NEW; END $$;
CREATE TRIGGER memory_window_guard BEFORE UPDATE ON fixture_memory_eval_window FOR EACH ROW EXECUTE FUNCTION fixture_memory_eval_window_guard();
CREATE TRIGGER memory_window_no_delete BEFORE DELETE ON fixture_memory_eval_window FOR EACH ROW EXECUTE FUNCTION fixture_memory_eval_append_only();`
	if _, err := c.pool.Exec(ctx, ddl); err != nil {
		return errors.New("fresh collector database required")
	}
	priorCalls, priorUnits := int64(0), int64(0)
	if c.priorPins != nil {
		priorCalls, priorUnits = c.priorPins.Calls, c.priorPins.ReservedUnits
	}
	_, err := c.pool.Exec(ctx, `INSERT INTO fixture_memory_eval_window VALUES(TRUE,$1,$4,$5,$2,$3,FALSE,$4,$5)`, acceptance.MemoryDigest(c.plan), acceptance.MemorySuitePolicy(c.suite).MaxCalls, c.started.Add(3*time.Hour), priorCalls, priorUnits)
	return err
}

func (c *memoryEvalCollector) recordExpense(ctx context.Context, e acceptance.MemoryExpense) error {
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if _, err = c.pool.Exec(ctx, `INSERT INTO fixture_memory_eval_expenses VALUES($1,$2,$3,$4,$5)`, e.ID, e.CaseID, e.Arm, e.Stage, raw); err != nil {
		return err
	}
	c.measurements.Expenses = append(c.measurements.Expenses, e)
	return nil
}
func (c *memoryEvalCollector) writeEvidence(name string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return c.writeOriginal(name, raw)
}

func (c *memoryEvalCollector) writeOriginal(name string, raw []byte) error {
	if len(raw) > 16<<20 || c.retainedBytes+int64(len(raw)) > 128<<20 {
		return errors.New("finite original report capacity exhausted")
	}
	path := filepath.Join(c.private, name)
	err := writeShadowV5ExactBytes(path, raw)
	if st, statErr := os.Stat(path); statErr == nil && st.Mode().IsRegular() {
		c.retainedBytes += st.Size()
	}
	return err
}

func (c *memoryEvalCollector) owner(cse acceptance.MemoryCase) (string, context.Context) {
	owner := "memory-eval-owner:" + acceptance.MemoryOpaqueCaseID(cse)
	return owner, owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: owner, Channel: "web", DeliveryID: "memory-eval-explicit-owner"})
}

func memoryEvalPolicy() budget.Policy {
	p := budget.DefaultPolicy()
	p.Task.ModelTurns = 128
	p.Day.ModelTurns = 280
	p.Task.Tokens = 4000000
	p.Day.Tokens = 4000000
	p.Conversation.Tokens = 4000000
	p.OutputTokens = 32768
	p.ModelInputBytes = 32768
	p.Models["antigravity:gemini-3.8-flash-high"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	p.Models["local:google/embeddinggemma-2"] = budget.ModelPrice{Basis: "local_no_billing"}
	return p
}
func (c *memoryEvalCollector) ownedRun(ctx context.Context, cse acceptance.MemoryCase, arm string) (context.Context, string, error) {
	owner, _ := c.owner(cse)
	id := acceptance.MemoryOpaqueCaseID(cse)
	run := "memory-run-" + acceptance.MemoryDigest([]string{c.plan.WindowID, id, arm})[:32]
	session := run + ":session"
	task := id + ":task"
	revision := max(int64(1), cse.Task.Revision)
	_, err := c.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,$2,'web');`, session, owner)
	if err != nil {
		return nil, "", err
	}
	goal := cse.Task.Goal
	if goal == "" {
		goal = cse.Question
	}
	constraints, _ := json.Marshal([]string{cse.Task.Instruction})
	_, err = c.pool.Exec(ctx, `INSERT INTO tasks(task_id,session_key,owner_id,goal,revision,constraints_revision,constraints_json,working_json) VALUES($1,$2,$3,$4,$5,$5,$6,$7) ON CONFLICT(task_id) DO NOTHING`, task, session, owner, goal, revision, constraints, json.RawMessage(`{"plan":[],"pending_actions":[],"scratch":{}}`))
	if err != nil {
		return nil, "", err
	}
	_, err = c.pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at,task_id,task_revision) VALUES($1,$2,$1,'running','mode_1','model_running','antigravity',clock_timestamp(),clock_timestamp(),$3,$4);`, run, session, task, revision)
	if err != nil {
		return nil, "", err
	}
	_, err = c.pool.Exec(ctx, `INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES($1,$2,$2,1,$3)`, session, run, c.started.Add(3*time.Hour))
	if err != nil {
		return nil, "", err
	}
	return ownership.WithToken(ctx, nil, ownership.Token{SessionKey: session, RunID: run, LeaseID: run, FenceEpoch: 1}), run, nil
}

func (c *memoryEvalCollector) capture(ctx context.Context, cse acceptance.MemoryCase) (err error) {
	if cse.Blocked() {
		return nil
	}
	start := time.Now()
	id := acceptance.MemoryOpaqueCaseID(cse)
	stage := "write"
	defer func() {
		if c.captureMilliseconds == nil {
			c.captureMilliseconds = map[string]int64{}
		}
		c.captureMilliseconds[id] = time.Since(start).Milliseconds()
		if err != nil {
			retention, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			var count int
			if c.pool.QueryRow(retention, `SELECT count(*) FROM fixture_memory_eval_expenses WHERE case_id=$1 AND stage=$2`, id, stage).Scan(&count) == nil && count == 0 {
				zero := int64(0)
				elapsed := time.Since(start).Milliseconds()
				_ = c.recordExpense(retention, acceptance.MemoryExpense{ID: id + "/" + stage + "/failed-prefix", CaseID: id, Arm: "retrieval", Stage: stage, Status: "unknown", Outcome: "failed", EvidenceSHA256: acceptance.MemoryDigest([]string{id, stage, "failed-prefix"}), InputTokens: &zero, OutputTokens: &zero, TotalTokens: &zero, Milliseconds: &elapsed, Microunits: &zero, StorageBytes: nil})
			}
		}
	}()
	owner, actor := c.owner(cse)
	a, _ := owneridentity.FromContext(actor)
	actor = owneridentity.WithActor(ctx, a)
	refs := map[string]catalog.SourceRef{}
	logical := map[string]acceptance.MemorySource{}
	allRefs := []catalog.SourceRef{}
	for _, h := range cse.History {
		if h.Status == "deleted" {
			// Content-free fixture placeholder exercises the actual deletion fence;
			// it never reconstructs the unknown deleted contact or its derivatives.
			src, err := c.catalog.Put(actor, catalog.PutSource{ExternalKey: h.ID, Kind: "document", ScopeType: "user", ScopeID: owner, Title: h.ID, Body: "synthetic deletion lifecycle placeholder; no original value", Origin: content.Origin{Kind: content.Document, Sensitivity: content.Public}})
			if err != nil {
				return err
			}
			deletion, err := c.catalog.Delete(actor, src.ID, "delete", src.Revision)
			if err != nil {
				return err
			}
			if err = c.writeEvidence("deletion-"+strings.TrimPrefix(id, "memory:")+"-"+h.ID+".json", deletion); err != nil {
				return err
			}
			continue
		}
		event, _ := time.Parse(time.RFC3339, h.EventUTC)
		src, err := c.catalog.Put(actor, catalog.PutSource{ExternalKey: h.ID, Kind: "document", ScopeType: "user", ScopeID: owner, Title: h.ID, Body: h.Body, Author: "synthetic-reviewed-owner", EventTime: &event, Origin: content.Origin{Kind: content.Document, Sensitivity: content.Public, SourceRefs: []string{h.ID}}})
		if err != nil {
			return err
		}
		ref := catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
		refs[h.ID] = ref
		logical[src.ID] = h
		allRefs = append(allRefs, ref)
		parsed, err := (ingestion.Parser{}).Parse(ctx, ingestion.Input{Format: "text", Path: h.ID + ".txt", Raw: []byte(h.Body)})
		if err != nil {
			return err
		}
		if _, err = c.catalog.PublishFragments(actor, ref, parsed.Fragments); err != nil {
			return err
		}
	}
	c.sourceRefs[id] = refs
	if err = c.writeEvidence("bindings-"+strings.TrimPrefix(id, "memory:")+".json", refs); err != nil {
		return err
	}
	writeMS := time.Since(start).Milliseconds()
	zero := int64(0)
	storage := int64(0)
	if err := c.pool.QueryRow(ctx, `SELECT COALESCE((SELECT sum(pg_column_size(r)) FROM memory_source_revisions r JOIN memory_sources s ON s.source_id=r.source_id WHERE s.owner_id=$1),0)+COALESCE((SELECT sum(pg_column_size(c)) FROM memory_chunks c WHERE scope_type='user' AND scope_id=$1),0)`, owner).Scan(&storage); err != nil {
		return err
	}
	writeID := id + "/write"
	if c.prior != nil {
		writeID = id + "/rebase-write"
	}
	if err := c.recordExpense(ctx, acceptance.MemoryExpense{ID: writeID, CaseID: id, Arm: "retrieval", Stage: "write", Status: "measured", Outcome: "success", EvidenceSHA256: acceptance.MemoryDigest(allRefs), Calls: 1, InputTokens: &zero, OutputTokens: &zero, TotalTokens: &zero, Milliseconds: &writeMS, Microunits: &zero, StorageBytes: &storage}); err != nil {
		return err
	}
	if c.prior != nil {
		return c.reuseCapture(ctx, id, refs)
	}
	live, run, err := c.ownedRun(ctx, cse, "retrieval-capture")
	if err != nil {
		return err
	}
	target, err := budget.NewLocalTarget("local-models:9090", "operator-pinned-service")
	if err != nil {
		return err
	}
	guard, err := semantic.NewCatalogGuard(c.catalog, memoryEvalPurpose)
	if err != nil {
		return err
	}
	s := semantic.NewStore(c.pool)
	s.Guard = guard
	traced := &memoryEvalBackend{c, c.backend, id}
	if len(allRefs) > 0 {
		stage = "background"
		js := jobs.New(c.pool, c.catalog, c.budgets, &jobs.LocalTarget{Endpoint: target.Endpoint(), Identity: target.Identity()})
		runtime := &indexing.Runtime{Catalog: c.catalog, Jobs: js, Backend: traced, Target: target, Space: memoryEvalSpace(), Price: budget.ModelPrice{Basis: "local_no_billing"}}
		begin := time.Now()
		receipt, err := runtime.Enqueue(actor, indexing.Request{Job: jobs.Request{ID: "memory-index:" + id, Kind: "index", Purpose: memoryEvalPurpose, TaskID: id + ":task", OriginalRunID: run, Sources: allRefs, Allowance: budget.Units{ModelTurns: 64, Tokens: 1000000, ActiveMillis: 600000, ProviderInputBytes: 2000000}, Deadline: c.started.Add(3 * time.Hour)}, ScopeType: "user", ScopeID: owner, MaxChunks: 128})
		if err != nil {
			return err
		}
		if worked, err := runtime.ProcessOne(ctx); !worked || err != nil {
			return errors.New("owned index job failed")
		}
		proofRaw, err := os.ReadFile(filepath.Join(shadowRepoRoot(), "deploy/local-models/verification-retrieval-quality.json"))
		if err != nil {
			return err
		}
		proof := semantic.Proof{ArtifactRef: "deploy/local-models/verification-retrieval-quality.json", ArtifactSHA256: hashMemoryBytes(proofRaw), CorpusVersion: "retrieval-quality-v1", Cases: 24, BaselineTop1: 11.0 / 24, CandidateTop1: 22.0 / 24}
		if err = runtime.Review(actor, receipt.GenerationID, "", proof); err != nil {
			return err
		}
		elapsed := time.Since(begin).Milliseconds()
		var indexBytes int64
		if err = c.pool.QueryRow(ctx, `SELECT COALESCE(sum(pg_column_size(e)),0) FROM semantic_index_entries e WHERE generation_id=$1`, receipt.GenerationID).Scan(&indexBytes); err != nil {
			return err
		}
		if err = c.recordExpense(ctx, acceptance.MemoryExpense{ID: id + "/background-index", CaseID: id, Arm: "retrieval", Stage: "background", Status: "measured", Outcome: "success", EvidenceSHA256: acceptance.MemoryDigest(receipt), InputTokens: &zero, OutputTokens: &zero, TotalTokens: &zero, Milliseconds: &elapsed, Microunits: &zero, StorageBytes: &indexBytes}); err != nil {
			return err
		}
	}
	gate, err := semantic.NewLiveAdmission(c.budgets, guard, target)
	if err != nil {
		return err
	}
	reader := &semantic.HybridReader{Store: s, Adapter: &semantic.Adapter{Backend: traced, Space: memoryEvalSpace(), Admission: gate}, Rerank: false, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	originals := map[int64]acceptance.MemorySource{}
	rows, err := c.pool.Query(ctx, `SELECT object_id,source_id FROM memory_object_sources WHERE memory_type='chunk' AND source_id=ANY($1)`, func() []string {
		out := []string{}
		for _, ref := range allRefs {
			out = append(out, ref.ID)
		}
		return out
	}())
	if err != nil {
		return err
	}
	for rows.Next() {
		var chunk int64
		var src string
		if err = rows.Scan(&chunk, &src); err != nil {
			rows.Close()
			return err
		}
		originals[chunk] = logical[src]
	}
	rows.Close()
	bridge := &memoryEvalReader{c, reader, id, originals}
	builder := memoryservice.New(memoryservice.Config{Semantic: bridge, ChunkLimit: 2, BundleBudget: 12, ScopeOrder: []string{"user"}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	begin := time.Now()
	stage = "read"
	bundle, err := builder.BuildBundle(live, memoryservice.BundleRequest{SessionKey: run + ":session", UserID: owner, UserMessage: cse.Question, IncludeQuery: true})
	if err != nil {
		return err
	}
	ids := []string{}
	if chunks, ok := bundle.Items["chunks"].([]map[string]any); ok {
		for _, h := range chunks {
			ref := h["source_ref"].(string)
			seen := false
			for _, old := range ids {
				if ref == old {
					seen = true
				}
			}
			if !seen {
				ids = append(ids, ref)
			}
		}
	}
	cap := acceptance.MemoryCapture{CaseID: id, ConfigSHA256: c.plan.MemoryConfigSHA256, EvidenceSHA256: acceptance.MemoryDigest(bundle), SourceIDs: ids, Prompt: bundle.Prompt}
	if err = c.writeEvidence("bundle-"+strings.TrimPrefix(id, "memory:")+".json", bundle); err != nil {
		return err
	}
	c.captures = append(c.captures, cap)
	elapsed := time.Since(begin).Milliseconds()
	if err = c.recordExpense(ctx, acceptance.MemoryExpense{ID: id + "/read", CaseID: id, Arm: "retrieval", Stage: "read", Status: "measured", Outcome: "success", EvidenceSHA256: cap.EvidenceSHA256, Calls: 1, InputTokens: &zero, OutputTokens: &zero, TotalTokens: &zero, Milliseconds: &elapsed, Microunits: &zero, StorageBytes: &zero}); err != nil {
		return err
	}
	_, err = c.pool.Exec(ctx, `UPDATE runs SET current_state='completed',status='completed',finished_at=clock_timestamp() WHERE run_id=$1`, run)
	return err
}

func (c *memoryEvalCollector) fillUnused(ctx context.Context, i acceptance.MemoryInput) error {
	for _, stage := range acceptance.MemoryExpenseStages() {
		carried := false
		if c.prior != nil {
			for _, e := range c.prior.Measurements.Expenses {
				if e.CaseID == i.CaseID && e.Arm == i.Arm && e.Stage == stage {
					carried = true
					break
				}
			}
		}
		if carried {
			continue
		}
		var count int
		if err := c.pool.QueryRow(ctx, `SELECT count(*) FROM fixture_memory_eval_expenses WHERE case_id=$1 AND arm=$2 AND stage=$3`, i.CaseID, i.Arm, stage).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		zero := int64(0)
		e := acceptance.MemoryExpense{ID: i.CaseID + "/" + i.Arm + "/" + stage + "/not-used", CaseID: i.CaseID, Arm: i.Arm, Stage: stage, Status: "not_used", Outcome: "not_used", EvidenceSHA256: acceptance.MemoryDigest([]string{c.plan.MemoryConfigSHA256, stage, "disabled_or_not_invoked"}), InputTokens: &zero, OutputTokens: &zero, TotalTokens: &zero, Milliseconds: &zero, Microunits: &zero, StorageBytes: &zero}
		if err := c.recordExpense(ctx, e); err != nil {
			return err
		}
	}
	return nil
}
