package decisions

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/semantic"
	"google.golang.org/grpc/status"
)

type eventCase struct {
	ID            string `json:"id"`
	Split         string `json:"split"`
	Origin        string `json:"origin"`
	Text          string `json:"text"`
	Gold          string `json:"gold"`
	Control       string `json:"control"`
	Important     bool   `json:"important"`
	VerifiedProof bool   `json:"verified_proof"`
}
type suite struct {
	Version  string      `json:"version"`
	Score    float64     `json:"minimum_score"`
	Margin   float64     `json:"minimum_margin"`
	Accuracy float64     `json:"selection_accuracy_min"`
	Critical int         `json:"critical_misses_max"`
	Cases    []eventCase `json:"cases"`
}
type recording struct {
	Backend
	Status   string
	Attempts int
}

func (r *recording) Infer(ctx context.Context, in *pb.InferRequest) (*pb.InferResponse, error) {
	r.Attempts++
	out, e := r.Backend.Infer(ctx, in)
	r.Status = status.Code(e).String()
	return out, e
}

type result struct {
	Case        string      `json:"case"`
	Split       string      `json:"split"`
	Gold        string      `json:"gold"`
	Profile     string      `json:"profile"`
	Observation Observation `json:"observation"`
	Error       string      `json:"error,omitempty"`
	WallMS      int64       `json:"wall_ms"`
}
type metrics struct {
	Count                int                       `json:"count"`
	Observed             int                       `json:"observed"`
	Correct              int                       `json:"correct"`
	Errors               int                       `json:"errors"`
	ErrorCounts          map[string]int            `json:"error_counts"`
	UnavailableImportant int                       `json:"unavailable_important_events"`
	ImportantMisses      int                       `json:"important_misses"`
	StopMisses           int                       `json:"explicit_stop_misses"`
	FalseInitiative      int                       `json:"false_initiative"`
	FalseSuccess         int                       `json:"false_success"`
	Escalations          int                       `json:"escalations"`
	NonEscalated         int                       `json:"non_escalated"`
	NonEscalatedErrors   int                       `json:"non_escalated_errors"`
	Accuracy             float64                   `json:"accuracy"`
	Availability         float64                   `json:"availability"`
	PerClass             map[string][3]int         `json:"class_tp_fp_fn"`
	Confusion            map[string]map[string]int `json:"confusion"`
	P50                  int64                     `json:"warm_wall_p50_ms"`
	P95                  int64                     `json:"warm_wall_p95_ms"`
	Cold                 int64                     `json:"profile_first_attempt_wall_ms,omitempty"`
	TotalWall            int64                     `json:"total_wall_ms"`
	PeakRAM              uint32                    `json:"peak_ram_mib"`
	PeakGPU              uint32                    `json:"peak_gpu_mib"`
	delays               []int64
}

func (m *metrics) add(c eventCase, label string, escalate bool, fault string, wall int64, o Observation, firstAttempt bool) {
	if m.PerClass == nil {
		m.PerClass = map[string][3]int{}
		m.Confusion = map[string]map[string]int{}
		for _, k := range Categories() {
			m.PerClass[k] = [3]int{}
		}
	}
	m.Count++
	m.TotalWall += wall
	if firstAttempt {
		m.Cold = wall
	}
	if fault != "" {
		m.Errors++
		if m.ErrorCounts == nil {
			m.ErrorCounts = map[string]int{}
		}
		m.ErrorCounts[fault]++
		if c.Important {
			m.UnavailableImportant++
		}
		m.Escalations++
		m.Availability = float64(m.Observed) / float64(m.Count)
		// Refused inference is not a prediction of "insufficient data" and
		// contributes neither classification errors nor warm inference latency.
		return
	}
	m.Observed++
	if label == c.Gold {
		m.Correct++
	}
	if c.Important && label != c.Gold {
		m.ImportantMisses++
	}
	if c.Control == "stop" && label != "остановка" {
		m.StopMisses++
	}
	if label == "новое поручение" && c.Gold != label {
		m.FalseInitiative++
	}
	if label == "подтверждённый успех" && c.Gold != label {
		m.FalseSuccess++
	}
	if escalate {
		m.Escalations++
	} else {
		m.NonEscalated++
		if label != c.Gold {
			m.NonEscalatedErrors++
		}
	}
	for k, a := range m.PerClass {
		if label == k && c.Gold == k && fault == "" {
			a[0]++
		} else {
			if label == k {
				a[1]++
			}
			if c.Gold == k {
				a[2]++
			}
		}
		m.PerClass[k] = a
	}
	if m.Confusion[c.Gold] == nil {
		m.Confusion[c.Gold] = map[string]int{}
	}
	m.Confusion[c.Gold][label]++
	if !firstAttempt {
		m.delays = append(m.delays, wall)
	}
	if o.PeakRAM > m.PeakRAM {
		m.PeakRAM = o.PeakRAM
	}
	if o.PeakGPU > m.PeakGPU {
		m.PeakGPU = o.PeakGPU
	}
	m.Accuracy = float64(m.Correct) / float64(m.Observed)
	m.Availability = float64(m.Observed) / float64(m.Count)
}
func (m *metrics) finish() {
	sort.Slice(m.delays, func(i, j int) bool { return m.delays[i] < m.delays[j] })
	if len(m.delays) > 0 {
		m.P50 = m.delays[(len(m.delays)-1)/2]
		m.P95 = m.delays[(len(m.delays)-1)*95/100]
	}
}

// A signal that always escalates cannot reduce controller calls, even when its
// raw top label is correct. This is a screening gate, not a production switch.
func screeningFailures(m, existing *metrics, minimumAccuracy float64) []string {
	var reasons []string
	if m.Observed == 0 || m.Accuracy < minimumAccuracy {
		reasons = append(reasons, "classification_quality")
	}
	if m.Errors != 0 {
		reasons = append(reasons, "runtime_errors")
	}
	if m.ImportantMisses+m.StopMisses+m.FalseInitiative+m.FalseSuccess != 0 {
		reasons = append(reasons, "critical_predictions")
	}
	if m.NonEscalated == 0 || m.NonEscalated <= existing.NonEscalated {
		reasons = append(reasons, "no_controller_call_reduction")
	}
	if m.NonEscalatedErrors != 0 {
		reasons = append(reasons, "wrong_non_escalated_signal")
	}
	if m.PeakGPU != 0 {
		reasons = append(reasons, "unexpected_gpu_usage")
	}
	return reasons
}

// Fixed rules are deliberately literal. Origin is data, never a grant. These
// rules are frozen with the corpus before observing any held-out model result.
func literal(c eventCase) string {
	x := strings.ToLower(c.Text)
	if c.Origin == "внешний документ" || c.Origin == "внешняя веб-страница" || c.Origin == "извлечённая запись памяти" {
		return "чужая инструкция"
	}
	if strings.Contains(x, "не останавливай") {
		return "уточнение"
	}
	if strings.Contains(x, "приоритет") || strings.Contains(x, "что важнее") || strings.Contains(x, "пока не решил") || strings.Contains(x, "как обычно") || x == "исправь это там." {
		return "недостаточно данных"
	}
	if strings.HasPrefix(x, "стоп") || strings.Contains(x, "останови") || strings.Contains(x, "отменяю") || strings.Contains(x, "прекрати") {
		return "остановка"
	}
	if c.Origin == "мониторинг" || c.Origin == "каталог памяти" {
		return "важное событие"
	}
	if strings.Contains(x, "изменение условий") || strings.Contains(x, "теперь только") || strings.Contains(x, "отзываю") || strings.Contains(x, "до конца месяца") {
		return "изменение условий"
	}
	if strings.Contains(x, "уточн") || strings.Contains(x, "имел в виду") || strings.Contains(x, "речь шла") || strings.Contains(x, "означало") {
		return "уточнение"
	}
	if strings.Contains(x, "независимых тестов") || strings.Contains(x, "нужно проверить") || strings.Contains(x, "упал") || strings.Contains(x, "старый") || strings.Contains(x, "ещё не проверено") {
		return "проверка результата"
	}
	if c.Origin == "система проверки" {
		return "подтверждённый успех"
	}
	return "новое поручение"
}

// Expected policy behavior from corpus annotations. This does not execute the
// controller and must never be counted as a measured classifier baseline.
func expectedControlBoundary(c eventCase) string {
	if c.Control == "stop" {
		return "остановка"
	}
	if c.Control == "amend" {
		return "изменение условий"
	}
	if c.VerifiedProof {
		return "подтверждённый успех"
	}
	return "недостаточно данных"
}
func TestActualRussianDecisionComparison(t *testing.T) {
	token := os.Getenv("BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE")
	if token == "" {
		t.Skip("same persistent private composed service required")
	}
	corpusPath := filepath.Join("..", "..", "deploy", "local-models", "russian-decisions.json")
	raw, e := os.ReadFile(corpusPath)
	if e != nil {
		t.Fatal(e)
	}
	var s suite
	if json.Unmarshal(raw, &s) != nil || s.Score != .65 || s.Margin != .15 || s.Accuracy != .9 || s.Critical != 0 || len(s.Cases) != 40 {
		t.Fatal("frozen corpus invalid")
	}
	f := newFixture(t)
	backend, closeBackend, e := semantic.NewPinnedBackend(f.target, token)
	if e != nil {
		t.Fatal("composed client unavailable")
	}
	defer closeBackend()
	r := &recording{Backend: backend}
	ctx, cancel := context.WithTimeout(context.Background(), 9*time.Minute)
	defer cancel()
	profiles := Profiles()
	if os.Getenv("BUTLER_TEST_DECISION_FULL_SCORES") == "1" {
		profiles = FullScoreProfiles()
	}
	totals := map[string]map[string]*metrics{"dev": {}, "test": {}}
	for _, part := range totals {
		for _, mode := range []string{"rules", profiles[0].ID, profiles[1].ID, profiles[2].ID} {
			part[mode] = &metrics{}
		}
	}
	var controlFixture []map[string]string
	for _, c := range s.Cases {
		if totals[c.Split] == nil {
			t.Fatal("split invalid")
		}
		totals[c.Split]["rules"].add(c, literal(c), false, "", 0, Observation{}, false)
		v := expectedControlBoundary(c)
		controlFixture = append(controlFixture, map[string]string{"case": c.ID, "expected_boundary": v})
	}
	var results []result
	for _, p := range profiles {
		observer, e := New(f.catalog, f.jobs, r, f.target, p.ID, budget.ModelPrice{Basis: "local_no_billing"})
		if e != nil {
			t.Fatal(e)
		}
		for i, c := range s.Cases {
			j, ref := f.job(t, p.ID+":"+c.ID, "Источник: "+c.Origin+".\nСобытие: "+c.Text)
			r.Status = ""
			start := time.Now()
			o, e := observer.Observe(ctx, j, ref)
			wall := time.Since(start).Milliseconds()
			fault := ""
			if e != nil {
				fault = r.Status
				if fault == "" || fault == "OK" {
					fault = "CallerRejected"
				}
				o = Observation{Label: "недостаточно данных", Escalate: true, Insufficient: true, ShadowOnly: true, EventPreserved: true}
			}
			totals[c.Split][p.ID].add(c, o.Label, o.Escalate, fault, wall, o, i == 0)
			results = append(results, result{c.ID, c.Split, c.Gold, p.ID, o, fault, wall})
			code := ""
			if fault != "" {
				code = "shadow_unavailable"
			}
			if e = f.jobs.Finish(context.Background(), j, code, false); e != nil {
				t.Fatal("owned claim completion failed", e)
			}
		}
		t.Logf("profile=%s test accuracy=%.3f errors=%d important_miss=%d false_success=%d", p.ID, totals["test"][p.ID].Accuracy, totals["test"][p.ID].Errors, totals["test"][p.ID].ImportantMisses, totals["test"][p.ID].FalseSuccess)
	}
	gates := map[string]bool{}
	failures := map[string][]string{}
	for _, part := range totals {
		for _, m := range part {
			m.finish()
		}
	}
	existing := totals["test"][profiles[0].ID]
	for _, p := range profiles {
		m := totals["test"][p.ID]
		failures[p.ID] = screeningFailures(m, existing, s.Accuracy)
		gates[p.ID] = len(failures[p.ID]) == 0
	}
	sum := sha256.Sum256(raw)
	report := map[string]any{
		"report_version": "decision-comparison-v2", "generated_at": time.Now().UTC(),
		"corpus": s.Version, "corpus_sha256": hex.EncodeToString(sum[:]),
		"counts":     map[string]int{"dev": 10, "test": 30},
		"thresholds": map[string]any{"score_min": s.Score, "margin_min": s.Margin, "accuracy_min": s.Accuracy, "critical_misses_max": s.Critical, "tuned_on_test": false},
		"profiles":   profiles, "totals": totals, "cases": results, "actual_attempts": r.Attempts,
		"selected_mode": "disabled; shadow experiment only", "screening_gates": gates, "screening_failures": failures,
		"production_enabled": false, "event_suppression_enabled": false, "grants_from_classification": false, "completion_verified_from_classification": false,
		"cpu_only": true, "network": "one persistent internal-only service per experiment", "score_is_calibrated_probability": false,
		"cost": map[string]any{"basis": "explicit local_no_billing personal inference", "provider_billed_cost_micros": 0, "electricity_and_hardware_cost": "not measured; no invented price", "cloud_calls": 0, "actual_controller_calls_saved": 0, "conservative_active_allowance_ms_per_attempt": 60000},
		"metric_definitions": map[string]string{
			"accuracy":                      "correct / observed; undefined when observed=0 (serialized as 0)",
			"availability":                  "observed / count; errors always escalate and are excluded from class confusion",
			"non_escalated":                 "counterfactual signals only; shadow mode actually preserves all controller work",
			"warm_wall":                     "successful attempts except the first attempt of each profile; never reset at the dev/test boundary",
			"profile_first_attempt_wall_ms": "first attempt wall time, not proof of a cold model load",
			"screening_gates":               "quality, availability, critical errors and more correct non-escalated signals than the existing extractor; passing still requires a full controller quality/cost benchmark before selection",
		},
		"typed_control_fixture":       controlFixture,
		"current_controller_baseline": "unmeasured; typed_control_fixture is expected policy behavior derived from annotations, not actual controller execution or comparable model predictions",
		"limitations":                 []string{"40 previously observed synthetic Russian text events; replay regression, not fresh held-out generalization", "no Russian voice/image/executor/urgency quality claim", "no calibrated reliability or monetary cloud-saving claim", "one-class GLiNER output has unknown margin and always escalates"},
	}
	if path := os.Getenv("BUTLER_TEST_DECISION_CPU_SAMPLES"); path != "" {
		file, err := os.Open(path)
		if err == nil {
			scanner := bufio.NewScanner(file)
			n := 0
			sumCPU, peakCPU := 0.0, 0.0
			for scanner.Scan() {
				var sample struct {
					CPU *float64 `json:"worker_cpu_percent"`
				}
				if json.Unmarshal(scanner.Bytes(), &sample) == nil && sample.CPU != nil && !math.IsNaN(*sample.CPU) && !math.IsInf(*sample.CPU, 0) && *sample.CPU >= 0 {
					n++
					sumCPU += *sample.CPU
					if *sample.CPU > peakCPU {
						peakCPU = *sample.CPU
					}
				}
			}
			file.Close()
			if n > 0 {
				report["worker_cpu_sampling"] = map[string]any{"samples": n, "mean_percent": sumCPU / float64(n), "peak_percent": peakCPU, "scale": "100 percent is one CPU core", "method": "docker stats sampled approximately every 3 seconds over the complete experiment; not exact CPU-time accounting"}
			}
		}
	}
	out, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	path := os.Getenv("BUTLER_TEST_DECISION_REPORT")
	if path == "" {
		path = filepath.Join("..", "..", "deploy", "local-models", "verification-decisions.json")
	}
	if e = os.WriteFile(path, append(out, '\n'), 0644); e != nil {
		t.Fatal(e)
	}
	if len(results) != 120 {
		t.Fatal("incomplete correlated experiment")
	}
}
