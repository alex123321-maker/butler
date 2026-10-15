package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestMemorySemanticProjectionAndRepeatIsolation(t *testing.T) {
	s := acceptance.MemorySemanticSuite{Cases: []acceptance.MemorySemanticCase{{ID: "fresh", Question: "question", RequiredFacts: []string{"VERIFIER_ONLY_FACT"}, ForbiddenClaims: []string{"VERIFIER_ONLY_FORBIDDEN"}, OracleSourceIDs: []string{"s1"}, RepeatRetrieval: true, History: []acceptance.MemorySource{{ID: "s1", Body: "ordinary source", Status: "active"}}}}}
	id := acceptance.MemoryOpaqueCaseID(s.Cases[0].MemoryCase())
	inputs, err := semanticFreshInputs(s, []acceptance.MemoryCapture{{CaseID: id, SourceIDs: []string{"s1"}, Prompt: "only actual excerpt", EvidenceSHA256: strings.Repeat("e", 64)}})
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) != 4 || len(inputs[0].Memory) != 0 || inputs[1].Memory[0].Body != "ordinary source" || inputs[2].Memory[0].Body != "" || inputs[3].Memory[0].Body != "" {
		t.Fatal("arm isolation", inputs)
	}
	a, _ := acceptance.MemoryModelProjection(inputs[2])
	b, _ := acceptance.MemoryModelProjection(inputs[3])
	if string(a) != string(b) || strings.Contains(string(a), "VERIFIER_ONLY") || strings.Contains(string(a), "ordinary source") {
		t.Fatal("rubric or unselected original leaked")
	}
	var schema map[string]any
	if json.Unmarshal(acceptance.MemorySemanticSchema(), &schema) != nil {
		t.Fatal("invalid schema")
	}
	if strings.Contains(string(acceptance.MemorySemanticSchema()), "abstain") || strings.Contains(string(acceptance.MemorySemanticSchema()), "confidence") {
		t.Fatal("legacy response formatting reintroduced")
	}
}

func TestMemorySemanticFreeSchemaWithoutAuthentication(t *testing.T) {
	calls := 0
	_, err := antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, MaximumOutputTokens: 32768, GoogleResponseSchema: acceptance.MemorySemanticSchema(), AuthSource: shadowV5NoCredentials{&calls}})
	if err != nil || calls != 0 {
		t.Fatal("schema rejected or authenticated", err)
	}
}

func TestMemorySemanticCollectorFreeAnswerReopenedReadIntegration(t *testing.T) {
	c := memoryCollectorFixture(t)
	ctx := context.Background()
	mc := c.suite.Cases[0]
	if err := c.capture(ctx, mc); err != nil {
		t.Fatal(err)
	}
	id := acceptance.MemoryOpaqueCaseID(mc)
	pool, err := pgxpool.NewWithConfig(ctx, c.pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	// A fresh manifest reader and pool must validate the durable source graph.
	anchor := filepath.Join(t.TempDir(), "anchor")
	manifest := catalog.NewFileManifest(anchor)
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	c.pool = pool
	c.catalog = catalog.New(pool, manifest)
	c.private = t.TempDir()
	c.freeAnswer = true
	c.readSuffix = "-reopened-test"
	c.captures = nil
	refs := []catalog.SourceRef{}
	logical := map[string]acceptance.MemorySource{}
	for _, h := range mc.History {
		if ref, ok := c.sourceRefs[id][h.ID]; ok {
			refs = append(refs, ref)
			logical[ref.ID] = h
		}
	}
	live, run, err := c.ownedRun(ctx, mc, "reopened-read")
	if err != nil {
		t.Fatal(err)
	}
	if err = c.captureRead(ctx, mc, refs, logical, live, run); err != nil {
		t.Fatal(err)
	}
	if len(c.captures) != 1 || len(c.captures[0].SourceIDs) == 0 {
		t.Fatal("durable retrieval lost sources")
	}
	i := acceptance.MemoryInput{CaseID: id, Arm: "oracle", Instructions: acceptance.MemorySemanticInstructions, Question: mc.Question, Task: mc.Task, Memory: mc.History}
	raw := `{"answer":"Обычный ответ; независимая смысловая оценка ещё не выполнена."}`
	c.factory = func(json.RawMessage) (transport.ModelProvider, error) { return &memoryFixtureProvider{raw: raw}, nil }
	o, err := c.sample(ctx, mc, i)
	if err != nil || o.Status != "completed" {
		t.Fatal(o, err)
	}
	if string(acceptance.MemoryObservationOriginal(o)) != raw {
		t.Fatal("free answer rewritten")
	}
	var scope string
	if err = pool.QueryRow(ctx, `SELECT task_scope FROM fixture_memory_eval_results WHERE case_id=$1 AND arm='oracle'`, id).Scan(&scope); err != nil || scope != "free_answer_semantics_pending_independent_review" {
		t.Fatal(scope, err)
	}
	if _, err = os.Stat(filepath.Join(c.private, "final-"+strings.TrimPrefix(id, "memory:")+"-oracle.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestMemorySemanticRuntimeProofDeniesDrift(t *testing.T) {
	pin := memorySemanticPins{Image: "sha256:test", TrialID: "memory-semantic-trial-fixture", SourceDir: "/srv/storage/butler/agents/source/snapshots/test", TrialRoot: "/srv/storage/butler/agents/memory-semantic-trial-fixture"}
	p := memorySemanticExecution{Image: pin.Image, Path: "/runtime/collector.test", User: "10001:10001", Args: []string{"-test.run", "^TestMemorySemanticActualTrial$", "-test.v", "-test.timeout", "245m"}, Memory: 1536 << 20, NanoCpus: 2000000000, PidsLimit: 128, ReadonlyRootfs: true, ProxyConfigured: true, LocalBypass: true, Networks: []string{pin.TrialID, "butler-lab_local-inference", "butler-lab_backend"}}
	p.CapDrop = []string{"ALL"}
	p.SecurityOpt = []string{"no-new-privileges:true"}
	p.Tmpfs = map[string]string{"/tmp": "rw,noexec,nosuid,size=16777216"}
	sources := map[string]string{"/src": pin.SourceDir, "/runtime": pin.TrialRoot + "/build", "/metadata": pin.TrialRoot + "/metadata", "/private": pin.TrialRoot + "/private", "/auth/local-token": "/srv/storage/butler/agents/local-models/private/token", "/auth/native-import.json": "/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli/butler-native-import.json"}
	for dest, rw := range map[string]bool{"/src": false, "/runtime": false, "/metadata": false, "/private": true, "/auth/local-token": false, "/auth/native-import.json": false} {
		p.Mounts = append(p.Mounts, memorySemanticMount{Destination: dest, RW: rw, Type: "bind", Source: sources[dest]})
	}
	if err := validateMemorySemanticExecution(p, pin, "read"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"user", "image", "writable_source", "proxy", "extra_network", "args", "caps", "privileged", "source_leaf", "security_opt"} {
		t.Run(kind, func(t *testing.T) {
			b, _ := json.Marshal(p)
			var changed memorySemanticExecution
			_ = json.Unmarshal(b, &changed)
			switch kind {
			case "user":
				changed.User = "0"
			case "image":
				changed.Image = "other"
			case "writable_source":
				for n := range changed.Mounts {
					if changed.Mounts[n].Destination == "/src" {
						changed.Mounts[n].RW = true
					}
				}
			case "proxy":
				changed.ProxyConfigured = false
			case "extra_network":
				changed.Networks = append(changed.Networks, "external")
			case "args":
				changed.Args = nil
			case "caps":
				changed.CapAdd = []string{"SYS_ADMIN"}
			case "privileged":
				changed.Privileged = true
			case "source_leaf":
				changed.Mounts[0].Source = "/other"
			case "security_opt":
				changed.SecurityOpt = nil
			}
			if validateMemorySemanticExecution(changed, pin, "read") == nil {
				t.Fatal("runtime drift admitted")
			}
		})
	}
}

func TestMemorySemanticEarlyFailureRetainsAndSealsIntegration(t *testing.T) {
	for _, phase := range []string{"write", "read"} {
		t.Run(phase, func(t *testing.T) {
			c := memoryCollectorFixture(t)
			mc := c.suite.Cases[0]
			var err error
			if phase == "write" {
				c.backend = &memoryFixtureBackend{fail: true}
				err = c.capture(context.Background(), mc)
			} else {
				if err = c.capture(context.Background(), mc); err != nil {
					t.Fatal(err)
				}
				id := acceptance.MemoryOpaqueCaseID(mc)
				c.private = t.TempDir()
				c.freeAnswer = true
				c.factory = func(json.RawMessage) (transport.ModelProvider, error) {
					return nil, errors.New("fixture denies native construction after owned admission")
				}
				_, err = c.sample(context.Background(), mc, acceptance.MemoryInput{CaseID: id, Arm: "oracle", Task: mc.Task, Question: mc.Question, Instructions: acceptance.MemorySemanticInstructions, Memory: mc.History})
			}
			if err == nil {
				t.Fatal("failed embedding not exercised")
			}
			if err = c.finishSemantic(false); err != nil {
				t.Fatal(err)
			}
			var closed bool
			var expenses, aux int
			if err = c.pool.QueryRow(context.Background(), `SELECT closed,(SELECT count(*) FROM fixture_memory_eval_expenses),(SELECT count(*) FROM fixture_memory_eval_aux) FROM fixture_memory_eval_window`).Scan(&closed, &expenses, &aux); err != nil || !closed || expenses == 0 || aux == 0 {
				t.Fatal("failed spend not retained and closed", closed, expenses, aux, err)
			}
			if _, err = os.Stat(filepath.Join(c.private, "ledger.json")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type memorySemanticUnavailableProvider struct{ memoryFixtureProvider }

func (p *memorySemanticUnavailableProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	return nativeScriptedStream(transport.NewRunStartedEvent(r.Context.RunID, antigravity.Name, transport.CapabilitySnapshot{}, nil), transport.NewTransportErrorEvent(r.Context.RunID, antigravity.Name, &transport.Error{Type: transport.ErrorTypeProviderTimeout})), nil
}

func TestMemorySemanticFixedScheduleRetainsFailedEntryIntegration(t *testing.T) {
	c := memoryCollectorFixture(t)
	ctx := context.Background()
	b, err := os.ReadFile(filepath.Join(shadowRepoRoot(), "testdata/acceptance/ru-memory-semantic-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var suite acceptance.MemorySemanticSuite
	if err = json.Unmarshal(b, &suite); err != nil || suite.Validate() != nil {
		t.Fatal("fresh suite invalid", err)
	}
	for _, cse := range suite.Cases {
		if err = c.capture(ctx, cse.MemoryCase()); err != nil {
			t.Fatal(cse.ID, err)
		}
	}
	inputs, err := semanticFreshInputs(suite, c.captures)
	if err != nil || len(inputs) != 38 {
		t.Fatal(len(inputs), err)
	}
	c.freeAnswer = true
	calls := 0
	c.factory = func(json.RawMessage) (transport.ModelProvider, error) {
		calls++
		if calls == 2 {
			return &memorySemanticUnavailableProvider{}, nil
		}
		return &memoryFixtureProvider{raw: `{"answer":"Контролируемый тестовый текст, не модельная оценка качества."}`}, nil
	}
	if err = collectMemorySemanticSamples(ctx, c, suite, inputs, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 38 || len(c.measurements.Observations) != 38 || c.measurements.Observations[1].Status != "failed" || c.measurements.Observations[2].Status != "completed" {
		t.Fatal("failed entry retried, omitted or blocked distinct schedule", calls)
	}
	if err = c.finishSemantic(false); err != nil {
		t.Fatal(err)
	}
	var attempted, failed int
	if err = c.pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE outcome<>'completed') FROM fixture_memory_eval_attempts`).Scan(&attempted, &failed); err != nil || attempted != 38 || failed != 1 {
		t.Fatal(attempted, failed, err)
	}
}

func TestMemorySemanticSQLFailureStillRetainsFileIntegration(t *testing.T) {
	c := memoryCollectorFixture(t)
	if err := c.capture(context.Background(), c.suite.Cases[0]); err != nil {
		t.Fatal(err)
	}
	c.measurements.Observations = []acceptance.MemoryObservation{}
	c.pool.Close()
	if err := c.finishSemantic(false); err == nil {
		t.Fatal("SQL loss hidden")
	}
	b, err := os.ReadFile(filepath.Join(c.private, "measurements.json"))
	if err != nil {
		t.Fatal("available file evidence lost with SQL", err)
	}
	var m acceptance.MemoryMeasurements
	if err = json.Unmarshal(b, &m); err != nil || len(m.Expenses) == 0 {
		t.Fatal("paid prefix missing", err)
	}
}
