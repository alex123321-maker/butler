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

type shadowFixtureProvider struct {
	mode    string
	entries int
}

func (*shadowFixtureProvider) Name() string { return "fixture" }
func (*shadowFixtureProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{}, nil
}
func (*shadowFixtureProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errors.New("forbidden continuation")
}
func (*shadowFixtureProvider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, errors.New("forbidden tool result")
}
func (*shadowFixtureProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *shadowFixtureProvider) StartRun(_ context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
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
	final.Usage = usage
	version := "gemini-3.8-flash-observed"
	final.ProviderModelVersion = &version
	terminal := transport.NewTerminalEvent(run, transport.EventTypeRunCompleted, p.Name())
	events := []transport.TransportEvent{final, terminal}
	switch p.mode {
	case "known":
	case "missing_usage":
		final.Usage = nil
		final.AssistantFinal.Usage = nil
		events[0] = final
	case "missing_version":
		final.ProviderModelVersion = nil
		events[0] = final
	case "legacy_total":
		usage.TotalTokens = 0
	case "oversized_usage":
		usage.OutputTokens = 8193
		usage.TotalTokens = 8197
	case "wrong_run":
		final.RunID = "foreign"
		events[0] = final
	case "truncated":
		final.AssistantFinal.FinishReason = "MAX_TOKENS"
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

func TestScenarioShadowSamplerChargedTruthAndNoRetryIntegration(t *testing.T) {
	corpus, err := loadShadowCorpus()
	if err != nil {
		t.Fatal(err)
	}
	inputs, _ := acceptance.ExportCloudInputs(corpus, "")
	for _, mode := range []string{"known", "missing_usage", "missing_version", "legacy_total", "oversized_usage", "start_failure", "wrong_run", "truncated", "missing_terminal", "duplicate_terminal", "duplicate_final", "error_after_final", "conflicting_version", "conflicting_usage", "tool"} {
		t.Run(mode, func(t *testing.T) {
			pool := memorytest.Database(t)
			w, err := createShadowWindow(context.Background(), pool, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			provider := &shadowFixtureProvider{mode: mode}
			pin := acceptance.CloudPin{}
			sampler := &shadowSampler{provider: provider, window: w, pin: &pin, private: t.TempDir()}
			a, err := sampler.call(context.Background(), "quality", inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil)
			completed := mode == "known" || mode == "missing_usage" || mode == "missing_version" || mode == "legacy_total"
			if (err == nil) != completed || provider.entries != 1 || a.AttemptID != "native-046" || pin.ProviderModelVersion != nil {
				t.Fatal("admission/result/immutable pin truth lost")
			}
			if completed != (a.Outcome == "completed") || !completed && string(a.Output) != "null" {
				t.Fatal("failed output promoted")
			}
			if !completed {
				raw, readErr := os.ReadFile(filepath.Join(sampler.private, "attempt-046-diagnostic.json"))
				if readErr != nil || strings.Contains(string(raw), "private fixture") || strings.Contains(string(raw), "fixture detail") {
					t.Fatal("safe diagnostic missing or raw upstream data escaped")
				}
				if mode == "start_failure" && !strings.Contains(string(raw), "model_capacity_unavailable") {
					t.Fatal("typed capacity failure discarded")
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
			var attempts, rows int
			if pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_scenario_shadow_limit`).Scan(&attempts) != nil || attempts != 46 || pool.QueryRow(context.Background(), `SELECT count(*) FROM fixture_scenario_shadow_attempts WHERE outcome=$1`, a.Outcome).Scan(&rows) != nil || rows != 1 {
				t.Fatal("charged failure/outcome lost")
			}
			if _, err := pool.Exec(context.Background(), `UPDATE fixture_scenario_shadow_attempts SET outcome='completed'`); err == nil {
				t.Fatal("terminal truth mutable")
			}
			if _, err = sampler.call(context.Background(), "quality", inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil); err == nil || provider.entries != 1 {
				t.Fatal("spent call retried")
			}
		})
	}
}

func TestScenarioShadowReadinessRequiresKnownFullCost(t *testing.T) {
	q := acceptance.CloudQualityReport{Status: "NO_GO_FULL_TOKEN_COST_UNKNOWN", Complete: true, ThresholdsPassed: true}
	c := acceptance.CloudComparisonReport{Status: "NO_GO_FULL_TOKEN_COST_UNKNOWN", PairedTasks: 4, SingleCorrect: 4, MultiCorrect: 4, ThresholdsPassed: true}
	if shadowQualityPassed(q) || shadowComparisonPassed(c) {
		t.Fatal("unknown full cost promoted")
	}
}
