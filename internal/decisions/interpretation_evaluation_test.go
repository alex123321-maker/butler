package decisions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/localmodels"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/semantic"
)

type interpretationRow struct {
	Case         string               `json:"case"`
	Window       int                  `json:"window"`
	Profile      string               `json:"profile"`
	SourceKind   string               `json:"source_kind_fixture"`
	Quote        InterpretationWindow `json:"source_window"`
	Gold         interpretationGold   `json:"gold"`
	Observation  Interpretation       `json:"observation"`
	Source       catalog.SourceRef    `json:"source"`
	Error        string               `json:"error,omitempty"`
	WallMS       int64                `json:"wall_ms"`
	Response     *pb.InferResponse    `json:"response,omitempty"`
	QwenResponse *qwenResponse        `json:"qwen_response,omitempty"`
}

type intentMeasure struct {
	TP        int     `json:"tp"`
	FP        int     `json:"fp"`
	FN        int     `json:"fn"`
	Precision float64 `json:"precision"`
	Recall    float64 `json:"recall"`
}
type interpretationMetrics struct {
	Windows                   int                       `json:"windows"`
	Observed                  int                       `json:"observed"`
	Unavailable               int                       `json:"unavailable"`
	IntentSetsCorrect         int                       `json:"intent_sets_correct"`
	ModalityCorrect           int                       `json:"modality_correct"`
	TargetCorrect             int                       `json:"target_correct"`
	JointCorrect              int                       `json:"joint_correct"`
	Events                    int                       `json:"events"`
	EventsCorrect             int                       `json:"events_correct"`
	NonEscalated              int                       `json:"non_escalated"`
	NonEscalatedWrong         int                       `json:"non_escalated_wrong"`
	DirectStopMissed          int                       `json:"direct_stop_missed"`
	DirectStopFalse           int                       `json:"direct_stop_false"`
	StopScopeWrong            int                       `json:"stop_scope_wrong"`
	ConstraintMissed          int                       `json:"direct_constraint_missed"`
	FalseDirectInitiative     int                       `json:"false_direct_initiative"`
	QuotationProposalPromoted int                       `json:"quotation_proposal_promoted"`
	QuoteValid                int                       `json:"source_window_valid"`
	ForeignSourcePromoted     int                       `json:"foreign_source_promoted"`
	ByIntent                  map[string]*intentMeasure `json:"by_intent"`
	ConfusionModality         map[string]map[string]int `json:"modality_confusion"`
	ConfusionTarget           map[string]map[string]int `json:"target_confusion"`
	FirstAttemptMS            int64                     `json:"first_attempt_ms"`
	WarmP50MS                 int64                     `json:"warm_p50_ms"`
	WarmP95MS                 int64                     `json:"warm_p95_ms"`
	PeakRAM                   uint32                    `json:"peak_service_ram_mib"`
	ScreenPassed              bool                      `json:"provisional_screen_passed"`
}

func summarizeInterpretation(rows []interpretationRow) interpretationMetrics {
	m := interpretationMetrics{ByIntent: map[string]*intentMeasure{}, ConfusionModality: map[string]map[string]int{}, ConfusionTarget: map[string]map[string]int{}}
	_, d := localmodels.InterpretationSchema()
	for _, intent := range d.Intents {
		m.ByIntent[intent.ID] = &intentMeasure{}
	}
	events := map[string]bool{}
	var warm []int64
	for i, r := range rows {
		m.Windows++
		if _, ok := events[r.Case]; !ok {
			events[r.Case] = true
		}
		if i == 0 {
			m.FirstAttemptMS = r.WallMS
		}
		if r.Error != "" {
			m.Unavailable++
			events[r.Case] = false
			continue
		}
		m.Observed++
		m.QuoteValid++
		if i > 0 {
			warm = append(warm, r.WallMS)
		}
		if r.Response != nil {
			m.PeakRAM = max(m.PeakRAM, r.Response.PeakRamMib)
		}
		gold, o := r.Gold, r.Observation
		intentCorrect := sameIntents(gold.Intents, o.Intents)
		if intentCorrect {
			m.IntentSetsCorrect++
		}
		if gold.Modality == o.Modality {
			m.ModalityCorrect++
		}
		if gold.Target == o.Target {
			m.TargetCorrect++
		}
		joint := intentCorrect && gold.Modality == o.Modality && gold.Target == o.Target
		if joint {
			m.JointCorrect++
		} else {
			events[r.Case] = false
		}
		if !o.Escalate {
			m.NonEscalated++
			if !joint {
				m.NonEscalatedWrong++
			}
		}
		for _, intent := range d.Intents {
			g, p := hasIntent(gold.Intents, intent.ID), hasIntent(o.Intents, intent.ID)
			count := m.ByIntent[intent.ID]
			if g && p {
				count.TP++
			}
			if !g && p {
				count.FP++
			}
			if g && !p {
				count.FN++
			}
		}
		for _, pair := range []struct {
			table      map[string]map[string]int
			gold, pred string
		}{{m.ConfusionModality, gold.Modality, o.Modality}, {m.ConfusionTarget, gold.Target, o.Target}} {
			if pair.table[pair.gold] == nil {
				pair.table[pair.gold] = map[string]int{}
			}
			pair.table[pair.gold][pair.pred]++
		}
		gStop := hasIntent(gold.Intents, "stop_work") && gold.Modality == "direct"
		pStop := hasIntent(o.Intents, "stop_work") && o.Modality == "direct"
		if gStop && !pStop {
			m.DirectStopMissed++
		}
		if pStop && !gStop {
			m.DirectStopFalse++
		}
		if gStop && pStop && gold.Target != o.Target {
			m.StopScopeWrong++
		}
		if gold.Modality == "direct" && hasIntent(gold.Intents, "change_constraint") && !hasIntent(o.Intents, "change_constraint") {
			m.ConstraintMissed++
		}
		if o.Modality == "direct" {
			falseInitiative := false
			for _, intent := range []string{"request_action", "change_constraint", "stop_work"} {
				if hasIntent(o.Intents, intent) && (gold.Modality != "direct" || !hasIntent(gold.Intents, intent)) {
					falseInitiative = true
				}
			}
			if falseInitiative {
				m.FalseDirectInitiative++
				if gold.Modality != "direct" {
					m.QuotationProposalPromoted++
				}
			}
		}
		if r.SourceKind != "owner" && TrustedSourceAllowsOwnerIntent(r.SourceKind, o) {
			m.ForeignSourcePromoted++
		}
	}
	m.Events = len(events)
	for _, ok := range events {
		if ok {
			m.EventsCorrect++
		}
	}
	m.ScreenPassed = m.Observed > 0 && m.Unavailable == 0 && float64(m.JointCorrect)/float64(m.Observed) >= .9 && m.NonEscalated > 0 && m.NonEscalatedWrong == 0 && m.DirectStopMissed == 0 && m.DirectStopFalse == 0 && m.StopScopeWrong == 0 && m.ConstraintMissed == 0 && m.FalseDirectInitiative == 0
	for _, c := range m.ByIntent {
		if c.TP+c.FP > 0 {
			c.Precision = float64(c.TP) / float64(c.TP+c.FP)
		}
		if c.TP+c.FN > 0 {
			c.Recall = float64(c.TP) / float64(c.TP+c.FN)
		}
		if c.Precision < .9 || c.Recall < .8 {
			m.ScreenPassed = false
		}
	}
	m.WarmP50MS = percentile(warm, .5)
	m.WarmP95MS = percentile(warm, .95)
	return m
}

func percentile(values []int64, p float64) int64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]int64(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	return copyValues[int(float64(len(copyValues)-1)*p)]
}

func (f *fixture) interpretationJob(t *testing.T, id, body string, tokens int64) (jobs.Job, catalog.SourceRef) {
	t.Helper()
	src, err := f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: id, Kind: "document", ScopeType: "user", ScopeID: "owner", Author: "public synthetic interpretation fixture", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	ref := catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
	if _, err = f.jobs.Enqueue(f.owner, jobs.Request{ID: "interpretation-" + id, Kind: "extract", Purpose: "decision_shadow", Sources: []catalog.SourceRef{ref}, Allowance: budget.Units{ModelTurns: 1, Tokens: tokens, ActiveMillis: 60000, ProviderInputBytes: 16000}, Deadline: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	j, ok, err := f.jobs.Claim(context.Background())
	if err != nil || !ok {
		t.Fatal("job not admitted", err)
	}
	return j, ref
}

func writeInterpretationReport(t *testing.T, path string, report any) {
	t.Helper()
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path+".tmp", raw, 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path+".tmp", path); err != nil {
		t.Fatal(err)
	}
}

func TestActualInterpretationV3Comparison(t *testing.T) {
	token := os.Getenv("BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE")
	if token == "" {
		t.Skip("isolated tensor service required")
	}
	phase := os.Getenv("BUTLER_TEST_DECISION_PHASE")
	if phase != "dev" && phase != "test" {
		t.Fatal("explicit dev/test required")
	}
	cases, corpus := readInterpretationCorpus(t, phase)
	path := os.Getenv("BUTLER_TEST_DECISION_REPORT")
	if path == "" {
		t.Fatal("report path required")
	}
	f := newFixture(t)
	backend, closeBackend, err := semantic.NewPinnedBackend(f.target, token)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBackend()
	profiles := InterpretationProfiles()
	windows := 0
	adequate := 0
	for _, c := range cases {
		windows += len(InterpretationWindows(c.Text))
		if c.Adequate {
			adequate++
		}
	}
	planned := 4 * windows
	// Finite prospective fixture allowance, established before the first model call.
	// Qwen residency adds one initial CPU embedding probe and one after each
	// Qwen window. Their distinct reservations are part of this finite plan.
	f.budgets.Policy.Day.ModelTurns = int64(planned + windows + 1)
	recorder := &recording{Backend: backend}
	var all []interpretationRow
	totals := map[string]interpretationMetrics{}
	complete := false
	qwenAttempts := 0
	checkpoint := func() {
		writeInterpretationReport(t, path+".checkpoint.json", map[string]any{"complete": complete, "phase": phase, "cases": all, "totals": totals, "planned_attempts": planned, "actual_attempts": recorder.Attempts + qwenAttempts})
	}
	defer checkpoint()
	for _, p := range profiles {
		var rows []interpretationRow
		for _, c := range cases {
			for i, w := range InterpretationWindows(c.Text) {
				if ValidateInterpretationQuote(c.Text, w) != nil {
					t.Fatal("source quote invalid")
				}
				j, ref := f.interpretationJob(t, p.ID+":"+phase+":"+c.ID+fmt.Sprint(i), interpretationInput(c.Text, c.Context, w), 7*2048)
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				if len(rows) > 0 {
					cancel()
					ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
				}
				start := time.Now()
				recorder.Status = ""
				o, response, callError := Interpret(ctx, f.catalog, f.jobs, recorder, f.target, p, j, ref)
				cancel()
				fault := ""
				if callError != nil {
					fault = recorder.Status
					if fault == "" || fault == "OK" {
						fault = "caller_rejected"
					}
					o = Interpretation{Escalate: true, ConfidenceUnknown: true}
				}
				row := interpretationRow{Case: c.ID, Window: i, Profile: p.ID, SourceKind: c.SourceKind, Quote: w, Gold: c.Gold[i], Observation: o, Source: ref, Error: fault, WallMS: time.Since(start).Milliseconds(), Response: response}
				rows = append(rows, row)
				all = append(all, row)
				checkpoint()
				if err = f.jobs.Finish(context.Background(), j, fault, false); err != nil {
					t.Fatal(err)
				}
			}
		}
		totals[p.ID] = summarizeInterpretation(rows)
		m := totals[p.ID]
		t.Logf("%s observed=%d unavailable=%d intent=%d joint=%d / %d", p.ID, m.Observed, m.Unavailable, m.IntentSetsCorrect, m.JointCorrect, m.Windows)
	}
	qwen, err := newInterpretationQwen(t, f, backend)
	if err != nil {
		t.Fatal(err)
	}
	var qwenRows []interpretationRow
	for _, c := range cases {
		for i, w := range InterpretationWindows(c.Text) {
			row, attempted := qwen.observe(t, c, i, w, len(qwenRows) == 0)
			if attempted {
				qwenAttempts++
			}
			qwenRows = append(qwenRows, row)
			all = append(all, row)
			checkpoint()
		}
	}
	qwen.finish(t, path)
	totals[interpretationQwenID] = summarizeInterpretation(qwenRows)
	ledger, err := f.budgets.Ledger.Get(context.Background(), budget.ScopeID("owner_day", "owner", time.Now().UTC().Format("2006-01-02")))
	if err != nil || ledger.Used.ModelTurns != int64(recorder.Attempts+qwenAttempts+len(qwen.embedding)) {
		t.Fatal("actual classification/probe attempts differ from durable reservations", err, ledger)
	}
	var rules []interpretationRow
	for _, c := range cases {
		for i, w := range InterpretationWindows(c.Text) {
			start := time.Now()
			o := RuleInterpretation(w.Quote, c.Context)
			rules = append(rules, interpretationRow{Case: c.ID, Window: i, Profile: InterpretationRulesVersion, SourceKind: c.SourceKind, Quote: w, Gold: c.Gold[i], Observation: o, WallMS: time.Since(start).Milliseconds()})
		}
	}
	totals[InterpretationRulesVersion] = summarizeInterpretation(rules)
	all = append(all, rules...)
	complete = true
	schema, _ := localmodels.InterpretationSchema()
	digest := sha256.Sum256(corpus)
	sd := sha256.Sum256(schema)
	freeze, err := os.ReadFile(filepath.Join("..", "..", "deploy", "local-models", "interpretation-v3-freeze.json"))
	if err != nil {
		t.Fatal(err)
	}
	report := map[string]any{"report_version": "interpretation-comparison-v3", "complete": true, "phase": phase, "corpus_sha256": hex.EncodeToString(digest[:]), "schema_sha256": hex.EncodeToString(sd[:]), "freeze": json.RawMessage(freeze), "events": len(cases), "windows": windows, "window_adequate_events": adequate, "totals": totals, "cases": all, "profiles": profiles, "planned_attempts": planned, "actual_attempts": recorder.Attempts + qwenAttempts, "production_enabled": false, "actual_controller_calls_saved": 0, "controller_baseline": "unmeasured", "thresholds_tuned_on_test": false, "quote_method": "deterministic code-derived source windows, not learned minimal spans", "input_provenance": "synthetic source kind is outside model input; real owner authentication is unmeasured", "cpu_tokens_reserved_per_window": 7 * 2048, "qwen_tokens_reserved_per_window": interpretationQwenTokens, "cold_timeout_ms": 60000, "warm_timeout_ms": 20000, "limitations": []string{"new synthetic corpus with one author, no independent human review or real-trace distribution claim", "CPU multi-question full input has per-encoded-row bounds and conservative seven-row reservations", "Qwen research fixture shares the private composed admission boundary; production Qwen transport remains unimplemented", "conditional token and classifier scores are not correctness probabilities", "every window is preserved, source-span validity does not establish semantic extraction quality"}}
	report["actual_embedding_attempts"] = len(qwen.embedding)
	report["qwen_tokens_reserved_per_window"] = interpretationQwenTokens
	report["planned_embedding_attempts"] = windows + 1
	report["actual_auxiliary_turns"] = ledger.Used.ModelTurns
	report["durable_owner_day_ledger"] = ledger
	writeInterpretationReport(t, path, report)
	for id, m := range totals {
		t.Logf("FINAL %s intent=%d/%d joint=%d/%d non-escalated=%d wrong=%d screen=%v", id, m.IntentSetsCorrect, m.Observed, m.JointCorrect, m.Observed, m.NonEscalated, m.NonEscalatedWrong, m.ScreenPassed)
	}
}
