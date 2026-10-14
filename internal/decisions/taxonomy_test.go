package decisions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/semantic"
)

type taxonomyEvent struct {
	ID         string `json:"id"`
	Split      string `json:"split"`
	Gold       string `json:"gold"`
	SourceRole string `json:"source_role"`
	Context    string `json:"context"`
	Text       string `json:"text"`
	Evidence   string `json:"evidence_status"`
}

// Only observable fixture input is serialized. Gold, split, ID and critical
// annotations never enter a model prompt. Fixture provenance is not real auth.
func (c taxonomyEvent) input() string {
	b, _ := json.Marshal(struct {
		SourceRole string `json:"source_role"`
		Context    string `json:"context"`
		Text       string `json:"text"`
		Evidence   string `json:"evidence_status"`
	}{c.SourceRole, c.Context, c.Text, c.Evidence})
	return string(b)
}
func (c taxonomyEvent) annotated() eventCase {
	control := ""
	if c.Gold == "остановка" {
		control = "stop"
	}
	return eventCase{ID: c.ID, Split: c.Split, Text: c.Text, Gold: c.Gold, Control: control, Important: c.Gold == "важное событие"}
}
func readTaxonomyEvents(t *testing.T, phase string) ([]taxonomyEvent, []byte) {
	t.Helper()
	if phase == "replay" {
		raw, e := os.ReadFile(filepath.Join("..", "..", "deploy", "local-models", "russian-decisions.json"))
		if e != nil {
			t.Fatal(e)
		}
		var old suite
		if e = json.Unmarshal(raw, &old); e != nil {
			t.Fatal(e)
		}
		roles := map[string]string{"владелец": "owner", "мониторинг": "monitor", "каталог памяти": "memory_catalog", "внешний документ": "document", "внешняя веб-страница": "page", "извлечённая запись памяти": "retrieved_memory", "исполнитель": "executor", "система проверки": "verifier", "HTTP-инструмент": "tool"}
		var events []taxonomyEvent
		for _, c := range old.Cases {
			evidence := "none"
			if c.VerifiedProof {
				evidence = "current_pass"
			}
			events = append(events, taxonomyEvent{c.ID, "replay", c.Gold, roles[c.Origin], "", c.Text, evidence})
		}
		return events, raw
	}
	raw, e := os.ReadFile(filepath.Join("..", "..", "deploy", "local-models", "russian-decisions-independent-v2.json"))
	if e != nil {
		t.Fatal(e)
	}
	var corpus struct {
		Version string          `json:"version"`
		Cases   []taxonomyEvent `json:"cases"`
	}
	if e = json.Unmarshal(raw, &corpus); e != nil || corpus.Version != "butler-russian-independent-v2" {
		t.Fatal("invalid independent corpus", e)
	}
	var events []taxonomyEvent
	for _, c := range corpus.Cases {
		if c.Split == phase {
			events = append(events, c)
		}
	}
	if len(events) == 0 {
		t.Fatal("empty split")
	}
	return events, raw
}

func TestTaxonomyInputDoesNotLeakAnswers(t *testing.T) {
	c := taxonomyEvent{ID: "hidden-id", Split: "test", Gold: "hidden-gold", SourceRole: "owner", Context: "обсуждали сервер", Text: "отмена", Evidence: "none"}
	for _, hidden := range []string{c.ID, c.Split, c.Gold, "gold", "split"} {
		if strings.Contains(c.input(), hidden) {
			t.Fatal("answer annotation leaked")
		}
	}
	events, _ := readTaxonomyEvents(t, "test")
	dev, _ := readTaxonomyEvents(t, "dev")
	if len(events) != 90 || len(dev) != 18 {
		t.Fatal("frozen split changed")
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	for _, c := range append(dev, events...) {
		if seen[c.ID] || seen[c.input()] {
			t.Fatal("duplicate case")
		}
		seen[c.ID] = true
		seen[c.input()] = true
		counts[c.Split+":"+c.Gold]++
	}
	for _, label := range Categories() {
		if counts["test:"+label] != 10 || counts["dev:"+label] != 2 {
			t.Fatal("unbalanced class", label)
		}
	}
}

func TestActualRussianDecisionTaxonomyV2(t *testing.T) {
	token := os.Getenv("BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE")
	if token == "" {
		t.Skip("isolated tensor service required")
	}
	phase := os.Getenv("BUTLER_TEST_DECISION_PHASE")
	if phase != "replay" && phase != "dev" && phase != "test" {
		t.Fatal("explicit evaluation phase required")
	}
	cases, raw := readTaxonomyEvents(t, phase)
	f := newFixture(t)
	backend, closeBackend, e := semantic.NewPinnedBackend(f.target, token)
	if e != nil {
		t.Fatal(e)
	}
	defer closeBackend()
	profiles := TaxonomyProfiles()
	if os.Getenv("BUTLER_TEST_DECISION_ONLY_QWEN") == "1" {
		profiles = nil
	}
	expectedAttempts := len(cases) * len(profiles)
	if os.Getenv("BUTLER_TEST_QWEN_EVALUATION") == "1" {
		expectedAttempts += len(cases)
	}
	// One explicit finite research policy, established before any reservation.
	// Production's 256-turn daily default is not modified or reset by the suite.
	f.budgets.Policy.Day.ModelTurns = int64(expectedAttempts)
	recorder := &recording{Backend: backend}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	totals := map[string]*metrics{}
	var results []result
	qwenAttempts := 0
	complete := false
	checkpoint := func() {
		body, _ := json.MarshalIndent(map[string]any{"report_version": "decision-taxonomy-v2-checkpoint", "complete": complete, "phase": phase, "totals": totals, "cases": results, "actual_attempts": recorder.Attempts + qwenAttempts, "expected_attempts": expectedAttempts}, "", "  ")
		path := os.Getenv("BUTLER_TEST_DECISION_REPORT") + ".checkpoint.json"
		if err := os.WriteFile(path+".tmp", body, 0644); err != nil {
			t.Errorf("checkpoint failed: %v", err)
			return
		}
		if err := os.Rename(path+".tmp", path); err != nil {
			t.Errorf("checkpoint publish failed: %v", err)
		}
	}
	defer checkpoint()
	for _, p := range profiles {
		observer, e := New(f.catalog, f.jobs, recorder, f.target, p.ID, budget.ModelPrice{Basis: "local_no_billing"})
		if e != nil {
			t.Fatal(e)
		}
		m := &metrics{}
		totals[p.ID] = m
		for i, c := range cases {
			j, ref := f.job(t, p.ID+":"+phase+":"+c.ID, c.input())
			start := time.Now()
			recorder.Status = ""
			callCtx := ctx
			cancelCall := func() {}
			if i > 0 {
				callCtx, cancelCall = context.WithTimeout(ctx, 10*time.Second)
			}
			o, err := observer.Observe(callCtx, j, ref)
			cancelCall()
			wall := time.Since(start).Milliseconds()
			fault := ""
			if err != nil {
				fault = recorder.Status
				if fault == "" || fault == "OK" {
					fault = "caller_rejected"
				}
				o = Observation{Label: "недостаточно данных", Escalate: true, Insufficient: true, ShadowOnly: true, EventPreserved: true}
			}
			m.add(c.annotated(), o.Label, o.Escalate, fault, wall, o, i == 0)
			results = append(results, result{c.ID, phase, c.Gold, p.ID, o, fault, wall})
			checkpoint()
			if e = f.jobs.Finish(context.Background(), j, fault, false); e != nil {
				t.Fatal(e)
			}
		}
		m.finish()
		t.Logf("%s accuracy=%.3f observed=%d unavailable=%d", p.ID, m.Accuracy, m.Observed, m.Errors)
	}
	schema, e := os.ReadFile(filepath.Join("..", "localmodels", "decision-schema-v2.json"))
	if e != nil {
		t.Fatal(e)
	}
	if os.Getenv("BUTLER_TEST_QWEN_EVALUATION") == "1" {
		m, rows := evaluateQwen(t, f, backend, cases, schema)
		totals[qwenEvaluationID] = m
		results = append(results, rows...)
		qwenAttempts = len(rows)
		t.Logf("Qwen accuracy=%.3f observed=%d unavailable=%d", m.Accuracy, m.Observed, m.Errors)
	}
	complete = true
	digest := sha256.Sum256(raw)
	schemaDigest := sha256.Sum256(schema)
	report := map[string]any{"report_version": "decision-taxonomy-v2", "phase": phase, "corpus_sha256": hex.EncodeToString(digest[:]), "schema_sha256": hex.EncodeToString(schemaDigest[:]), "profiles": profiles, "totals": totals, "cases": results, "actual_attempts": recorder.Attempts + qwenAttempts, "production_enabled": false, "actual_controller_calls_saved": 0, "thresholds_tuned_on_test": false, "qwen_co_residency_requested": os.Getenv("BUTLER_TEST_QWEN_CORESIDENT") == "1", "qwen_resource_metrics": "external whole-device sampler and pinned /api/ps; observation zero means unmeasured", "limitations": []string{"new synthetic events authored and frozen before any model predictions; no human review or production-distribution claim", "source role and proof metadata are fixtures, not live authenticated authority", "controller cost baseline remains unmeasured"}}
	report["finite_research_policy"] = map[string]any{"model_turns": expectedAttempts, "cold_timeout_ms": 60000, "warm_timeout_ms": 10000, "production_daily_policy_changed": false}
	body, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	path := os.Getenv("BUTLER_TEST_DECISION_REPORT")
	if path == "" {
		t.Fatal("report path required")
	}
	if e = os.WriteFile(path, body, 0644); e != nil {
		t.Fatal(e)
	}
}
