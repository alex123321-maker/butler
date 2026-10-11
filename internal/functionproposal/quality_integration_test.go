package functionproposal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/localmodels"
	"github.com/butler/butler/internal/memory/semantic"
	"google.golang.org/grpc/status"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

type functionCase struct {
	ID, Split, Role, Text, Name string
	Args                        map[string]any
}
type functionSuite struct {
	Version, Catalog     string
	MinimumExactAccuracy float64 `json:"minimum_exact_accuracy"`
	FalseCallsMax        int     `json:"false_calls_max"`
	Cases                []functionCase
}
type recording struct {
	Backend
	Code     string
	Attempts int
}

func (r *recording) Infer(ctx context.Context, in *pb.InferRequest) (*pb.InferResponse, error) {
	r.Attempts++
	out, e := r.Backend.Infer(ctx, in)
	r.Code = status.Code(e).String()
	return out, e
}

type qualityResult struct {
	ID          string         `json:"id"`
	Split       string         `json:"split"`
	Role        string         `json:"role"`
	GoldName    string         `json:"gold_name"`
	GoldArgs    map[string]any `json:"gold_args"`
	Observation Observation    `json:"observation"`
	RulesName   string         `json:"rules_name"`
	RulesArgs   map[string]any `json:"rules_args"`
	ModelExact  bool           `json:"model_exact"`
	RulesExact  bool           `json:"rules_exact"`
	Error       string         `json:"error,omitempty"`
	WallMS      int64          `json:"wall_ms"`
}

// Deliberately limited literal baseline, frozen before any held-out model call.
func literal(text, role string) (string, map[string]any) {
	empty := map[string]any{}
	v := strings.ToLower(strings.TrimSpace(text))
	if role != "owner" || strings.Contains(v, "не ") || strings.Contains(v, "не запускай") || strings.Contains(v, "через ") || strings.Contains(v, "если ") {
		return "", empty
	}
	if strings.HasPrefix(v, "проверь") || strings.HasPrefix(v, "сделай диагностику") {
		if strings.Contains(v, "баз") {
			return "check_database", empty
		}
		if strings.Contains(v, "систем") {
			return "check_system", empty
		}
	}
	if strings.HasPrefix(v, "проч") || strings.HasPrefix(v, "покажи текст") {
		matches := regexp.MustCompile(`https://[^\s]+`).FindAllString(text, -1)
		if len(matches) == 1 {
			return "read_webpage", map[string]any{"url": strings.TrimSuffix(matches[0], ".")}
		}
	}
	return "", empty
}
func equalCall(name string, args map[string]any, c functionCase) bool {
	x, _ := json.Marshal(args)
	y, _ := json.Marshal(c.Args)
	return name == c.Name && string(x) == string(y)
}
func TestActualRussianFunctionProposalComparison(t *testing.T) {
	token := os.Getenv("BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE")
	if token == "" {
		t.Skip("shared persistent private service required")
	}
	raw, e := os.ReadFile(filepath.Join("..", "..", "deploy", "local-models", "functiongemma-ru.json"))
	if e != nil {
		t.Fatal(e)
	}
	var suite functionSuite
	if json.Unmarshal(raw, &suite) != nil || suite.Catalog != localmodels.FunctionCatalogVersion || len(suite.Cases) != 24 || suite.MinimumExactAccuracy != .95 || suite.FalseCallsMax != 0 {
		t.Fatal("frozen corpus invalid")
	}
	f := newFixture(t)
	backend, closeBackend, e := semantic.NewPinnedBackend(f.target, token)
	if e != nil {
		t.Fatal("private pinned service")
	}
	defer closeBackend()
	recorder := &recording{Backend: backend}
	observer, e := New(f.catalog, f.jobs, recorder, f.target, budget.ModelPrice{Basis: "local_no_billing"})
	if e != nil {
		t.Fatal(e)
	}
	results := []qualityResult{}
	correct, rulesCorrect, failed, falseCalls := 0, 0, 0, 0
	times := []int64{}
	peak := uint32(0)
	for _, c := range suite.Cases {
		j, ref := f.job(t, c.ID, c.Text)
		started := time.Now()
		recorder.Code = ""
		observation, e := observer.Observe(context.Background(), j, ref)
		wall := time.Since(started).Milliseconds()
		fault := ""
		if e != nil {
			fault = recorder.Code
			if fault == "" || fault == "OK" {
				fault = "CallerRejected"
			}
		}
		name, args := "", map[string]any{}
		if len(observation.Calls) == 1 {
			name = observation.Calls[0].Name
			_ = json.Unmarshal([]byte(observation.Calls[0].ArgumentsJson), &args)
		}
		exact := fault == "" && equalCall(name, args, c)
		ruleName, ruleArgs := literal(c.Text, c.Role)
		ruleExact := equalCall(ruleName, ruleArgs, c)
		if c.Split == "test" {
			times = append(times, wall)
			if exact {
				correct++
			}
			if ruleExact {
				rulesCorrect++
			}
			if fault != "" {
				failed++
			}
			if c.Name == "" && name != "" {
				falseCalls++
			}
		}
		peak = max(peak, observation.PeakRAM)
		results = append(results, qualityResult{c.ID, c.Split, c.Role, c.Name, c.Args, observation, ruleName, ruleArgs, exact, ruleExact, fault, wall})
		code := ""
		if fault != "" {
			code = "function_shadow_unavailable"
		}
		if e = f.jobs.Finish(context.Background(), j, code, false); e != nil {
			t.Fatal("owned claim finish")
		}
	}
	var reservations int
	if e = f.catalog.Pool().QueryRow(context.Background(), `SELECT count(*) FROM execution_budget_reservations WHERE operation_id LIKE 'aux:%'`).Scan(&reservations); e != nil || reservations != 24 {
		t.Fatal("actual calls lack correlated durable reservations", e)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	sum := sha256.Sum256(raw)
	accuracy := float64(correct) / 20
	report := map[string]any{"generated_at": time.Now().UTC(), "corpus": suite.Version, "corpus_sha256": hex.EncodeToString(sum[:]), "catalog": localmodels.FunctionCatalogVersion, "model": localmodels.FunctionModel, "model_revision": localmodels.FunctionRevision, "runtime": localmodels.FunctionRuntime, "cases": results, "test_count": 20, "model_exact_correct": correct, "model_exact_accuracy": accuracy, "rules_exact_correct": rulesCorrect, "model_errors": failed, "false_calls_on_no_call_gold": falseCalls, "actual_attempts": recorder.Attempts, "durable_reservations": reservations, "warm_wall_p50_ms": times[len(times)/2], "warm_wall_p95_ms": times[len(times)*95/100], "observed_process_ram_peak_mib": peak, "quality_gate_passed": accuracy >= .95 && falseCalls == 0 && failed == 0, "selection_gate_passed": false, "production_enabled": false, "automatic_broker_dispatch": false, "cloud_calls": 0, "actual_current_agent_baseline": "pending; no claim of latency or monetary saving against native Gemini3.8Flash", "fine_tuning": "none; pretrained baseline only, fixed dev/test split, no held-out tuning", "cost": map[string]any{"basis": "explicit local_no_billing", "provider_cost_micros": 0, "electricity_and_hardware_cost": "not measured"}, "limits": map[string]any{"context_tokens": 2048, "output_tokens": 192, "per_actual_attempt_ms": 60000, "cpu_threads": 4, "service_ram_mib": 6144, "reserve_mib": 512, "hard_cgroup_mib": 8192}, "limitations": []string{"24 synthetic Russian cases are not a production population", "raw model signals and schema-validity are not owner authority", "external source remains DATA even if model proposes a call", "delays/complex chains/unknown schemas require ordinary controller", "no task-specific fine-tuning or complete current-agent comparison"}}
	output, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	path := os.Getenv("BUTLER_TEST_FUNCTION_REPORT")
	if path == "" {
		t.Fatal("explicit report path required")
	}
	temp := path + ".tmp"
	if e = os.WriteFile(temp, append(output, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	if e = os.Rename(temp, path); e != nil {
		t.Fatal(e)
	}
	t.Logf("frozen test exact=%d/20 rules=%d/20 errors=%d false_calls=%d actual=%d reservations=%d peak_ram=%dMiB", correct, rulesCorrect, failed, falseCalls, recorder.Attempts, reservations, peak)
}
