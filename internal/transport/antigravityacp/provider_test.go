package antigravityacp

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/acp"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func fixtureProvider(test *testing.T, execute runner) *Provider {
	test.Helper()
	provider, err := newProvider(Config{Model: "gemini-fixture", Timeout: time.Second}, execute)
	if err != nil {
		test.Fatal(err)
	}
	test.Cleanup(func() {
		provider.mu.Lock()
		defer provider.mu.Unlock()
		for _, state := range provider.runs {
			if state.expiry != nil {
				state.expiry.Stop()
			}
			if state.cancel != nil {
				state.cancel()
			}
		}
	})
	return provider
}

func fixtureStart() transport.StartRunRequest {
	return transport.StartRunRequest{
		Context:         transport.TransportRunContext{RunID: "run", SessionKey: "session", ProviderName: Name, ModelName: "gemini-fixture"},
		InputItems:      []transport.InputItem{content.Instruction("Butler owns actions", content.Origin{Kind: content.KernelInstruction, SourceRefs: []string{"builtin:fixture"}}), {Role: "user", Content: "Synthetic question"}},
		ToolDefinitions: []transport.ToolDefinition{{Name: "fixture.read", SchemaJSON: `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`}},
	}
}

func result(text string) acp.Result {
	return acp.Result{Text: text, ModelID: "gemini-fixture", StopReason: "end_turn", UsageState: "unknown"}
}

func collect(test *testing.T, events transport.EventStream, err error) []transport.TransportEvent {
	test.Helper()
	if err != nil {
		test.Fatal(err)
	}
	var values []transport.TransportEvent
	deadline := time.After(3 * time.Second)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return values
			}
			values = append(values, event)
		case <-deadline:
			test.Fatal("unclosed transport stream")
			return nil
		}
	}
}

func TestBrokerProposalThenResultAndFinal(test *testing.T) {
	var calls, dispatched atomic.Int32
	provider := fixtureProvider(test, func(ctx context.Context, _ acp.AntigravityProcess, model, prompt string, before func(context.Context) error) (acp.Result, error) {
		if model != "gemini-fixture" || !strings.Contains(prompt, "Butler owns actions") {
			return acp.Result{}, errors.New("lost model or system messages")
		}
		if err := before(ctx); err != nil {
			return acp.Result{}, err
		}
		if calls.Add(1) == 1 {
			return result(`{"type":"tool_call","name":"fixture.read","arguments":{"name":"sample"}}`), nil
		}
		if !strings.Contains(prompt, `\"status\":\"ok\"`) || !strings.Contains(prompt, `"tools":[]`) {
			return acp.Result{}, errors.New("lost result or replaced tools")
		}
		return result(`{"type":"answer","answer":"Synthetic complete"}`), nil
	})
	ctx := transport.WithInferenceDispatch(context.Background(), func(context.Context) error { dispatched.Add(1); return nil })
	events, err := provider.StartRun(ctx, fixtureStart())
	first := collect(test, events, err)
	if len(first) != 2 || first[1].ToolCall == nil || first[1].Usage != nil || first[0].CapabilitiesSnapshot.SupportsStatefulSessions || first[0].CapabilitiesSnapshot.SupportsUsageMetadata {
		test.Fatalf("unexpected proposal events: %+v", first)
	}
	ref := first[0].ProviderSessionRef
	events, err = provider.SubmitToolResult(ctx, transport.SubmitToolResultRequest{RunID: "run", ProviderSessionRef: ref, ToolCallRef: first[1].ToolCall.ToolCallRef, ToolResultJSON: `{"status":"ok"}`, ToolSet: transport.NewToolSetSnapshot(nil)})
	second := collect(test, events, err)
	if len(second) != 3 || second[1].AssistantFinal == nil || second[1].AssistantFinal.Content != "Synthetic complete" || second[2].EventType != transport.EventTypeRunCompleted || dispatched.Load() != 2 {
		test.Fatalf("unexpected final events: %+v dispatches=%d", second, dispatched.Load())
	}
}

func TestRejectsMalformedOrUnadvertisedDecisions(test *testing.T) {
	for _, text := range []string{
		`{"type":"tool_call","name":"fixture.delete","arguments":{}}`,
		`{"type":"tool_call","name":"fixture.read","arguments":{"name":9}}`,
		`{"type":"tool_call","name":"fixture.read","arguments":{}}`,
		`{"type":"tool_call","name":"fixture.read","arguments":{"name":"ok","extra":true}}`,
		`{"type":"answer","answer":"ok","name":"fixture.read"}`,
		`{"type":"answer","type":"answer","answer":"ok"}`,
		`{"type":"answer","answer":"ok","secret":"never returned"}`,
		"```json\n{}\n```", `{"type":"answer","answer":""}`,
	} {
		test.Run(text, func(test *testing.T) {
			provider := fixtureProvider(test, func(context.Context, acp.AntigravityProcess, string, string, func(context.Context) error) (acp.Result, error) {
				return result(text), nil
			})
			events, err := provider.StartRun(context.Background(), fixtureStart())
			values := collect(test, events, err)
			if len(values) != 2 || values[1].TransportError == nil || strings.Contains(values[1].PayloadJSON, "never returned") {
				test.Fatalf("invalid output escaped: %+v", values)
			}
		})
	}
}

func TestAuthorityCheckedBeforeStartupPromptAndRelease(test *testing.T) {
	for _, revokeAt := range []int32{1, 2, 3} {
		test.Run(string(rune('0'+revokeAt)), func(test *testing.T) {
			var checked, started, dispatched atomic.Int32
			provider := fixtureProvider(test, func(ctx context.Context, _ acp.AntigravityProcess, _, _ string, before func(context.Context) error) (acp.Result, error) {
				started.Add(1)
				if err := before(ctx); err != nil {
					return acp.Result{}, err
				}
				return result(`{"type":"answer","answer":"must not escape"}`), nil
			})
			ctx := transport.WithInferenceAuthority(context.Background(), func(context.Context) error {
				if checked.Add(1) >= revokeAt {
					return errors.New("revoked")
				}
				return nil
			})
			ctx = transport.WithInferenceDispatch(ctx, func(context.Context) error { dispatched.Add(1); return nil })
			events, err := provider.StartRun(ctx, fixtureStart())
			values := collect(test, events, err)
			for _, event := range values {
				if event.AssistantFinal != nil || event.ToolCall != nil {
					test.Fatal("revoked authority released a decision")
				}
			}
			if revokeAt == 1 && started.Load() != 0 || revokeAt <= 2 && dispatched.Load() != 0 || revokeAt == 3 && dispatched.Load() != 1 {
				test.Fatalf("started=%d dispatched=%d", started.Load(), dispatched.Load())
			}
		})
	}
}

func TestInputBudgetAndOptionsRefuseBeforeProcess(test *testing.T) {
	var invoked atomic.Int32
	provider := fixtureProvider(test, func(context.Context, acp.AntigravityProcess, string, string, func(context.Context) error) (acp.Result, error) {
		invoked.Add(1)
		return acp.Result{}, nil
	})
	request := fixtureStart()
	request.TransportOptions = map[string]any{"executable": "untrusted"}
	if _, err := provider.StartRun(context.Background(), request); err == nil {
		test.Fatal("model-selected process accepted")
	}
	request = fixtureStart()
	request.Context.ModelName = "different-model"
	if _, err := provider.StartRun(context.Background(), request); err == nil {
		test.Fatal("model fallback accepted")
	}
	ctx := transport.WithInputByteLimit(context.Background(), 2)
	events, err := provider.StartRun(ctx, fixtureStart())
	values := collect(test, events, err)
	if len(values) != 1 || values[0].TransportError == nil || invoked.Load() != 0 {
		test.Fatalf("budget did not prevent process: %+v", values)
	}
}

func TestCancelAndStaleResume(test *testing.T) {
	started := make(chan struct{})
	provider := fixtureProvider(test, func(ctx context.Context, _ acp.AntigravityProcess, _, _ string, before func(context.Context) error) (acp.Result, error) {
		if err := before(ctx); err != nil {
			return acp.Result{}, err
		}
		close(started)
		<-ctx.Done()
		return acp.Result{}, ctx.Err()
	})
	events, err := provider.StartRun(context.Background(), fixtureStart())
	if err != nil {
		test.Fatal(err)
	}
	first := <-events
	<-started
	foreign := *first.ProviderSessionRef
	foreign.SessionRef = "foreign"
	if _, err := provider.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "run", ProviderSessionRef: &foreign}); err == nil {
		test.Fatal("foreign cancel accepted")
	}
	if _, err := provider.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "run", ProviderSessionRef: first.ProviderSessionRef}); err != nil {
		test.Fatal(err)
	}
	collect(test, events, nil)
	if _, err := provider.ContinueRun(context.Background(), transport.ContinueRunRequest{RunID: "run", ProviderSessionRef: first.ProviderSessionRef, InputItems: []transport.InputItem{{Role: "user", Content: "resume"}}}); err == nil {
		test.Fatal("stale context resumed")
	}
}

func TestContinuationPreservesSystemAndReplacesTools(test *testing.T) {
	var count atomic.Int32
	provider := fixtureProvider(test, func(ctx context.Context, _ acp.AntigravityProcess, _, prompt string, before func(context.Context) error) (acp.Result, error) {
		if err := before(ctx); err != nil {
			return acp.Result{}, err
		}
		if count.Add(1) == 2 {
			var envelope struct {
				Messages []transport.InputItem
				Tools    []transport.ToolDefinition
			}
			if json.Unmarshal([]byte(prompt), &envelope) != nil || envelope.Messages[0].Content != "Butler owns actions" || len(envelope.Tools) != 0 {
				return acp.Result{}, errors.New("bad continuation")
			}
		}
		return result(`{"type":"answer","answer":"ok"}`), nil
	})
	events, err := provider.StartRun(context.Background(), fixtureStart())
	values := collect(test, events, err)
	request := transport.ContinueRunRequest{RunID: "run", ProviderSessionRef: values[0].ProviderSessionRef, InputItems: []transport.InputItem{{Role: "system", Content: "replace"}}}
	if _, err := provider.ContinueRun(context.Background(), request); err == nil {
		test.Fatal("system replacement accepted")
	}
	request.InputItems = []transport.InputItem{{Role: "user", Content: "correct once"}}
	request.ToolSet = transport.NewToolSetSnapshot(nil)
	events, err = provider.ContinueRun(context.Background(), request)
	values = collect(test, events, err)
	if len(values) != 3 || values[1].AssistantFinal == nil || count.Load() != 2 {
		test.Fatalf("bad continuation: %+v", values)
	}
}

func TestPublicConstructorRequiresExplicitLinuxOptIn(test *testing.T) {
	profile := filepath.Join(test.TempDir(), "dedicated")
	if err := acp.PrepareAntigravityProfile(profile); err != nil {
		test.Fatal(err)
	}
	config := Config{Model: "gemini-fixture", Timeout: time.Second, Process: acp.AntigravityProcess{
		Executable: filepath.Join(test.TempDir(), "not-launched"), SHA256: strings.Repeat("0", 64), ProfileDir: profile,
	}}
	if _, err := NewProvider(config); err == nil {
		test.Fatal("disabled experimental runtime accepted")
	}
	if runtime.GOOS != "linux" {
		config.Enabled = true
		if _, err := NewProvider(config); err == nil {
			test.Fatal("unsupported runtime accepted")
		}
	}
}

func TestUnqualifiedLimitsAreNotAdvertised(test *testing.T) {
	provider := fixtureProvider(test, func(context.Context, acp.AntigravityProcess, string, string, func(context.Context) error) (acp.Result, error) {
		test.Fatal("qualification check dispatched inference")
		return acp.Result{}, nil
	})
	if _, ok := any(provider).(interface{ InferenceLimits() (int64, int64) }); ok {
		test.Fatal("unverified native token allowance advertised")
	}
}

func TestSecretInputAndDispatchFailureDoNotReleaseDecisions(test *testing.T) {
	var invoked atomic.Int32
	provider := fixtureProvider(test, func(ctx context.Context, _ acp.AntigravityProcess, _, _ string, before func(context.Context) error) (acp.Result, error) {
		invoked.Add(1)
		if err := before(ctx); err != nil {
			return acp.Result{}, err
		}
		return result(`{"type":"answer","answer":"must not escape"}`), nil
	})
	request := fixtureStart()
	request.InputItems = []transport.InputItem{content.Data("private-canary", content.Origin{Kind: content.OwnerInput, Sensitivity: content.Secret, SourceRefs: []string{"fixture:secret"}})}
	if _, err := provider.StartRun(context.Background(), request); err == nil || invoked.Load() != 0 {
		test.Fatal("secret-class input reached runtime")
	}
	ctx := transport.WithInferenceDispatch(context.Background(), func(context.Context) error { return errors.New("private-canary") })
	events, err := provider.StartRun(ctx, fixtureStart())
	values := collect(test, events, err)
	if len(values) != 2 || values[1].TransportError == nil || strings.Contains(values[1].PayloadJSON, "private-canary") {
		test.Fatalf("dispatch failure leaked a result: %+v", values)
	}
}
