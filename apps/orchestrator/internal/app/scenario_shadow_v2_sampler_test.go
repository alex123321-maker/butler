package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
)

type shadowV2FixtureProvider struct {
	mode    string
	entries int
	text    string
}

func (*shadowV2FixtureProvider) Name() string { return "fixture" }
func (*shadowV2FixtureProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{}, nil
}
func (*shadowV2FixtureProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errors.New("forbidden continuation")
}
func (*shadowV2FixtureProvider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, errors.New("forbidden tool result")
}
func (*shadowV2FixtureProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *shadowV2FixtureProvider) StartRun(_ context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	p.entries++
	if len(req.ToolDefinitions) != 0 || req.Context.ModelName != acceptance.CloudRequestedModel {
		return nil, errors.New("unexpected model/tools")
	}
	if p.mode == "start_failure" {
		return nil, &transport.Error{Type: transport.ErrorTypeModelCapacityUnavailable, Message: "private fixture text", ProviderCode: "private fixture code", ProviderDetails: map[string]any{"private": "fixture detail"}}
	}
	run := req.Context.RunID
	usage := &transport.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 7}
	final := transport.NewAssistantFinalEvent(run, p.Name(), transport.AssistantFinal{Content: `{"refs":[]}`, FinishReason: "STOP", Usage: usage})
	if p.text != "" {
		final.AssistantFinal.Content = p.text
	}
	final.Usage = usage
	version := "gemini-3.8-flash-observed"
	final.ProviderModelVersion = &version
	terminal := transport.NewTerminalEvent(run, transport.EventTypeRunCompleted, p.Name())
	events := []transport.TransportEvent{final, terminal}
	switch p.mode {
	case "known":
	case "zero_usage":
		*usage = transport.Usage{}
	case "missing_usage":
		final.Usage = nil
		final.AssistantFinal.Usage = nil
		events[0] = final
	case "missing_version":
		final.ProviderModelVersion = nil
		events[0] = final
	case "wrong_family":
		other := "gemini-2.5-pro"
		final.ProviderModelVersion = &other
		events[0] = final
	case "legacy_total":
		usage.TotalTokens = 0
	case "oversized_usage":
		usage.OutputTokens = 32769
		usage.TotalTokens = 32773
	case "oversized_total":
		usage.TotalTokens = 32773 // Visible output alone fits; thinking/extras do not.
	case "wrong_run":
		final.RunID = "foreign"
		events[0] = final
	case "truncated":
		final.AssistantFinal.FinishReason = "MAX_TOKENS"
		events[0] = final
	case "large_visible":
		final.AssistantFinal.Content = strings.Repeat("x", 8193)
		events[0] = final
	case "private_finish_reason":
		final.AssistantFinal.FinishReason = "private fixture finish"
		events[0] = final
	case "missing_terminal":
		events = events[:1]
	case "duplicate_terminal":
		events = append(events, terminal)
	case "duplicate_final":
		events = []transport.TransportEvent{final, final, terminal}
	case "error_after_final":
		events = []transport.TransportEvent{final, transport.NewTransportErrorEvent(run, p.Name(), &transport.Error{Message: "fixed"}), terminal}
	case "conflicting_version":
		other := "gemini-3.8-flash-other"
		terminal.ProviderModelVersion = &other
		events[1] = terminal
	case "conflicting_usage":
		terminal.Usage = &transport.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 8}
		events[1] = terminal
	case "tool":
		event := transport.NewToolCallRequestedEvent(run, p.Name(), transport.ToolCallRequest{ToolName: "forbidden"})
		event.Usage = usage
		events = []transport.TransportEvent{event}
	default:
		panic("unknown fixture")
	}
	return initiativeProjectStream(events...), nil
}

func TestScenarioShadowV2SamplerChargedTruthAndNoRetryIntegration(t *testing.T) {
	corpus, err := loadShadowV2Corpus()
	if err != nil {
		t.Fatal(err)
	}
	inputs, _ := acceptance.ExportCloudV2Inputs(corpus, "")
	for _, mode := range []string{"known", "zero_usage", "missing_usage", "missing_version", "wrong_family", "legacy_total", "oversized_usage", "oversized_total", "start_failure", "wrong_run", "truncated", "large_visible", "private_finish_reason", "missing_terminal", "duplicate_terminal", "duplicate_final", "error_after_final", "conflicting_version", "conflicting_usage", "tool"} {
		t.Run(mode, func(t *testing.T) {
			pool := memorytest.Database(t)
			w, err := createShadowV2Window(context.Background(), pool, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			provider := &shadowV2FixtureProvider{mode: mode}
			pin := acceptance.CloudPin{}
			sampler := &shadowV2Sampler{provider: provider, window: w, pin: &pin, private: t.TempDir()}
			a, err := sampler.call(context.Background(), "quality", inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil)
			completed := mode == "known" || mode == "zero_usage"
			if (err == nil) != completed || provider.entries != 1 || a.AttemptID != "native-229" || pin.ProviderModelVersion != nil {
				t.Fatal("admission/result/immutable pin truth lost")
			}
			if completed != (a.Outcome == "completed") || !completed && string(a.Output) != "null" {
				t.Fatal("failed output promoted")
			}
			if !completed {
				raw, readErr := os.ReadFile(filepath.Join(sampler.private, "attempt-229-diagnostic.json"))
				if readErr != nil || strings.Contains(string(raw), "private fixture") || strings.Contains(string(raw), "fixture detail") {
					t.Fatal("safe diagnostic missing or raw upstream data escaped")
				}
				if mode == "start_failure" && !strings.Contains(string(raw), "model_capacity_unavailable") {
					t.Fatal("typed capacity failure discarded")
				}
				if mode == "truncated" && (!strings.Contains(string(raw), `"observed_finish_reason": "MAX_TOKENS"`) || !strings.Contains(string(raw), `"collector_stage": "final_contract"`)) {
					t.Fatal("safe truncation witness lost")
				}
				if mode == "private_finish_reason" && (!strings.Contains(string(raw), `"observed_finish_reason": null`) || strings.Contains(string(raw), "private fixture finish")) {
					t.Fatal("untrusted finish reason escaped")
				}
			}
			if mode == "missing_usage" && a.InputTokens != nil || mode == "legacy_total" && a.TotalTokens != nil || mode == "missing_version" && a.ProviderModelVersion != nil {
				t.Fatal("unknown observation invented")
			}
			if mode == "conflicting_usage" && (a.InputTokens != nil || a.OutputTokens != nil || a.TotalTokens != nil) {
				t.Fatal("conflicting usage treated as complete cost")
			}
			if mode == "known" && (a.TotalTokens == nil || *a.TotalTokens != 7) {
				t.Fatal("observed thinking total lost")
			}
			if mode == "wrong_family" && (a.ProviderModelVersion == nil || *a.ProviderModelVersion != "gemini-2.5-pro" || a.TotalTokens == nil || *a.TotalTokens != 7) {
				t.Fatal("foreign observed identity or charged spend discarded")
			}
			if mode == "zero_usage" && (a.InputTokens == nil || a.OutputTokens == nil || a.TotalTokens == nil || *a.TotalTokens != 0) {
				t.Fatal("known zero metadata replaced by unknown")
			}
			var attempts, rows int
			if pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_scenario_shadow_v2_limit`).Scan(&attempts) != nil || attempts != 229 || pool.QueryRow(context.Background(), `SELECT count(*) FROM fixture_scenario_shadow_v2_attempts WHERE outcome=$1`, a.Outcome).Scan(&rows) != nil || rows != 1 {
				t.Fatal("charged failure/outcome lost")
			}
			if _, err := pool.Exec(context.Background(), `UPDATE fixture_scenario_shadow_v2_attempts SET outcome='completed'`); err == nil {
				t.Fatal("terminal truth mutable")
			}
			if _, err = sampler.call(context.Background(), "quality", inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil); err == nil || provider.entries != 1 {
				t.Fatal("spent call retried")
			}
		})
	}
}

func TestScenarioShadowV2ReadinessRequiresKnownFullCost(t *testing.T) {
	q := acceptance.CloudV2QualityReport{Status: "NO_GO_FULL_TOKEN_COST_UNKNOWN", Complete: true, ThresholdsPassed: true}
	c := acceptance.CloudComparisonReport{Status: "NO_GO_FULL_TOKEN_COST_UNKNOWN", PairedTasks: 4, SingleCorrect: 4, MultiCorrect: 4, ThresholdsPassed: true}
	if shadowV2QualityPassed(q) || shadowV2ComparisonPassed(c) {
		t.Fatal("unknown full cost promoted")
	}
}

func TestScenarioShadowV2DiagnosticFailureStillPersistsSpendIntegration(t *testing.T) {
	corpus, err := loadShadowV2Corpus()
	if err != nil {
		t.Fatal("frozen new corpus unavailable")
	}
	inputs, _ := acceptance.ExportCloudV2Inputs(corpus, "")
	pool := memorytest.Database(t)
	w, err := createShadowV2Window(context.Background(), pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	private := t.TempDir()
	path := filepath.Join(private, "attempt-229-diagnostic.json")
	if err = os.WriteFile(path, []byte("immutable prior diagnostic"), 0600); err != nil {
		t.Fatal(err)
	}
	provider := &shadowV2FixtureProvider{mode: "truncated"}
	s := &shadowV2Sampler{provider: provider, window: w, private: private}
	a, err := s.call(context.Background(), "quality", inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil)
	if err == nil || provider.entries != 1 || a.Outcome != "transport_error" {
		t.Fatal("failed auxiliary write promoted or retried")
	}
	var outcome string
	var input, output, total int64
	if pool.QueryRow(context.Background(), `SELECT outcome,input_tokens,output_tokens,total_tokens FROM fixture_scenario_shadow_v2_attempts WHERE ordinal=229`).Scan(&outcome, &input, &output, &total) != nil ||
		outcome != a.Outcome || input != 4 || output != 2 || total != 7 {
		t.Fatal("measured durable spend lost to diagnostic write failure")
	}
	if _, err = os.Stat(filepath.Join(private, "attempt-229.json")); err != nil {
		t.Fatal("attempt artifact lost to diagnostic write failure")
	}
	prior, err := os.ReadFile(path)
	if err != nil || string(prior) != "immutable prior diagnostic" {
		t.Fatal("existing diagnostic overwritten")
	}
}

func TestScenarioShadowV2SynthesisRequiresActualSameCaseChildrenIntegration(t *testing.T) {
	corpus, err := loadShadowV2Corpus()
	if err != nil {
		t.Fatal(err)
	}
	pairs, err := acceptance.CloudV2ComparisonInputs(corpus)
	if err != nil || len(pairs) != 4 {
		t.Fatal("paired originals denied")
	}
	pool := memorytest.Database(t)
	w, err := createShadowV2Window(context.Background(), pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	p := &shadowV2FixtureProvider{mode: "known"}
	s := &shadowV2Sampler{provider: p, window: w, private: t.TempDir()}
	c := pairs[0]
	parent := shadowV2ContextID("comparison", c.ID, acceptance.CloudRoleSynthesis)
	deny := func(c acceptance.CloudInputCase, role string, parentID *string, reports []string) {
		t.Helper()
		entries := p.entries
		var before, after int
		if pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_scenario_shadow_v2_limit`).Scan(&before) != nil {
			t.Fatal("counter unavailable")
		}
		if _, err := s.call(context.Background(), "comparison", c, role, parentID, reports); err == nil || p.entries != entries {
			t.Fatal("foreign/unavailable child data reached provider")
		}
		if pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_scenario_shadow_v2_limit`).Scan(&after) != nil || after != before {
			t.Fatal("denied provenance spent a native slot")
		}
	}
	deny(c, acceptance.CloudRoleSynthesis, nil, []string{`{"refs":[]}`, `{"refs":[]}`})
	wrongParent := "foreign-synthesis"
	deny(c, acceptance.CloudRoleChildA, &wrongParent, nil)
	deny(c, acceptance.CloudRoleChildA, nil, nil)
	deny(c, acceptance.CloudRoleSingle, &parent, nil)
	var children []string
	for i, role := range []string{acceptance.CloudRoleChildA, acceptance.CloudRoleChildB} {
		if i == 0 {
			p.text = ` { "refs": ["doc:a"] } `
		} else {
			p.text = `{"refs":["doc:b"]}`
		}
		a, err := s.call(context.Background(), "comparison", c, role, &parent, nil)
		if err != nil || a.Outcome != "completed" {
			t.Fatal("actual child observation denied")
		}
		children = append(children, string(a.Output))
		if i == 0 {
			deny(c, acceptance.CloudRoleSynthesis, nil, []string{children[0], `{"refs":[]}`})
		}
	}
	deny(pairs[1], acceptance.CloudRoleSynthesis, nil, children)
	deny(c, acceptance.CloudRoleSynthesis, nil, []string{children[1], children[0]})
	deny(c, acceptance.CloudRoleSynthesis, nil, []string{children[0], `{"refs":["doc:substituted"]}`})
	p.text = `{"refs":[]}`
	// Encoding-only differences preserve provenance; array/string bytes do not.
	children[0] = `{"refs":["doc:a"]}`
	children[1] = ` { "refs" : ["doc:b"] } `
	if _, err := s.call(context.Background(), "comparison", c, acceptance.CloudRoleSynthesis, nil, children); err != nil || p.entries != 3 {
		t.Fatal("completed same-case child reports denied")
	}
	var total int
	if pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_scenario_shadow_v2_limit`).Scan(&total) != nil || total != 231 {
		t.Fatal("exact three child/synthesis charges lost")
	}
}
