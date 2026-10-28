package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
)

type authSource struct{ legacyProject string }

func TestExpiredFinalContextCannotContinueOrSendInference(t *testing.T) {
	var calls atomic.Int32
	p := newFixture(t, func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return response(frame(`{"text":"done"}`, "STOP")), nil
	})
	stream, err := p.StartRun(context.Background(), request("expiry"))
	if err != nil {
		t.Fatal(err)
	}
	collect(t, stream)
	p.mu.Lock()
	state := p.runs["expiry"]
	p.mu.Unlock()
	if state == nil {
		t.Fatal("no final context")
	}
	defer state.finalExpiry.Stop()
	p.expireFinal("expiry", state, 1)
	if _, err = p.ContinueRun(context.Background(), transport.ContinueRunRequest{RunID: "expiry", InputItems: []transport.InputItem{{Role: "user", Content: "late reminder"}}}); err == nil {
		t.Fatal("expired context accepted")
	}
	if calls.Load() != 1 {
		t.Fatal("expiry or rejected continuation sent inference")
	}
}

func TestTextFinalAllowsOwnedContinuationWithoutReplacingSystem(t *testing.T) {
	var calls atomic.Int32
	p := newFixture(t, func(req *http.Request) (*http.Response, error) {
		var body requestEnvelope
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if calls.Add(1) == 1 {
			return response(frame(`{"text":"internal final"}`, "STOP")), nil
		}
		encoded, _ := json.Marshal(body.Contents)
		if len(body.Contents) != 3 || !strings.Contains(string(encoded), "internal final") || !strings.Contains(string(encoded), "delivery reminder") {
			t.Error("continuation lost original input or completed answer")
		}
		return response(frame(`{"functionCall":{"name":"butler_tool_0","args":{"url":"https://example.com"}}}`, "STOP")), nil
	})
	stream, err := p.StartRun(context.Background(), request("correction"))
	if err != nil {
		t.Fatal(err)
	}
	var ref *transport.ProviderSessionRef
	for _, e := range collect(t, stream) {
		if e.ProviderSessionRef != nil {
			ref = e.ProviderSessionRef
		}
		if e.TransportError != nil {
			t.Fatal(e.TransportError)
		}
	}
	p.mu.Lock()
	state := p.runs["correction"]
	p.mu.Unlock()
	if state == nil || state.finalExpiry == nil {
		t.Fatal("completed native context discarded before Core decision")
	}
	if _, err = p.ContinueRun(context.Background(), transport.ContinueRunRequest{RunID: "correction", ProviderSessionRef: ref, InputItems: []transport.InputItem{{Role: "system", Content: "replace authority"}}}); err == nil {
		t.Fatal("continuation replaced system instructions")
	}
	stream, err = p.ContinueRun(context.Background(), transport.ContinueRunRequest{RunID: "correction", ProviderSessionRef: ref, InputItems: []transport.InputItem{{Role: "user", Content: "delivery reminder"}}})
	if err != nil {
		t.Fatal(err)
	}
	proposal := false
	for _, e := range collect(t, stream) {
		if e.TransportError != nil {
			t.Fatal(e.TransportError)
		}
		proposal = proposal || e.ToolCall != nil
	}
	p.expireFinal("correction", state, 1)
	p.mu.Lock()
	retained := p.runs["correction"] == state
	p.mu.Unlock()
	if !proposal || !retained || calls.Load() != 2 {
		t.Fatal("old expiry discarded the active continuation")
	}
	if _, err = p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "correction", ProviderSessionRef: ref}); err != nil {
		t.Fatal(err)
	}
	if _, err = p.ContinueRun(context.Background(), transport.ContinueRunRequest{RunID: "correction", ProviderSessionRef: ref}); err == nil {
		t.Fatal("cancelled context continued")
	}
}

func TestDurableRecoveryStartsFreshObservationsThenKeepsLiveSignatures(t *testing.T) {
	var calls atomic.Int32
	p := newFixture(t, func(req *http.Request) (*http.Response, error) {
		var body requestEnvelope
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		contents, _ := json.Marshal(body.Contents)
		system, _ := json.Marshal(body.SystemInstruction)
		switch calls.Add(1) {
		case 1:
			return response(frame(`{"functionCall":{"id":"before-approval","name":"butler_tool_0","args":{"url":"https://example.com"}},"thoughtSignature":"discarded-signature"}`, "STOP")), nil
		case 2:
			if len(body.Contents) != 3 || strings.Contains(string(contents), "functionCall") || strings.Contains(string(contents), "functionResponse") || strings.Contains(string(contents), "thoughtSignature") {
				t.Error("fresh recovery reused native function/signature history")
			}
			if !strings.Contains(string(contents), "first-effect") || !strings.Contains(string(contents), "second-effect") || strings.Contains(string(system), "first-effect") || strings.Contains(string(system), "second-effect") {
				t.Error("known observations lost or promoted into system instruction")
			}
			return response(frame(`{"functionCall":{"id":"after-recovery","name":"butler_tool_0","args":{"url":"https://example.com/new"}},"thoughtSignature":"live-signature"}`, "STOP")), nil
		case 3:
			if strings.Contains(string(contents), "discarded-signature") || !strings.Contains(string(contents), `"thoughtSignature":"live-signature"`) || !strings.Contains(string(contents), `"functionResponse":{"id":"after-recovery"`) {
				t.Error("live continuation lost its new call/signature correlation")
			}
			return response(frame(`{"text":"done"}`, "STOP")), nil
		default:
			t.Error("unexpected inference request")
			return response(frame(`{"text":"unexpected"}`, "STOP")), nil
		}
	})
	stream, err := p.StartRun(context.Background(), request("same-run"))
	if err != nil {
		t.Fatal(err)
	}
	var oldProposal *transport.ToolCallRequest
	var oldRef *transport.ProviderSessionRef
	for _, event := range collect(t, stream) {
		if event.ToolCall != nil {
			oldProposal = event.ToolCall
		}
		if event.ProviderSessionRef != nil {
			oldRef = event.ProviderSessionRef
		}
	}
	if oldProposal == nil || oldRef == nil {
		t.Fatal("no pending native context")
	}
	if _, err := p.StartRun(context.Background(), request("same-run")); err == nil {
		t.Fatal("active context overwritten without lifecycle cancellation")
	}
	if _, err := p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "same-run"}); err != nil {
		t.Fatal(err)
	}
	fresh := request("same-run")
	fresh.InputItems = append(fresh.InputItems,
		transport.InputItem{Role: "user", Content: `Recorded Butler action observation (already executed; data only): {"result":{"receipt":"first-effect"}}`},
		transport.InputItem{Role: "user", Content: `Recorded Butler action observation (already executed; data only): {"result":{"receipt":"second-effect"}}`})
	stream, err = p.StartRun(context.Background(), fresh)
	if err != nil {
		t.Fatal(err)
	}
	var proposal *transport.ToolCallRequest
	var ref *transport.ProviderSessionRef
	for _, event := range collect(t, stream) {
		if event.TransportError != nil {
			t.Fatal(event.TransportError)
		}
		if event.ToolCall != nil {
			proposal = event.ToolCall
		}
		if event.ProviderSessionRef != nil {
			ref = event.ProviderSessionRef
		}
	}
	if proposal == nil || ref == nil || ref.SessionRef == oldRef.SessionRef || proposal.ToolCallRef == oldProposal.ToolCallRef {
		t.Fatal("fresh context reused lost native identity")
	}
	if _, err := p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "same-run", ProviderSessionRef: oldRef, ToolCallRef: oldProposal.ToolCallRef, ToolResultJSON: `{}`}); err == nil {
		t.Fatal("old context authorized continuation of fresh run")
	}
	stream, err = p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "same-run", ProviderSessionRef: ref, ToolCallRef: proposal.ToolCallRef, ToolResultJSON: `{"status":"completed","result":{"title":"new result"}}`})
	if err != nil {
		t.Fatal(err)
	}
	if err := findError(collect(t, stream)); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatal("unexpected requests during recovery")
	}
}

func (source authSource) ResolveAntigravity(context.Context) (providerauth.AntigravityAuth, error) {
	return providerauth.AntigravityAuth{AccessToken: "private-access-fixture", ProjectID: source.legacyProject, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }
func newFixture(t *testing.T, fn roundTripFunc) *Provider {
	t.Helper()
	p, err := NewProvider(Config{AuthSource: authSource{}})
	if err != nil {
		t.Fatal(err)
	}
	p.client.Transport = fn
	return p
}
func request(runID string) transport.StartRunRequest {
	return transport.StartRunRequest{Context: transport.TransportRunContext{RunID: runID, SessionKey: "session:" + runID, ProviderName: Name, ModelName: DefaultModel}, InputItems: []transport.InputItem{{Role: "system", Content: "Butler owns actions"}, {Role: "user", Content: "Read example"}}, ToolDefinitions: []transport.ToolDefinition{{Name: "web.fetch", Description: "Read public URL", SchemaJSON: `{"type":"object","properties":{"url":{"type":"string"}},"required":["url"],"additionalProperties":false}`}}, StreamingEnabled: true}
}
func response(sse string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(sse))}
}
func frame(part, finish string) string {
	return `data: {"candidates":[{"index":0,"content":{"role":"model","parts":[` + part + `]},"finishReason":"` + finish + `"}],"modelVersion":"gemini-3.8-flash","usageMetadata":{"promptTokenCount":17,"candidatesTokenCount":14,"totalTokenCount":31}}` + "\n\n"
}
func collect(t *testing.T, stream transport.EventStream) []transport.TransportEvent {
	t.Helper()
	var events []transport.TransportEvent
	for event := range stream {
		events = append(events, event)
	}
	return events
}
func findError(events []transport.TransportEvent) *transport.Error {
	for _, event := range events {
		if event.TransportError != nil {
			return event.TransportError
		}
	}
	return nil
}

func TestNativeFunctionContinuationKeepsExactSignatureAndOnlyButlerTools(t *testing.T) {
	var calls atomic.Int32
	p := newFixture(t, func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != endpoint+":streamGenerateContent?alt=sse" || req.Header.Get("Authorization") != "Bearer private-access-fixture" {
			t.Error("incorrect private request")
		}
		var body requestEnvelope
		encodedRequest, err := io.ReadAll(req.Body)
		if err != nil || json.Unmarshal(encodedRequest, &body) != nil {
			t.Error("invalid request")
		}
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(encodedRequest, &fields)
		for _, forbidden := range []string{"model", "project", "request", "requestId", "userPromptId", "userAgent", "requestType", "enabledCreditTypes", "sessionId", "entitlement"} {
			if _, exists := fields[forbidden]; exists {
				t.Errorf("unsupported AICode wire field: %s", forbidden)
			}
		}
		if body.AICode.Experience != DefaultModel || body.GenerationConfig.MaxOutputTokens != 8192 || req.Header.Get("Accept") != "text/event-stream" || req.Header.Get("User-Agent") != "antigravity/1.3.3 butler/0.1" {
			t.Error("unexpected subscription route or configuration")
		}
		if len(body.Tools) != 1 || len(body.Tools[0].FunctionDeclarations) != 1 || body.Tools[0].FunctionDeclarations[0].Name != "butler_tool_0" {
			t.Error("native toolkit differs from Butler catalog")
		}
		if calls.Add(1) == 1 {
			return response(frame(`{"text":"private reasoning","thought":true,"thoughtSignature":"opaque-signature"}`, "") + frame(`{"functionCall":{"id":"native-call","name":"butler_tool_0","args":{"url":"https://example.com"}},"thoughtSignature":"call-signature"}`, "STOP")), nil
		}
		if len(body.Contents) != 3 || body.Contents[1].Role != "model" {
			t.Error("model content not replayed")
		}
		encoded, _ := json.Marshal(body.Contents)
		if !strings.Contains(string(encoded), `"thoughtSignature":"call-signature"`) || !strings.Contains(string(encoded), `"id":"native-call","name":"butler_tool_0","response":{"result":{"title":"Example"},"status":"success"}`) {
			t.Error("signature or correlated function response lost")
		}
		return response(frame(`{"text":"Готово"}`, "STOP")), nil
	})
	// Older typed sources can still carry a project, but it is never routed to
	// AICode. All other fixtures resolve tokens without any project at all.
	p.config.AuthSource = authSource{legacyProject: "test-project"}
	stream, err := p.StartRun(context.Background(), request("run"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, stream)
	if err := findError(events); err != nil {
		t.Fatal(err)
	}
	var proposal *transport.ToolCallRequest
	var ref *transport.ProviderSessionRef
	for _, event := range events {
		if event.ToolCall != nil {
			proposal = event.ToolCall
		}
		if event.ProviderSessionRef != nil {
			ref = event.ProviderSessionRef
		}
		if event.AssistantDelta != nil {
			t.Fatal("private thought text exposed")
		}
	}
	if proposal == nil || proposal.ToolName != "web.fetch" || ref == nil {
		t.Fatal("missing Butler proposal/context")
	}
	if _, err := p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "run", ToolCallRef: "foreign-call", ToolResultJSON: `{}`}); err == nil {
		t.Fatal("foreign call accepted")
	}
	foreign := *ref
	foreign.SessionRef = "foreign-run"
	if _, err := p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "run", ProviderSessionRef: &foreign, ToolCallRef: proposal.ToolCallRef, ToolResultJSON: `{}`}); err == nil {
		t.Fatal("foreign context accepted")
	}
	stream, err = p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "run", ProviderSessionRef: ref, ToolCallRef: proposal.ToolCallRef, ToolResultJSON: `{"status":"success","result":{"title":"Example"}}`})
	if err != nil {
		t.Fatal(err)
	}
	events = collect(t, stream)
	if err := findError(events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[2].AssistantFinal == nil || events[2].AssistantFinal.Content != "Готово" || events[3].EventType != transport.EventTypeRunCompleted {
		t.Fatal("missing final streaming result")
	}
	if usage := events[2].AssistantFinal.Usage; usage == nil || usage.InputTokens != 17 || usage.OutputTokens != 14 || usage.TotalTokens != 31 {
		t.Fatal("flat usage metadata lost")
	}
	if _, err := p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "run", ToolCallRef: proposal.ToolCallRef, ToolResultJSON: `{}`}); err == nil {
		t.Fatal("result replay accepted")
	}
}

func TestTruncatedTextRetainsNativeFinishReason(t *testing.T) {
	p := newFixture(t, func(*http.Request) (*http.Response, error) {
		return response(frame(`{"text":"partial response"}`, "MAX_TOKENS")), nil
	})
	stream, err := p.StartRun(context.Background(), request("partial"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, stream)
	if err := findError(events); err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.AssistantFinal != nil {
			if event.AssistantFinal.FinishReason != "MAX_TOKENS" || event.AssistantFinal.Content != "partial response" {
				t.Fatal("truncated answer misrepresented as a complete response")
			}
			return
		}
	}
	t.Fatal("partial response finish reason missing")
}

func TestUnsafeOrIncompleteResponsesNeverProposeTools(t *testing.T) {
	cases := map[string]string{
		"foreign function":     frame(`{"functionCall":{"name":"run_shell_command","args":{}}}`, "STOP"),
		"bad schema":           frame(`{"functionCall":{"name":"butler_tool_0","args":{"url":42}}}`, "STOP"),
		"extra argument":       frame(`{"functionCall":{"name":"butler_tool_0","args":{"url":"https://example.com","shell":"x"}}}`, "STOP"),
		"duplicate key":        frame(`{"functionCall":{"name":"butler_tool_0","args":{"url":"good","url":"bad"}}}`, "STOP"),
		"parallel calls":       frame(`{"functionCall":{"name":"butler_tool_0","args":{"url":"a"}}},{"functionCall":{"name":"butler_tool_0","args":{"url":"b"}}}`, "STOP"),
		"server execution":     frame(`{"codeExecutionResult":{"output":"bad"}}`, "STOP"),
		"incomplete":           frame(`{"functionCall":{"name":"butler_tool_0","args":{"url":"a"}}}`, ""),
		"unterminated frame":   strings.TrimSuffix(frame(`{"functionCall":{"name":"butler_tool_0","args":{"url":"a"}}}`, "STOP"), "\n\n"),
		"truncated":            frame(`{"functionCall":{"name":"butler_tool_0","args":{"url":"a"}}}`, "MAX_TOKENS"),
		"automatic history":    `data: {"automaticFunctionCallingHistory":[{}],"candidates":[]}` + "\n\n",
		"multiple candidates":  `data: {"candidates":[{},{}]}` + "\n\n",
		"old wrapped response": `data: {"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"wrong route"}]},"finishReason":"STOP"}]}}` + "\n\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			p := newFixture(t, func(*http.Request) (*http.Response, error) { return response(body), nil })
			stream, err := p.StartRun(context.Background(), request("run"))
			if err != nil {
				t.Fatal(err)
			}
			events := collect(t, stream)
			if findError(events) == nil {
				t.Fatal("unsafe response accepted")
			}
			if name == "parallel calls" && (transport.DiagnosticCode(findError(events)) != "capability_mismatch" || transport.DiagnosticReason(findError(events)) != "batch_tool_calls_unsupported") {
				t.Fatal("parallel denial lost its fixed content-free reason")
			}
			for _, event := range events {
				if event.ToolCall != nil || event.ToolCallBatch != nil {
					t.Fatal("unsafe proposal exposed")
				}
			}
		})
	}
}

func TestHTTPFailuresMaskedNoRetryOrAuthRedirect(t *testing.T) {
	for _, status := range []int{302, 400, 401, 403, 404, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			p := newFixture(t, func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return &http.Response{StatusCode: status, Header: http.Header{"Location": {"https://foreign.invalid/steal"}}, Body: io.NopCloser(strings.NewReader(`{"error":"private-access-fixture account@email.test"}`))}, nil
			})
			stream, err := p.StartRun(context.Background(), request("run"))
			if err != nil {
				t.Fatal(err)
			}
			errorEvent := findError(collect(t, stream))
			if errorEvent == nil || strings.Contains(errorEvent.Message, "private-access") || strings.Contains(errorEvent.Message, "email") || errorEvent.Retryable || calls.Load() != 1 {
				t.Fatal("error leaked or retried")
			}
			if transport.DiagnosticHTTPStatus(errorEvent) != status {
				t.Fatal("safe actual HTTP status lost")
			}
			if status == 429 && errorEvent.Type != transport.ErrorTypeRateLimited {
				t.Fatal("quota not normalized")
			}
		})
	}
}

func TestCapacityFailureKeepsPrivateDiagnosticsBoundedAndDoesNotRetry(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		capacity   bool
	}{
		{"capacity", `{"error":{"message":"private-access-fixture account@email.test","details":[{"reason":"HIGH_DEMAND","metadata":{"account":"private-access-fixture"}}]}}`, true},
		{"quota", `{"error":{"details":[{"reason":"QUOTA_EXCEEDED"}]}}`, false},
		{"malformed", `{"error":{"details":[{"reason":"HIGH_DEMAND"}]`, false},
		{"oversized", `{"padding":"` + strings.Repeat("x", 64*1024) + `","error":{"details":[{"reason":"HIGH_DEMAND"}]}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			p := newFixture(t, func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})
			stream, err := p.StartRun(context.Background(), request("run"))
			if err != nil {
				t.Fatal(err)
			}
			event := findError(collect(t, stream))
			want := transport.ErrorTypeRateLimited
			if tc.capacity {
				want = transport.ErrorTypeModelCapacityUnavailable
			}
			if event == nil || event.Type != want || event.Retryable || calls.Load() != 1 {
				t.Fatal("capacity rejection must remain a single normalized failure")
			}
			if strings.Contains(event.Message, "capacity") != tc.capacity || strings.Contains(event.Message, "private-access") || strings.Contains(event.Message, "email") {
				t.Fatal("capacity classification leaked or accepted invalid diagnostics")
			}
		})
	}
}

func TestCancellationClosesHTTPAndIsolatedRuns(t *testing.T) {
	started := make(chan struct{}, 2)
	stopped := make(chan struct{}, 2)
	p := newFixture(t, func(req *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-req.Context().Done()
		stopped <- struct{}{}
		return nil, req.Context().Err()
	})
	streamA, err := p.StartRun(context.Background(), request("a"))
	if err != nil {
		t.Fatal(err)
	}
	streamB, err := p.StartRun(context.Background(), request("b"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	<-started
	if _, err := p.StartRun(context.Background(), request("a")); err == nil {
		t.Fatal("duplicate active run accepted")
	}
	if _, err := p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "a"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("HTTP not cancelled")
	}
	collect(t, streamA)
	p.mu.Lock()
	_, bAlive := p.runs["b"]
	p.mu.Unlock()
	if !bAlive {
		t.Fatal("cross-run cancellation")
	}
	_, _ = p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "b"})
	collect(t, streamB)
	if _, err := p.ContinueRun(context.Background(), transport.ContinueRunRequest{RunID: "a"}); err == nil {
		t.Fatal("lost context resumed")
	}
}

func TestCatalogSchemasAreSupported(t *testing.T) {
	data, err := os.ReadFile("../../../configs/tools.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Tools []struct {
			Name   string `json:"tool_name"`
			Schema string `json:"input_schema_json"`
		} `json:"tools"`
	}
	if json.Unmarshal(data, &catalog) != nil {
		t.Fatal("invalid catalog fixture")
	}
	for _, tool := range catalog.Tools {
		// Catalog contracts retain Broker bounds. Exercise the actual native
		// declaration conversion before checking the provider dialect.
		native, err := nativeToolSchema(tool.Schema)
		if err != nil {
			t.Fatalf("unsupported current schema %s: %v", tool.Name, err)
		}
		if err := validateSchema(string(native)); err != nil {
			t.Fatalf("unsupported native schema %s: %v", tool.Name, err)
		}
	}
	// Representative typed credential ref used by broker-aware browser fields.
	schema := `{"type":"object","properties":{"value":{"oneOf":[{"type":"string"},{"type":"object","properties":{"type":{"type":"string","enum":["credential_ref"]},"alias":{"type":"string"},"field":{"type":"string"}},"required":["type","alias","field"],"additionalProperties":false}]}},"required":["value"],"additionalProperties":false}`
	if err := validateSchema(schema); err != nil {
		t.Fatal(err)
	}
	if err := validateArguments(schema, json.RawMessage(`{"value":{"type":"credential_ref","alias":"Login","field":"password"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := validateSchema(`{"type":"object","$ref":"file:///secret"}`); err == nil {
		t.Fatal("unsupported schema accepted")
	}
	if err := validateArguments(`{"type":"object","properties":{"n":{"type":"integer"}}}`, json.RawMessage(`{"n":9007199254740993}`)); err != nil {
		t.Fatal("integer precision lost")
	}
}

func TestCancellationClosesRealSSEConnection(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, frame(`{"text":"partial"}`, ""))
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
		close(cancelled)
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	p := newFixture(t, func(req *http.Request) (*http.Response, error) {
		clone := req.Clone(req.Context())
		clone.URL.Scheme = target.Scheme
		clone.URL.Host = target.Host
		return http.DefaultTransport.RoundTrip(clone)
	})
	stream, err := p.StartRun(context.Background(), request("cancel-sse"))
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if _, err := p.CancelRun(context.Background(), transport.CancelRunRequest{RunID: "cancel-sse"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("real SSE connection was not cancelled")
	}
	collect(t, stream)
}
