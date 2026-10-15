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
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/semantic"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

type memorySemanticState struct {
	Started      time.Time
	Deadline     time.Time
	Bindings     map[string]map[string]catalog.SourceRef
	Measurements acceptance.MemoryMeasurements
}

type memorySemanticPins struct{ Release, CorpusSHA256, RubricSHA256, SourceSHA256, BinarySHA256, Image, TrialID, SourceDir, TrialRoot string }
type memorySemanticMount struct {
	Destination, Type, Source string
	RW                        bool
}
type memorySemanticExecution struct {
	Image, Path, User                            string
	Args, Networks                               []string
	Memory, NanoCpus, PidsLimit                  int64
	ReadonlyRootfs, ProxyConfigured, LocalBypass bool
	Privileged                                   bool
	CapDrop, CapAdd, SecurityOpt                 []string
	Tmpfs                                        map[string]string
	Mounts                                       []memorySemanticMount
}

func validateMemorySemanticExecution(p memorySemanticExecution, pin memorySemanticPins, phase string) error {
	if pin.Image == "" || !strings.HasPrefix(pin.TrialID, "memory-semantic-trial-") || p.Image != pin.Image || p.Path != "/runtime/collector.test" || p.User != "10001:10001" || p.Memory != 1536<<20 || p.NanoCpus != 2000000000 || p.PidsLimit != 128 || !p.ReadonlyRootfs || strings.Join(p.Args, " ") != "-test.run ^TestMemorySemanticActualTrial$ -test.v -test.timeout 245m" {
		return errors.New("semantic runtime identity/resources differ")
	}
	if p.Privileged || len(p.CapAdd) != 0 || len(p.CapDrop) != 1 || p.CapDrop[0] != "ALL" || len(p.SecurityOpt) != 1 || p.SecurityOpt[0] != "no-new-privileges:true" || len(p.Tmpfs) != 1 || p.Tmpfs["/tmp"] != "rw,noexec,nosuid,size=16777216" {
		return errors.New("semantic sandbox controls differ")
	}
	if !strings.HasPrefix(pin.SourceDir, "/srv/storage/butler/agents/") || pin.TrialRoot != "/srv/storage/butler/agents/"+pin.TrialID {
		return errors.New("semantic pinned roots differ")
	}
	allowed := map[string]bool{"/src": false, "/runtime": false, "/metadata": false, "/private": true, "/auth/local-token": false}
	sources := map[string]string{"/src": pin.SourceDir, "/runtime": pin.TrialRoot + "/build", "/metadata": pin.TrialRoot + "/metadata", "/private": pin.TrialRoot + "/private", "/auth/local-token": "/srv/storage/butler/agents/local-models/private/token"}
	networks := []string{pin.TrialID, "butler-lab_local-inference"}
	if phase == "read" {
		allowed["/auth/native-import.json"] = false
		sources["/auth/native-import.json"] = "/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli/butler-native-import.json"
		networks = append(networks, "butler-lab_backend")
		if !p.ProxyConfigured || !p.LocalBypass {
			return errors.New("native proxy/bypass differ")
		}
	}
	if phase != "write" && phase != "read" {
		return errors.New("semantic phase differs")
	}
	if len(p.Mounts) != len(allowed) || len(p.Networks) != len(networks) {
		return errors.New("semantic mount/network set differs")
	}
	for _, m := range p.Mounts {
		rw, ok := allowed[m.Destination]
		if !ok || rw != m.RW || m.Type != "bind" || m.Source != sources[m.Destination] {
			return errors.New("semantic mount access differs")
		}
		delete(allowed, m.Destination)
	}
	for _, network := range networks {
		if !memoryStringMember(p.Networks, network) {
			return errors.New("semantic network differs")
		}
	}
	return nil
}

// Installed before embedding/native spend. Only a successful writer handoff
// preserves the original fixed expiry; failure and final read seal it.
func (c *memoryEvalCollector) finishSemantic(handoff bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	closeWindow := func() error {
		_, e := c.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET closed=TRUE WHERE singleton AND NOT closed`)
		return e
	}
	var errs []error
	if !handoff {
		errs = append(errs, closeWindow())
	}
	errs = append(errs, c.writeEvidence("measurements.json", c.measurements))
	errs = append(errs, c.exportLedger(ctx))
	result := errors.Join(errs...)
	if result != nil {
		result = errors.Join(result, closeWindow())
	}
	return result
}

func semanticFreshInputs(s acceptance.MemorySemanticSuite, captures []acceptance.MemoryCapture) ([]acceptance.MemoryInput, error) {
	byID := map[string]acceptance.MemoryCapture{}
	for _, cap := range captures {
		byID[cap.CaseID] = cap
	}
	var inputs []acceptance.MemoryInput
	for _, c := range s.Cases {
		id := acceptance.MemoryOpaqueCaseID(c.MemoryCase())
		cap, ok := byID[id]
		if !ok {
			return nil, errors.New("current durable retrieval capture missing")
		}
		arms := []string{"no_ltm", "oracle", "retrieval"}
		if c.RepeatRetrieval {
			arms = append(arms, "retrieval_repeat")
		}
		for _, arm := range arms {
			i := acceptance.MemoryInput{CaseID: id, Arm: arm, Instructions: acceptance.MemorySemanticInstructions, Question: c.Question, AsOfUTC: c.AsOfUTC, Task: c.Task, Memory: []acceptance.MemorySource{}}
			for _, h := range c.History {
				if h.Status == "deleted" {
					continue
				}
				if arm == "oracle" && memoryStringMember(c.OracleSourceIDs, h.ID) {
					i.Memory = append(i.Memory, h)
				}
				if strings.HasPrefix(arm, "retrieval") && memoryStringMember(cap.SourceIDs, h.ID) {
					h.Body = ""
					i.Memory = append(i.Memory, h)
				}
			}
			if strings.HasPrefix(arm, "retrieval") {
				i.MemoryPrompt = cap.Prompt
				i.CaptureSHA256 = cap.EvidenceSHA256
			}
			inputs = append(inputs, i)
		}
	}
	return inputs, nil
}

func memoryStringMember(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

func collectMemorySemanticSamples(ctx context.Context, c *memoryEvalCollector, suite acceptance.MemorySemanticSuite, inputs []acceptance.MemoryInput, observe func(acceptance.MemoryObservation)) error {
	for _, i := range inputs {
		var mc acceptance.MemoryCase
		for _, cse := range suite.Cases {
			if acceptance.MemoryOpaqueCaseID(cse.MemoryCase()) == i.CaseID {
				mc = cse.MemoryCase()
				break
			}
		}
		o, e := c.sample(ctx, mc, i)
		c.measurements.Observations = append(c.measurements.Observations, o)
		if observe != nil {
			observe(o)
		}
		if e != nil && !errors.Is(e, errMemoryAvailability) {
			return e
		}
		if e = c.fillUnused(ctx, i); e != nil {
			return e
		}
	}
	return nil
}

// Two explicit processes: the first owns ordinary Catalog/index writes; the
// second reconnects to the same isolated durable database and performs new reads.
// No authentication or inference is possible in the ordinary test path.
func TestMemorySemanticActualTrial(t *testing.T) {
	phase := os.Getenv("BUTLER_MEMORY_SEMANTIC_PHASE")
	if phase != "write" && phase != "read" {
		t.Skip("explicit fresh semantic trial required")
	}
	private := os.Getenv("BUTLER_MEMORY_EVAL_PRIVATE")
	release := os.Getenv("BUTLER_MEMORY_EVAL_RELEASE")
	if !filepath.IsAbs(private) || len(release) != 40 {
		t.Fatal("private isolated root and exact commit required")
	}
	corpus, err := os.ReadFile(filepath.Join(shadowRepoRoot(), "testdata/acceptance/ru-memory-semantic-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var suite acceptance.MemorySemanticSuite
	if json.Unmarshal(corpus, &suite) != nil || suite.Validate() != nil {
		t.Fatal("reviewed semantic corpus required")
	}
	method, err := os.ReadFile(filepath.Join(shadowRepoRoot(), "docs/ai/memory-eval-semantic-methodology.md"))
	if err != nil {
		t.Fatal(err)
	}
	var pin memorySemanticPins
	b, err := os.ReadFile("/metadata/semantic-pins.json")
	if err != nil || json.Unmarshal(b, &pin) != nil {
		t.Fatal("operator pins required before credentials")
	}
	tree, err := acceptance.TreeHash(shadowRepoRoot(), nil)
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	binary, err := acceptance.FileHash(exe)
	if err != nil || pin.Release != release || pin.CorpusSHA256 != hashMemoryBytes(corpus) || pin.RubricSHA256 != hashMemoryBytes(method) || pin.SourceSHA256 != tree || pin.BinarySHA256 != binary {
		t.Fatal("source/binary/corpus/method pins differ")
	}
	var actual memorySemanticExecution
	b, err = os.ReadFile("/metadata/" + phase + "-execution.json")
	if err != nil || json.Unmarshal(b, &actual) != nil || validateMemorySemanticExecution(actual, pin, phase) != nil {
		t.Fatal("actual runtime denied before private state")
	}
	var pool *pgxpool.Pool
	dbConfig, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil || !strings.HasSuffix(dbConfig.ConnConfig.Database, "_test") {
		t.Fatal("isolated semantic test database required")
	}
	if phase == "write" {
		pool = memoryEvalDatabase(t)
	} else {
		pool, err = pgxpool.New(context.Background(), os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
	}
	manifest := catalog.NewFileManifest(filepath.Join(private, "deletion-anchor"))
	if phase == "write" {
		if err = manifest.Initialize(); err != nil {
			t.Fatal(err)
		}
	}
	cat := catalog.New(pool, manifest)
	budgets := budget.NewController(pool, memoryEvalPolicy())
	budgets.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	target, err := budget.NewLocalTarget("local-models:9090", "operator-pinned-service")
	if err != nil {
		t.Fatal(err)
	}
	backend, closeBackend, err := semantic.NewPinnedBackend(target, os.Getenv("BUTLER_MEMORY_EVAL_LOCAL_TOKEN_FILE"))
	if err != nil {
		t.Fatal("pinned local backend unavailable")
	}
	defer closeBackend()
	state := memorySemanticState{Started: time.Now().UTC(), Bindings: map[string]map[string]catalog.SourceRef{}, Measurements: acceptance.MemoryMeasurements{Observations: []acceptance.MemoryObservation{}, Expenses: []acceptance.MemoryExpense{}}}
	state.Deadline = state.Started.Add(4 * time.Hour)
	if phase == "read" {
		b, err = os.ReadFile(filepath.Join(private, "writer-state.json"))
		if err != nil || json.Unmarshal(b, &state) != nil {
			t.Fatal("writer's immutable state missing")
		}
	}
	ctx, cancel := context.WithDeadline(context.Background(), state.Deadline)
	defer cancel()
	plan := acceptance.MemoryPlan{Version: acceptance.MemorySemanticVersion, WindowID: "fresh-semantic-" + release[:12], CodeCommit: release, Model: antigravity.FlashHighModel, ModelVersion: "gemini-3.8-flash-n", Runtime: "current-Catalog/index/HybridReader/BuildBundle/native-5min", MemoryConfigSHA256: memoryEvalConfigDigest()}
	c := &memoryEvalCollector{pool: pool, catalog: cat, budgets: budgets, backend: backend, suite: acceptance.MemorySuite{Version: acceptance.MemorySemanticVersion}, plan: plan, private: private, sourceRefs: state.Bindings, started: state.Started, deadline: state.Deadline, measurements: state.Measurements, freeAnswer: true, captureMilliseconds: map[string]int64{}}
	if phase == "write" {
		if err = c.init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	handoff := false
	defer func() {
		if e := c.finishSemantic(handoff); e != nil {
			t.Error("retain and seal semantic trial", e)
		}
	}()
	if phase == "write" {
		for _, cse := range suite.Cases {
			if err = c.capture(ctx, cse.MemoryCase()); err != nil {
				t.Fatal(cse.ID, err)
			}
		}
		state.Bindings = c.sourceRefs
		state.Measurements = c.measurements
		if err = c.writeEvidence("writer-state.json", state); err != nil {
			t.Fatal(err)
		}
		if err = c.writeEvidence("writer-captures.json", c.captures); err != nil {
			t.Fatal(err)
		}
		t.Log("writer committed and exits; native calls=0")
		handoff = true
		return
	}
	// The post-restart query process has its own exclusive artifact directory.
	readDir := filepath.Join(private, "read-after-restart")
	if err = os.Mkdir(readDir, 0700); err != nil {
		t.Fatal(err)
	}
	c.private = readDir
	c.readSuffix = "-after-restart"
	for _, cse := range suite.Cases {
		mc := cse.MemoryCase()
		id := acceptance.MemoryOpaqueCaseID(mc)
		all := []catalog.SourceRef{}
		logical := map[string]acceptance.MemorySource{}
		for _, h := range mc.History {
			if ref, ok := c.sourceRefs[id][h.ID]; ok {
				all = append(all, ref)
				logical[ref.ID] = h
			}
		}
		live, run, e := c.ownedRun(ctx, mc, "retrieval-after-restart")
		if e != nil {
			t.Fatal(e)
		}
		start := time.Now()
		if e = c.captureRead(ctx, mc, all, logical, live, run); e != nil {
			t.Fatal(mc.ID, e)
		}
		c.captureMilliseconds[id] = time.Since(start).Milliseconds()
	}
	inputs, err := semanticFreshInputs(suite, c.captures)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.writeEvidence("inputs-before-native.json", struct {
		Inputs []acceptance.MemoryInput `json:"inputs"`
	}{inputs}); err != nil {
		t.Fatal(err)
	}
	if err = c.writeEvidence("captures-before-native.json", c.captures); err != nil {
		t.Fatal(err)
	}
	// Auth is imported only inside the admitted fresh runtime; key bytes and raw
	// provider diagnostics never leave the private mount/owning encrypted store.
	c.factory, err = newSchemaDialectNativeFactoryWithTimeout(ctx, pool, os.Getenv("BUTLER_MEMORY_EVAL_IMPORT_FILE"), filepath.Join(private, "native-auth.key"), schemaDialectGoogle, memoryEvalModelTimeout)
	if err != nil {
		t.Fatal("native factory denied")
	}
	if err = collectMemorySemanticSamples(ctx, c, suite, inputs, func(o acceptance.MemoryObservation) { t.Log(o.CaseID, o.Arm, o.Status, o.Reason) }); err != nil {
		t.Fatal("retained trial prefix; no retry", err)
	}
}
