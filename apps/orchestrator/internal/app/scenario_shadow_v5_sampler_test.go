package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

type shadowV5FixtureProvider struct {
	mode           string
	entries        int
	text           string
	deadline       *time.Time
	legacyConflict bool
	batch          bool
}

func (*shadowV5FixtureProvider) Name() string { return "fixture" }
func (*shadowV5FixtureProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{}, nil
}
func (*shadowV5FixtureProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errors.New("forbidden continuation")
}
func (*shadowV5FixtureProvider) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, errors.New("forbidden tool result")
}
func (*shadowV5FixtureProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *shadowV5FixtureProvider) StartRun(ctx context.Context, req transport.StartRunRequest) (transport.EventStream, error) {
	p.entries++
	if p.deadline != nil {
		value, ok := ctx.Deadline()
		if !ok {
			return nil, errors.New("entry deadline missing")
		}
		*p.deadline = value
	}
	if len(req.ToolDefinitions) != 0 || req.Context.ModelName != acceptance.CloudRequestedModel {
		return nil, errors.New("unexpected model/tools")
	}
	if p.mode == "start_failure" {
		return nil, &transport.Error{Type: transport.ErrorTypeModelCapacityUnavailable, Message: "private fixture text", ProviderCode: "private fixture code", ProviderDetails: map[string]any{"private": "fixture detail"}}
	}
	if p.mode == "stage_authentication_failure" || p.mode == "stage_http_failure" {
		stage := transport.ProviderStageAuthentication
		if p.mode == "stage_http_failure" {
			stage = transport.ProviderStageHTTP
		}
		transport.ObserveProviderStage(ctx, stage)
		return nil, &transport.Error{Type: transport.ErrorTypeProviderTimeout, Message: "private fixture error", ProviderCode: "private fixture code"}
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
	case "stage_stream_failure":
		transport.ObserveProviderStage(ctx, transport.ProviderStage("private fixture stage"))
		transport.ObserveProviderStage(ctx, transport.ProviderStageStream)
		event := transport.NewTransportErrorEvent(run, p.Name(), &transport.Error{Type: transport.ErrorTypeProviderProtocolError, Message: "private fixture stream error"})
		event.Usage, event.ProviderModelVersion = usage, &version
		events = []transport.TransportEvent{event}
	case "known":
	case "cancelled_terminal", "timed_out_terminal", "cancelled_after_final", "timed_out_after_final":
		kind := transport.EventTypeRunCancelled
		if strings.HasPrefix(p.mode, "timed_out") {
			kind = transport.EventTypeRunTimedOut
		}
		event := transport.NewTerminalEvent(run, kind, p.Name())
		event.Usage, event.ProviderModelVersion = usage, &version
		events = []transport.TransportEvent{event}
		if strings.HasSuffix(p.mode, "after_final") {
			events = []transport.TransportEvent{final, event}
		}
	case "error_terminal_only":
		event := transport.NewTransportErrorEvent(run, p.Name(), &transport.Error{Type: transport.ErrorTypeProviderProtocolError, Message: "private fixture error"})
		event.Usage, event.ProviderModelVersion = usage, &version
		events = []transport.TransportEvent{event}
	case "zero_usage":
		*usage = transport.Usage{}
	case "missing_usage":
		final.Usage = nil
		final.AssistantFinal.Usage = nil
		events[0] = final
	case "missing_version":
		final.ProviderModelVersion = nil
		events[0] = final
	case "provisional_version":
		final.ProviderModelVersion = nil
		delta := transport.NewAssistantDeltaEvent(run, p.Name(), transport.AssistantDelta{})
		delta.ProviderModelVersion = &version
		events = []transport.TransportEvent{delta, final, terminal}
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
	case "malformed_raw_json":
		if p.text == "" {
			final.AssistantFinal.Content = strings.Repeat("x", 5000)
		}
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
		// Returning to the first value cannot resolve the contradiction.
		repeated := transport.NewAssistantDeltaEvent(run, p.Name(), transport.AssistantDelta{})
		repeated.ProviderModelVersion = &version
		events = append(events, repeated)
	case "conflicting_usage":
		terminal.Usage = &transport.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 8}
		if p.legacyConflict {
			final.Usage = terminal.Usage
			events[0] = final
		}
		events[1] = terminal
	case "tool":
		event := transport.NewToolCallRequestedEvent(run, p.Name(), transport.ToolCallRequest{ToolName: "forbidden"})
		if p.batch {
			event = transport.NewToolCallBatchRequestedEvent(run, p.Name(), transport.ToolCallBatch{ToolCalls: []transport.ToolCallRequest{{ToolName: "forbidden_a"}, {ToolName: "forbidden_b"}}})
		}
		event.Usage = usage
		event.ProviderModelVersion = &version
		events = []transport.TransportEvent{event}
	default:
		panic("unknown fixture")
	}
	return initiativeProjectStream(events...), nil
}

func TestScenarioShadowV5SamplerCriticalContractStopsNextEntryIntegration(t *testing.T) {
	corpus, err := loadShadowV5Corpus()
	if err != nil {
		t.Fatal(err)
	}
	selection, err := prepareShadowV5Selection(corpus)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, kind, text, category string }{
		{"extra_field", "retrieval", `{"refs":[],"extra":1}`, "critical_schema_contract"},
		{"duplicate", "retrieval", `{"refs":[],"refs":[]}`, ""},
		{"source", "retrieval", `{"refs":["not_provided"]}`, "critical_source_contract"},
		{"authority", "commands", `{"intent":"clarify","authority":"owner_required"}`, "critical_authority_contract"},
		{"tool", "retrieval", "", "critical_tool_contract"},
		{"nonliteral_quote", "extraction", `{"facts":[{"key":"field","value":"invented","quote":"invented","source_id":"not_provided"}]}`, "critical_source_contract"},
		{"array_cap", "extraction", "", "critical_schema_contract"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := memorytest.Database(t)
			w, err := createShadowV5WindowSelected(context.Background(), pool, time.Minute, selection)
			if err != nil {
				t.Fatal(err)
			}
			var input acceptance.CloudInputCase
			for _, c := range selection.inputs.Cases {
				if c.Kind == tc.kind {
					input = c
					break
				}
			}
			p := &shadowV5FixtureProvider{mode: "known", text: tc.text}
			if tc.name == "nonliteral_quote" || tc.name == "array_cap" {
				var id, body string
				for key, text := range input.Documents {
					id, body = key, text
					break
				}
				quote := "UNSUPPORTED_ORIGINAL_VALUE"
				count := 1
				if tc.name == "array_cap" {
					count = 33
					quote = string([]rune(body)[:1])
				}
				facts := []map[string]string{}
				for i := 0; i < count; i++ {
					facts = append(facts, map[string]string{"key": "field", "value": quote, "quote": quote, "source_id": id})
				}
				raw, _ := json.Marshal(map[string]any{"facts": facts})
				p.text = string(raw)
			}
			if tc.name == "tool" {
				p.mode = "tool"
			}
			private := t.TempDir()
			s := &shadowV5Sampler{provider: p, window: w, private: private}
			a, err := s.call(context.Background(), "quality", input, acceptance.CloudRoleSingle, nil, nil)
			if err == nil || p.entries != 1 || a.Outcome == "completed" || a.InputTokens == nil || a.TotalTokens == nil || *a.TotalTokens != 7 {
				t.Fatal("critical response lost charge or was promoted", err)
			}
			var code, stage, outcome string
			if pool.QueryRow(context.Background(), `SELECT d.diagnostic_code,d.collector_stage,a.outcome FROM fixture_scenario_shadow_v5_attempts a JOIN fixture_scenario_shadow_v5_diagnostics d USING(ordinal) WHERE ordinal=284`).Scan(&code, &stage, &outcome) != nil || outcome != a.Outcome || tc.category != "" && (code != string(transport.ErrorTypeProviderProtocolError) || stage != tc.category) {
				t.Fatal("exact critical category missing")
			}
			if p.text != "" {
				raw, err := os.ReadFile(filepath.Join(private, "final-284.raw"))
				if err != nil || string(raw) != p.text {
					t.Fatal("critical original bytes changed")
				}
			}
			if _, err = s.call(context.Background(), "quality", selection.inputs.Cases[1], acceptance.CloudRoleSingle, nil, nil); err == nil || p.entries != 1 {
				t.Fatal("critical failure admitted next case")
			}
		})
	}
}

func TestScenarioShadowV5NativeSchemasAndActualHistoryWithoutCredentials(t *testing.T) {
	h, raw, err := readShadowV5Prerequisites()
	if err != nil || h.NativeAttempts != 283 || len(h.Artifacts) != 21 || h.Artifacts[20].ReportSHA256 != highV5CompatibilityAuditSHA || len(raw) == 0 || checkShadowV5NativeSchemas() != nil {
		t.Fatal("frozen prospective prerequisites denied before auth", err)
	}
}

func TestScenarioShadowV5SamplerDurableDeadlineAndParentCapIntegration(t *testing.T) {
	corpus, err := loadShadowV5Corpus()
	if err != nil {
		t.Fatal(err)
	}
	selection, err := prepareShadowV5Selection(corpus)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"window", "parent"} {
		t.Run(name, func(t *testing.T) {
			pool := memorytest.Database(t)
			w, err := createShadowV5WindowSelected(context.Background(), pool, 30*time.Second, selection)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			var parentDeadline time.Time
			if name == "parent" {
				parentDeadline = time.Now().Add(10 * time.Second)
				child, stop := context.WithDeadline(ctx, parentDeadline)
				defer stop()
				ctx = child
			}
			observed := time.Time{}
			p := &shadowV5FixtureProvider{mode: "known", deadline: &observed}
			s := &shadowV5Sampler{provider: p, window: w, private: t.TempDir()}
			if _, err = s.call(ctx, "quality", selection.inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil); err != nil || p.entries != 1 {
				t.Fatal("known entry denied", err)
			}
			var expiry, created time.Time
			if pool.QueryRow(context.Background(), `SELECT a.created_at,w.expires_at FROM fixture_scenario_shadow_v5_attempts a CROSS JOIN fixture_scenario_shadow_v5_limit w WHERE a.ordinal=284`).Scan(&created, &expiry) != nil || observed.After(created.Add(120*time.Second)) || observed.After(expiry) || name == "parent" && !observed.Equal(parentDeadline) || name == "window" && !observed.Equal(expiry) {
				t.Fatal("persisted deadline or parent cap ignored")
			}
		})
	}
}

func TestScenarioShadowV5SamplerChargedTruthAndNoRetryIntegration(t *testing.T) {
	corpus, err := loadShadowV5Corpus()
	if err != nil {
		t.Fatal(err)
	}
	selection, err := prepareShadowV5Selection(corpus)
	if err != nil {
		t.Fatal(err)
	}
	inputs := selection.inputs
	// Auxiliary ambiguity variants use fresh disposable offline databases. A
	// failed scope itself is never resumed to test a different response shape.
	fresh := func(t *testing.T, p *shadowV5FixtureProvider) (*shadowV5Sampler, *pgxpool.Pool) {
		pool := memorytest.Database(t)
		w, err := createShadowV5WindowSelected(context.Background(), pool, time.Minute, selection)
		if err != nil {
			t.Fatal(err)
		}
		return &shadowV5Sampler{provider: p, window: w, private: t.TempDir()}, pool
	}
	for _, mode := range []string{"known", "zero_usage", "missing_usage", "missing_version", "provisional_version", "wrong_family", "legacy_total", "oversized_usage", "oversized_total", "start_failure", "wrong_run", "truncated", "large_visible", "malformed_raw_json", "private_finish_reason", "missing_terminal", "duplicate_terminal", "duplicate_final", "error_after_final", "conflicting_version", "conflicting_usage", "tool", "stage_authentication_failure", "stage_http_failure", "stage_stream_failure", "cancelled_terminal", "timed_out_terminal", "cancelled_after_final", "timed_out_after_final", "error_terminal_only"} {
		t.Run(mode, func(t *testing.T) {
			pool := memorytest.Database(t)
			w, err := createShadowV5WindowSelected(context.Background(), pool, time.Minute, selection)
			if err != nil {
				t.Fatal(err)
			}
			provider := &shadowV5FixtureProvider{mode: mode}
			pin := acceptance.CloudPin{}
			sampler := &shadowV5Sampler{provider: provider, window: w, pin: &pin, private: t.TempDir()}
			if mode == "known" {
				sampler.providerFactory = func(schema json.RawMessage) (transport.ModelProvider, error) {
					var parsed struct {
						Properties struct {
							Refs struct{ Items struct{ Enum []string } }
						}
					}
					if inputs.Cases[0].Kind != "retrieval" || json.Unmarshal(schema, &parsed) != nil {
						t.Fatal("expected retrieval schema unavailable at trusted factory")
					}
					ids := []string{}
					for id := range inputs.Cases[0].Documents {
						ids = append(ids, id)
					}
					sort.Strings(ids)
					if !reflect.DeepEqual(ids, parsed.Properties.Refs.Items.Enum) {
						t.Fatal("factory narrowed original namespace to a golden subset")
					}
					return provider, nil
				}
			}
			a, err := sampler.call(context.Background(), "quality", inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil)
			completed := mode == "known" || mode == "zero_usage"
			if (err == nil) != completed || provider.entries != 1 || a.AttemptID != "native-284" || pin.ProviderModelVersion != nil {
				t.Fatal("admission/result/immutable pin truth lost")
			}
			if completed != (a.Outcome == "completed") || !completed && string(a.Output) != "null" {
				t.Fatal("failed output promoted")
			}
			if !completed {
				raw, readErr := os.ReadFile(filepath.Join(sampler.private, "attempt-284-diagnostic.json"))
				if readErr != nil || strings.Contains(string(raw), "private fixture") || strings.Contains(string(raw), "fixture detail") {
					t.Fatal("safe diagnostic missing or raw upstream data escaped")
				}
				if mode == "start_failure" && !strings.Contains(string(raw), "model_capacity_unavailable") {
					t.Fatal("typed capacity failure discarded")
				}
				stages := map[string]transport.ProviderStage{"stage_authentication_failure": transport.ProviderStageAuthentication, "stage_http_failure": transport.ProviderStageHTTP, "stage_stream_failure": transport.ProviderStageStream}
				if expected, ok := stages[mode]; ok {
					if !strings.Contains(string(raw), `"provider_operation_stage": "`+string(expected)+`"`) {
						t.Fatal("request-local fixed operation stage lost")
					}
				} else if !strings.Contains(string(raw), `"provider_operation_stage": null`) {
					t.Fatal("unobserved stage invented or leaked across calls")
				}
				if mode == "truncated" && (!strings.Contains(string(raw), `"observed_finish_reason": "MAX_TOKENS"`) || !strings.Contains(string(raw), `"collector_stage": "final_contract"`)) {
					t.Fatal("safe truncation witness lost")
				}
				if mode == "private_finish_reason" && (!strings.Contains(string(raw), `"observed_finish_reason": null`) || strings.Contains(string(raw), "private fixture finish")) {
					t.Fatal("untrusted finish reason escaped")
				}
				if mode == "malformed_raw_json" && !strings.Contains(string(raw), `"collector_stage": "output_encoding_contract"`) {
					t.Fatal("malformed native JSON did not stop the observation driver")
				}
				if mode == "conflicting_version" && !strings.Contains(string(raw), `"collector_stage": "version_contract"`) {
					t.Fatal("identity contradiction lost its safe failure classification")
				}
			}
			if mode == "missing_usage" && a.InputTokens != nil || mode == "legacy_total" && a.TotalTokens != nil || (mode == "missing_version" || mode == "provisional_version") && a.ProviderModelVersion != nil {
				t.Fatal("unknown observation invented")
			}
			if mode == "conflicting_usage" && (a.InputTokens != nil || a.OutputTokens != nil || a.TotalTokens != nil) {
				t.Fatal("conflicting usage treated as complete cost")
			}
			if mode == "conflicting_version" && (a.ProviderModelVersion != nil || sampler.observedVersion == nil || *sampler.observedVersion != "gemini-3.8-flash-observed" || a.TotalTokens == nil || *a.TotalTokens != 7) {
				t.Fatal("conflicting identity became arbitrary known metadata or changed measured spend")
			}
			if mode == "known" && (a.TotalTokens == nil || *a.TotalTokens != 7) {
				t.Fatal("observed thinking total lost")
			}
			if mode == "stage_stream_failure" && (a.TotalTokens == nil || *a.TotalTokens != 7 || a.InputTokens == nil || *a.InputTokens != 4) {
				t.Fatal("known failed packet cost lost to stage diagnostic")
			}
			if mode == "missing_terminal" && (a.TotalTokens == nil || *a.TotalTokens != 7 || a.ProviderModelVersion == nil) {
				t.Fatal("complete parsed packet metadata lost on lifecycle EOF")
			}
			if mode == "wrong_family" && (a.ProviderModelVersion == nil || *a.ProviderModelVersion != "gemini-2.5-pro" || a.TotalTokens == nil || *a.TotalTokens != 7) {
				t.Fatal("foreign observed identity or charged spend discarded")
			}
			if mode == "zero_usage" && (a.InputTokens == nil || a.OutputTokens == nil || a.TotalTokens == nil || *a.TotalTokens != 0) {
				t.Fatal("known zero metadata replaced by unknown")
			}
			var attempts, rows int
			if pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_scenario_shadow_v5_limit`).Scan(&attempts) != nil || attempts != 284 || pool.QueryRow(context.Background(), `SELECT count(*) FROM fixture_scenario_shadow_v5_attempts WHERE outcome=$1`, a.Outcome).Scan(&rows) != nil || rows != 1 {
				t.Fatal("charged failure/outcome lost")
			}
			if strings.HasPrefix(mode, "cancelled") || strings.HasPrefix(mode, "timed_out") || mode == "error_terminal_only" || mode == "error_after_final" || mode == "duplicate_terminal" {
				want := "transport_error"
				if strings.HasPrefix(mode, "cancelled") {
					want = "cancelled"
				}
				if strings.HasPrefix(mode, "timed_out") {
					want = "timeout"
				}
				count := 1
				if mode == "error_after_final" || mode == "duplicate_terminal" {
					count = 2
				}
				var terminalCount int
				if a.Outcome != want || a.TotalTokens == nil || *a.TotalTokens != 7 || pool.QueryRow(context.Background(), `SELECT terminal_count FROM fixture_scenario_shadow_v5_diagnostics WHERE ordinal=284`).Scan(&terminalCount) != nil || terminalCount != count {
					t.Fatal("terminal state/count or observed cost lost")
				}
				if strings.HasSuffix(mode, "after_final") {
					raw, err := os.ReadFile(filepath.Join(sampler.private, "final-284.raw"))
					if err != nil || string(raw) != `{"refs":[]}` {
						t.Fatal("preterminal original final lost")
					}
				}
			}
			if _, err := pool.Exec(context.Background(), `UPDATE fixture_scenario_shadow_v5_attempts SET outcome='completed'`); err == nil {
				t.Fatal("terminal truth mutable")
			}
			if _, err = sampler.call(context.Background(), "quality", inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil); err == nil || provider.entries != 1 {
				t.Fatal("spent call retried")
			}
			if !completed {
				if _, err = sampler.call(context.Background(), "quality", inputs.Cases[1], acceptance.CloudRoleSingle, nil, nil); err == nil || provider.entries != 1 {
					t.Fatal("failed scope admitted a different once-selected case")
				}
			}
			if mode == "conflicting_usage" {
				// The event and legacy final can disagree even when a terminal
				// repeats the event counts. Exercise that distinct ambiguity on
				// another once-admitted case, without changing the counter.
				other := &shadowV5FixtureProvider{mode: mode, legacyConflict: true}
				isolated, _ := fresh(t, other)
				b, failure := isolated.call(context.Background(), "quality", inputs.Cases[1], acceptance.CloudRoleSingle, nil, nil)
				if failure == nil || other.entries != 1 || b.Outcome != "transport_error" || b.InputTokens != nil || b.OutputTokens != nil || b.TotalTokens != nil {
					t.Fatal("legacy/event ambiguity became known complete cost")
				}
			}
			if mode == "tool" {
				other := &shadowV5FixtureProvider{mode: mode, batch: true}
				isolated, _ := fresh(t, other)
				b, failure := isolated.call(context.Background(), "quality", inputs.Cases[1], acceptance.CloudRoleSingle, nil, nil)
				if failure == nil || other.entries != 1 || b.Outcome != "invalid_tool_proposal" || !b.UnsafeToolProposal || string(b.Output) != "null" || b.TotalTokens == nil || *b.TotalTokens != 7 || b.ProviderModelVersion == nil {
					t.Fatal("unsupported batch discarded complete spend or became an action/result")
				}
			}
			if mode == "malformed_raw_json" {
				// These are valid JSON encodings, but none is the structured
				// object required by the native schema. Preserve known spend
				// while rejecting every shape before another driver call.
				for _, text := range []string{`"wrapped text"`, `[]`, `null`} {
					other := &shadowV5FixtureProvider{mode: mode, text: text}
					isolated, isolatedPool := fresh(t, other)
					b, failure := isolated.call(context.Background(), "quality", inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil)
					if failure == nil || other.entries != 1 || b.Outcome != "transport_error" || string(b.Output) != "null" || b.TotalTokens == nil || *b.TotalTokens != 7 {
						t.Fatal("non-object native JSON became a completed observation or lost spend")
					}
					var outcome string
					var total int64
					if isolatedPool.QueryRow(context.Background(), `SELECT outcome,total_tokens FROM fixture_scenario_shadow_v5_attempts WHERE ordinal=$1`, 284).Scan(&outcome, &total) != nil || outcome != "transport_error" || total != 7 {
						t.Fatal("rejected JSON shape did not retain its single durable charge")
					}
					if _, retry := isolated.call(context.Background(), "quality", inputs.Cases[1], acceptance.CloudRoleSingle, nil, nil); retry == nil || other.entries != 1 {
						t.Fatal("rejected JSON shape reopened a spent attempt")
					}
				}
			}
		})
	}
}

func TestScenarioShadowV5ReadinessRequiresKnownFullCost(t *testing.T) {
	q := acceptance.CloudV5QualityReport{Status: "NO_GO_FULL_TOKEN_COST_UNKNOWN", Complete: true, ThresholdsPassed: true}
	c := acceptance.CloudComparisonReport{Status: "NO_GO_FULL_TOKEN_COST_UNKNOWN", PairedTasks: 4, SingleCorrect: 4, MultiCorrect: 4, ThresholdsPassed: true}
	if shadowV5QualityPassed(q) || shadowV5ComparisonPassed(c) {
		t.Fatal("unknown full cost promoted")
	}
}

func TestScenarioShadowV5DiagnosticFailureStillPersistsSpendIntegration(t *testing.T) {
	corpus, err := loadShadowV5Corpus()
	if err != nil {
		t.Fatal("frozen new corpus unavailable")
	}
	inputs, _ := acceptance.ExportCloudV5Inputs(corpus, "")
	pool := memorytest.Database(t)
	w, err := createShadowV5Window(context.Background(), pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	private := t.TempDir()
	path := filepath.Join(private, "attempt-284-diagnostic.json")
	if err = os.WriteFile(path, []byte("immutable prior diagnostic"), 0600); err != nil {
		t.Fatal(err)
	}
	provider := &shadowV5FixtureProvider{mode: "truncated"}
	s := &shadowV5Sampler{provider: provider, window: w, private: private}
	a, err := s.call(context.Background(), "quality", inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil)
	if err == nil || provider.entries != 1 || a.Outcome != "transport_error" {
		t.Fatal("failed auxiliary write promoted or retried")
	}
	var outcome string
	var input, output, total int64
	if pool.QueryRow(context.Background(), `SELECT outcome,input_tokens,output_tokens,total_tokens FROM fixture_scenario_shadow_v5_attempts WHERE ordinal=284`).Scan(&outcome, &input, &output, &total) != nil ||
		outcome != a.Outcome || input != 4 || output != 2 || total != 7 {
		t.Fatal("measured durable spend lost to diagnostic write failure")
	}
	if _, err = os.Stat(filepath.Join(private, "attempt-284.json")); err != nil {
		t.Fatal("attempt artifact lost to diagnostic write failure")
	}
	prior, err := os.ReadFile(path)
	if err != nil || string(prior) != "immutable prior diagnostic" {
		t.Fatal("existing diagnostic overwritten")
	}
}

func TestScenarioShadowV5SynthesisRequiresActualSameCaseChildrenIntegration(t *testing.T) {
	corpus, err := loadShadowV5Corpus()
	if err != nil {
		t.Fatal(err)
	}
	pairs, err := acceptance.CloudV5ComparisonInputs(corpus)
	if err != nil || len(pairs) != 4 {
		t.Fatal("paired originals denied")
	}
	pool := memorytest.Database(t)
	w, err := createShadowV5Window(context.Background(), pool, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	p := &shadowV5FixtureProvider{mode: "known"}
	s := &shadowV5Sampler{provider: p, window: w, private: t.TempDir()}
	c := pairs[0]
	parent := shadowV5ContextID("comparison", c.ID, acceptance.CloudRoleSynthesis)
	deny := func(c acceptance.CloudInputCase, role string, parentID *string, reports []string) {
		t.Helper()
		entries := p.entries
		var before, after int
		if pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_scenario_shadow_v5_limit`).Scan(&before) != nil {
			t.Fatal("counter unavailable")
		}
		if _, err := s.call(context.Background(), "comparison", c, role, parentID, reports); err == nil || p.entries != entries {
			t.Fatal("foreign/unavailable child data reached provider")
		}
		if pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_scenario_shadow_v5_limit`).Scan(&after) != nil || after != before {
			t.Fatal("denied provenance spent a native slot")
		}
	}
	deny(c, acceptance.CloudRoleSynthesis, nil, []string{`{"refs":[]}`, `{"refs":[]}`})
	wrongParent := "foreign-synthesis"
	deny(c, acceptance.CloudRoleChildA, &wrongParent, nil)
	deny(c, acceptance.CloudRoleChildA, nil, nil)
	deny(c, acceptance.CloudRoleSingle, &parent, nil)
	var children []string
	ids := []string{}
	for id := range c.Documents {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for i, role := range []string{acceptance.CloudRoleChildA, acceptance.CloudRoleChildB} {
		if i == 0 {
			p.text = ` { "refs": ["` + ids[0] + `"] } `
		} else {
			p.text = `{"refs":["` + ids[1] + `"]}`
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
	children[0] = `{"refs":["` + ids[0] + `"]}`
	children[1] = ` { "refs" : ["` + ids[1] + `"] } `
	if _, err := s.call(context.Background(), "comparison", c, acceptance.CloudRoleSynthesis, nil, children); err != nil || p.entries != 3 {
		t.Fatal("completed same-case child reports denied")
	}
	var total int
	if pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_scenario_shadow_v5_limit`).Scan(&total) != nil || total != 286 {
		t.Fatal("exact three child/synthesis charges lost")
	}
}
