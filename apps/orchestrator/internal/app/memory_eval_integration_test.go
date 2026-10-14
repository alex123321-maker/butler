package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

type memoryFixtureBackend struct{ fail bool }

func (*memoryFixtureBackend) Describe(context.Context, *pb.Empty) (*pb.Catalog, error) {
	s := memoryEvalSpace()
	return &pb.Catalog{Models: []*pb.ModelProfile{{Id: s.ProfileID, Model: s.Model, Revision: s.Revision, PreprocessingVersion: s.Preprocessing, RuntimeVersion: s.RuntimeVersion, Dimensions: 768, Format: "safetensors", Precision: "BF16", Enabled: true, SourceUrl: "https://huggingface.co/" + s.Model, Capabilities: []pb.Capability{pb.Capability_CAPABILITY_EMBED}}}}, nil
}
func (b *memoryFixtureBackend) Infer(_ context.Context, r *pb.InferRequest) (*pb.InferResponse, error) {
	if b.fail {
		return nil, errors.New("fixture failed call")
	}
	s := memoryEvalSpace()
	out := &pb.EmbedOutput{IndexSpace: s.WireID()}
	for _, t := range r.GetEmbed().Texts {
		v := make([]float32, 768)
		v[0] = 1
		out.Vectors = append(out.Vectors, &pb.Vector{Id: t.Id, Values: v})
	}
	return &pb.InferResponse{RequestId: r.RequestId, ProfileId: r.ProfileId, Revision: s.Revision, Output: &pb.InferResponse_Embed{Embed: out}}, nil
}

type memoryFixtureProvider struct {
	nativeScriptedObservationProvider
	raw   string
	fail  bool
	after func()
}

func (p *memoryFixtureProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if p.fail {
		return nil, errors.New("fixture failed native entry")
	}
	if len(r.ToolDefinitions) != 0 {
		return nil, errors.New("unexpected external capability")
	}
	if p.after != nil {
		p.after()
	}
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	e := transport.NewAssistantFinalEvent(r.Context.RunID, antigravity.Name, transport.AssistantFinal{Content: p.raw, FinishReason: "STOP", Usage: &transport.Usage{InputTokens: 120, OutputTokens: 30, TotalTokens: 170}})
	version := "gemini-3.8-flash-n"
	e.ProviderModelVersion = &version
	return nativeScriptedStream(e, transport.NewTerminalEvent(r.Context.RunID, transport.EventTypeRunCompleted, antigravity.Name)), nil
}

func TestMemoryEvalCollectorFullCorrelationIntegration(t *testing.T) {
	c := memoryCollectorFixture(t)
	ctx := context.Background()
	for _, cse := range c.suite.Cases {
		if err := c.capture(ctx, cse); err != nil {
			t.Fatal("complete fixture capture", cse.ID, err)
		}
	}
	f, err := acceptance.FreezeMemory(c.suite, c.plan, c.captures)
	if err != nil {
		t.Fatal(err)
	}
	c.frozen = f
	if err = c.writeEvidence("frozen-before-native.json", f); err != nil {
		t.Fatal(err)
	}
	for _, i := range f.Inputs {
		var cse acceptance.MemoryCase
		for _, candidate := range c.suite.Cases {
			if acceptance.MemoryOpaqueCaseID(candidate) == i.CaseID {
				cse = candidate
			}
		}
		answer := acceptance.MemoryAnswer{Abstain: true, Citations: []acceptance.CommandWitness{}, TaskRevision: i.Task.Revision, NextAction: cse.Expected.NextAction}
		raw, _ := json.Marshal(answer)
		// Valid JSON string and malformed UTF-8 text remain separate originals.
		if cse.ID == "v2-d01" && i.Arm == "no_ltm" {
			raw = []byte(`"foo"`)
		}
		c.factory = func(json.RawMessage) (transport.ModelProvider, error) {
			return &memoryFixtureProvider{raw: string(raw)}, nil
		}
		o, err := c.sample(ctx, cse, i)
		if err != nil {
			t.Fatal("fixture sample", cse.ID, i.Arm, err)
		}
		c.measurements.Observations = append(c.measurements.Observations, o)
		if err = c.fillUnused(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	c.measurements.FrozenSHA256 = f.SHA256
	if err = c.writeEvidence("measurements.json", c.measurements); err != nil {
		t.Fatal(err)
	}
	if _, err = c.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET closed=TRUE`); err != nil {
		t.Fatal(err)
	}
	if err = c.exportLedger(ctx); err != nil {
		t.Fatal(err)
	}
	audit, err := acceptance.AuditMemoryRunner(c.suite, f, c.measurements, c.private, strings.Repeat("4", 64))
	if err != nil {
		t.Fatal("controlled complete correlation", err)
	}
	if audit.NativeCalls != 64 || audit.QuestionResults != 64 || audit.UnknownBroaderTaskResults != 64 {
		t.Fatal("comparison count changed", audit)
	}
	// The auditor must fail against the original when the import hides a stage.
	bad := c.measurements
	bad.Expenses = bad.Expenses[1:]
	if _, err = acceptance.AuditMemoryRunner(c.suite, f, bad, c.private, strings.Repeat("4", 64)); err == nil {
		t.Fatal("paid original hidden by import")
	}
	for _, kind := range []string{"guard_operation", "guard_sources", "capture_digest", "capture_sources"} {
		t.Run(kind, func(t *testing.T) {
			// Corrupt a separate copy; keep the successfully correlated originals.
			dir := t.TempDir()
			entries, err := os.ReadDir(c.private)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				raw, err := os.ReadFile(filepath.Join(c.private, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(dir, entry.Name()), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			write := func(name string, value any) {
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(dir, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			forged := f
			want := "guard operation/current source refs differ"
			if strings.HasPrefix(kind, "guard_") {
				raw, err := os.ReadFile(filepath.Join(dir, "ledger.json"))
				if err != nil {
					t.Fatal(err)
				}
				var ledger map[string]json.RawMessage
				if err = json.Unmarshal(raw, &ledger); err != nil {
					t.Fatal(err)
				}
				var guards []map[string]any
				if err = json.Unmarshal(ledger["fixture_memory_eval_guards"], &guards); err != nil || len(guards) == 0 {
					t.Fatal("guard originals missing", err)
				}
				if kind == "guard_operation" {
					guards[0]["operation_id"] = "different-operation"
				} else {
					guards[0]["sources"] = []map[string]any{{"source_id": "different-source", "revision": 1, "access_revision": 1, "content_hash": strings.Repeat("a", 64)}}
				}
				ledger["fixture_memory_eval_guards"], _ = json.Marshal(guards)
				write("ledger.json", ledger)
			} else {
				forged.Captures = append([]acceptance.MemoryCapture(nil), f.Captures...)
				if kind == "capture_digest" {
					forged.Captures[0].EvidenceSHA256 = strings.Repeat("a", 64)
				} else {
					forged.Captures[0].SourceIDs = []string{"different-source"}
				}
				write("frozen-before-native.json", forged)
				want = "actual bundle prompt/digest/source selection differs"
			}
			if _, err := acceptance.AuditMemoryRunner(c.suite, forged, c.measurements, dir, strings.Repeat("4", 64)); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatal("tampered provenance must fail at its binding", err)
			}
		})
	}
}

func TestMemoryEvalCollectorCurrentAuthorityRacesIntegration(t *testing.T) {
	for _, kind := range []string{"task_amendment", "lease_expiry", "source_deletion", "retention_failure"} {
		t.Run(kind, func(t *testing.T) {
			c := memoryCollectorFixture(t)
			ctx := context.Background()
			cse := c.suite.Cases[0]
			if err := c.capture(ctx, cse); err != nil {
				t.Fatal(err)
			}
			f, err := acceptance.FreezeMemory(c.suite, c.plan, c.captures)
			if err != nil {
				t.Fatal(err)
			}
			var input acceptance.MemoryInput
			for _, i := range f.Inputs {
				if i.CaseID == acceptance.MemoryOpaqueCaseID(cse) && i.Arm == "retrieval" {
					input = i
				}
			}
			after := func() {
				var err error
				switch kind {
				case "task_amendment":
					_, err = c.pool.Exec(ctx, `UPDATE tasks SET revision=revision+1 WHERE task_id=$1`, input.CaseID+":task")
				case "lease_expiry":
					_, err = c.pool.Exec(ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE run_id IN(SELECT run_id FROM fixture_memory_eval_attempts)`)
				case "source_deletion":
					_, actor := c.owner(cse)
					ref := c.sourceRefs[input.CaseID][cse.History[0].ID]
					_, err = c.catalog.Delete(actor, ref.ID, "delete", ref.Revision)
				case "retention_failure":
					c.retainedBytes = 128 << 20
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			c.factory = func(json.RawMessage) (transport.ModelProvider, error) {
				return &memoryFixtureProvider{raw: `"foo"`, after: after}, nil
			}
			o, err := c.sample(ctx, cse, input)
			if err == nil {
				t.Fatal("late authority/retention failure accepted", o.Status)
			}
			var count int
			if c.pool.QueryRow(ctx, `SELECT count(*) FROM fixture_memory_eval_expenses WHERE stage='generation' AND record->>'calls'='1'`).Scan(&count) != nil || count != 1 {
				t.Fatal("charged failure disappeared")
			}
		})
	}
}
func memoryCollectorFixture(t *testing.T) *memoryEvalCollector {
	return memoryCollectorFixtureWithPrior(t, nil, nil, "")
}

func memoryCollectorFixtureWithPrior(t *testing.T, prior *acceptance.MemoryPrior, pins *acceptance.MemoryPriorPins, directory string) *memoryEvalCollector {
	t.Helper()
	pool := memorytest.Database(t)
	f, err := os.Open(filepath.Join(shadowRepoRoot(), "testdata/acceptance/ru-memory-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	suite, err := acceptance.LoadMemorySuite(f)
	if err != nil {
		t.Fatal(err)
	}
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	p := memoryEvalPolicy()
	budgets := budget.NewController(pool, p)
	budgets.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	c := &memoryEvalCollector{pool: pool, catalog: catalog.New(pool, manifest), budgets: budgets, backend: &memoryFixtureBackend{}, suite: suite, private: t.TempDir(), sourceRefs: map[string]map[string]catalog.SourceRef{}, started: time.Now().UTC()}
	c.plan = acceptance.MemoryPlan{Version: suite.Version, WindowID: "fixture-current-components", EvidenceKind: "model_observation", CodeCommit: strings.Repeat("1", 40), SourceSHA256: strings.Repeat("2", 64), HardwareSHA256: strings.Repeat("3", 64), Model: antigravity.FlashHighModel, ModelVersion: "gemini-3.8-flash-n", Runtime: "fixture-only-no-native", MemoryConfigSHA256: memoryEvalConfigDigest(), PricingBasis: "fixture_no_billing", SuiteSHA256: acceptance.MemorySuiteDigest(suite), CriteriaSHA256: acceptance.MemoryVersionCriteriaDigest(suite), Split: "all"}
	if prior != nil {
		c.prior, c.priorPins, c.priorDirectory = prior, pins, directory
		c.plan.WindowID = "fixture-pending-components"
		c.started = prior.Expires.Add(-3 * time.Hour)
	}
	if err = c.init(context.Background()); err != nil {
		t.Fatal(err)
	}
	return c
}

func TestMemoryEvalCollectorNativeSchemasValidateWithoutAuthentication(t *testing.T) {
	f, err := os.Open(filepath.Join(shadowRepoRoot(), "testdata/acceptance/ru-memory-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	suite, err := acceptance.LoadMemorySuite(f)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range suite.Cases {
		if c.Blocked() {
			continue
		}
		for _, arm := range acceptance.MemoryEvalArms() {
			i := acceptance.MemoryInput{Arm: arm, Memory: []acceptance.MemorySource{}}
			if arm != "no_ltm" {
				for _, h := range c.History {
					if h.Status != "deleted" {
						i.Memory = append(i.Memory, h)
					}
				}
			}
			calls := 0
			_, err := antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, MaximumOutputTokens: 32768, GoogleResponseSchema: acceptance.MemoryNativeSchema(i), AuthSource: shadowV5NoCredentials{&calls}})
			if err != nil || calls != 0 {
				t.Fatal("native schema rejected or authenticated", c.ID, arm, err)
			}
		}
	}
}
func TestMemoryEvalCollectorCurrentComponentsIntegration(t *testing.T) {
	c := memoryCollectorFixture(t)
	ctx := context.Background()
	// Includes oversized originals; current structured parser must keep all bytes
	// in bounded fragments, without a parallel retrieval implementation.
	for _, index := range []int{0, 6, 7} {
		if err := c.capture(ctx, c.suite.Cases[index]); err != nil {
			t.Fatal("current capture", index, err)
		}
	}
	frozen, err := acceptance.FreezeMemory(c.suite, c.plan, c.captures)
	if err != nil {
		t.Fatal(err)
	}
	c.frozen = frozen
	var input acceptance.MemoryInput
	for _, i := range frozen.Inputs {
		if i.CaseID == acceptance.MemoryOpaqueCaseID(c.suite.Cases[0]) && i.Arm == "retrieval" {
			input = i
		}
	}
	if input.SkipReason != "" || len(input.Memory) == 0 {
		t.Fatal("current bundle absent", input.SkipReason)
	}
	malformed := "not JSON\n{partial"
	c.factory = func(json.RawMessage) (transport.ModelProvider, error) {
		return &memoryFixtureProvider{raw: malformed}, nil
	}
	observation, err := c.sample(ctx, c.suite.Cases[0], input)
	if err != nil {
		t.Fatal("malformed semantic result is retained", err)
	}
	if observation.Status != "completed" || observation.Outcome.Success != nil || string(acceptance.MemoryObservationOriginal(observation)) != malformed {
		t.Fatal("invented task success or repaired original")
	}
	if err = c.fillUnused(ctx, input); err != nil {
		t.Fatal(err)
	}
	if err = c.exportLedger(ctx); err != nil {
		t.Fatal(err)
	}
	var actualTotal int64
	if err = c.pool.QueryRow(ctx, `SELECT total_tokens FROM fixture_memory_eval_attempts`).Scan(&actualTotal); err != nil || actualTotal != 170 {
		t.Fatal("reasoning tokens discarded", actualTotal, err)
	}
	if _, err = c.pool.Exec(ctx, `UPDATE fixture_memory_eval_expenses SET stage='retry'`); err == nil {
		t.Fatal("expense mutation accepted")
	}
	if _, err = c.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET closed=TRUE`); err != nil {
		t.Fatal(err)
	}
	if c.reserveCall(ctx, 1) == nil {
		t.Fatal("sealed window reopened")
	}
}
func TestMemoryEvalCollectorFailuresAndSourceFenceIntegration(t *testing.T) {
	c := memoryCollectorFixture(t)
	ctx := context.Background()
	cse := c.suite.Cases[0]
	if err := c.capture(ctx, cse); err != nil {
		t.Fatal(err)
	}
	frozen, err := acceptance.FreezeMemory(c.suite, c.plan, c.captures)
	if err != nil {
		t.Fatal(err)
	}
	var input acceptance.MemoryInput
	for _, i := range frozen.Inputs {
		if i.CaseID == acceptance.MemoryOpaqueCaseID(cse) && i.Arm == "no_ltm" {
			input = i
		}
	}
	c.factory = func(json.RawMessage) (transport.ModelProvider, error) { return &memoryFixtureProvider{fail: true}, nil }
	o, err := c.sample(ctx, cse, input)
	if err == nil || o.Status != "failed" {
		t.Fatal("transport failure continued")
	}
	var count int
	if c.pool.QueryRow(ctx, `SELECT count(*) FROM fixture_memory_eval_expenses WHERE stage='generation' AND record->>'outcome'='failed' AND record->'input_tokens'='null'::jsonb`).Scan(&count) != nil || count != 1 {
		t.Fatal("failed unknown usage dropped")
	}
	if c.pool.QueryRow(ctx, `SELECT count(*) FROM fixture_memory_eval_expenses WHERE id LIKE '%/original-write' AND record->>'outcome'='success'`).Scan(&count) != nil || count != 1 {
		t.Fatal("successful original retention confused with failed inference")
	}
	c.backend = &memoryFixtureBackend{fail: true}
	if c.capture(ctx, c.suite.Cases[1]) == nil {
		t.Fatal("local failure lost")
	}
	if c.pool.QueryRow(ctx, `SELECT count(*) FROM fixture_memory_eval_expenses WHERE record->>'outcome'='failed' AND stage IN ('generation','embedding')`).Scan(&count) != nil || count != 2 {
		t.Fatal("failed paid prefix dropped")
	}
	live, _, err := c.ownedRun(ctx, cse, "source-fence-probe")
	if err != nil {
		t.Fatal(err)
	}
	full := acceptance.MemoryInput{CaseID: input.CaseID, Memory: cse.History}
	if err = c.checkSources(live, full); err != nil {
		t.Fatal(err)
	}
	_, actor := c.owner(cse)
	ref := c.sourceRefs[input.CaseID][cse.History[0].ID]
	if _, err = c.catalog.Delete(actor, ref.ID, "delete", ref.Revision); err != nil {
		t.Fatal(err)
	}
	if c.checkSources(live, full) == nil {
		t.Fatal("deleted source accepted")
	}
}
