package openaicodex

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
)

type staticAuth struct{}

func (staticAuth) ResolveOpenAICodex(context.Context) (providerauth.OpenAICodexAuth, error) {
	return providerauth.OpenAICodexAuth{AccessToken: "codex-token", AccountID: "acc_123", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func TestStartRunAppliesCodexAuthAndStreamsEvents(t *testing.T) {
	t.Parallel()
	requestBody := make(chan map[string]any, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/codex/responses" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		requestBody <- payload
		if got := r.Header.Get("Authorization"); got != "Bearer codex-token" {
			t.Fatalf("unexpected authorization %q", got)
		}
		if got := r.Header.Get("chatgpt-account-id"); got != "acc_123" {
			t.Fatalf("unexpected account id %q", got)
		}
		if got := r.Header.Get("conversation_id"); got != "telegram:chat:1" {
			t.Fatalf("unexpected conversation id %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hello\",\"sequence_number\":1}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output_text\":\"Hello\",\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5}}}\n\n")
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5.1-codex", BaseURL: server.URL, Timeout: 5 * time.Second, AuthSource: staticAuth{}}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	stream, err := provider.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run-1", SessionKey: "telegram:chat:1", ProviderName: providerName, ModelName: "gpt-5.1-codex"}, InputItems: []transport.InputItem{{Role: "system", Content: "operator prompt"}, {Role: "user", Content: "hello"}}, ToolDefinitions: []transport.ToolDefinition{{Name: "http.request", Description: "Fetch URL", SchemaJSON: `{"type":"object","properties":{"storage_state":{"type":"object","properties":{"cookies":{"type":"array"}}}}}`}, {Name: "doctor.check_container", Description: "Check container health", SchemaJSON: `{"type":"object"}`}}, StreamingEnabled: true})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	payload := <-requestBody
	if payload["store"] != false {
		t.Fatalf("expected store=false, got %+v", payload)
	}
	if payload["instructions"] != "operator prompt" {
		t.Fatalf("expected instructions to include system prompt, got %+v", payload)
	}
	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) != 2 {
		t.Fatalf("expected two tools, got %+v", payload["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok || tool["name"] != "http_request" {
		t.Fatalf("expected sanitized tool name, got %+v", tools[0])
	}
	parameters, ok := tool["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("expected tool parameters, got %+v", tool)
	}
	properties, ok := parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected schema properties, got %+v", parameters)
	}
	storageState, ok := properties["storage_state"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested storage_state schema, got %+v", properties["storage_state"])
	}
	nestedProps, ok := storageState["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested properties, got %+v", storageState)
	}
	cookies, ok := nestedProps["cookies"].(map[string]any)
	if !ok {
		t.Fatalf("expected cookies schema, got %+v", nestedProps["cookies"])
	}
	if _, ok := cookies["items"].(map[string]any); !ok {
		t.Fatalf("expected array items to be added, got %+v", cookies)
	}
	doctorTool, ok := tools[1].(map[string]any)
	if !ok || doctorTool["name"] != "doctor_check_container" {
		t.Fatalf("expected sanitized doctor tool name, got %+v", tools[1])
	}
	doctorParams, ok := doctorTool["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("expected doctor parameters, got %+v", doctorTool)
	}
	if doctorProps, ok := doctorParams["properties"].(map[string]any); !ok || len(doctorProps) != 0 {
		t.Fatalf("expected empty object properties to be added, got %+v", doctorParams)
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("expected only user input to be forwarded, got %+v", payload["input"])
	}
	item, ok := input[0].(map[string]any)
	if !ok || item["role"] != "user" {
		t.Fatalf("expected user input item, got %+v", input[0])
	}
	var eventTypes []transport.EventType
	for event := range stream {
		eventTypes = append(eventTypes, event.EventType)
	}
	want := []transport.EventType{transport.EventTypeRunStarted, transport.EventTypeProviderSessionBound, transport.EventTypeAssistantDelta, transport.EventTypeAssistantFinal, transport.EventTypeRunCompleted}
	if !reflect.DeepEqual(eventTypes, want) {
		t.Fatalf("unexpected event types: got %v want %v", eventTypes, want)
	}
}

func TestStartRunStreamDoesNotTimeoutAfterHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n")
		flusher.Flush()
		time.Sleep(150 * time.Millisecond)
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"output_text\":\"Hello\"}}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5.1-codex", BaseURL: server.URL, Timeout: 50 * time.Millisecond, AuthSource: staticAuth{}}, nil)
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	stream, err := provider.StartRun(context.Background(), transport.StartRunRequest{
		Context:          transport.TransportRunContext{RunID: "run-1", SessionKey: "telegram:chat:1", ProviderName: providerName, ModelName: "gpt-5.1-codex"},
		InputItems:       []transport.InputItem{{Role: "user", Content: "hello"}},
		StreamingEnabled: true,
	})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}

	var sawFinal bool
	for event := range stream {
		if event.EventType == transport.EventTypeAssistantFinal {
			sawFinal = true
		}
		if event.EventType == transport.EventTypeTransportError {
			t.Fatalf("unexpected transport error: %+v", event.TransportError)
		}
	}
	if !sawFinal {
		t.Fatal("expected assistant final event after delayed stream body")
	}
}

func TestStartRunMapsSanitizedToolCallsBackToOriginalNames(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"http_request\",\"arguments\":\"{}\"}}\n\n")
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5.1-codex", BaseURL: server.URL, Timeout: 5 * time.Second, AuthSource: staticAuth{}}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	stream, err := provider.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run-1", SessionKey: "telegram:chat:1", ProviderName: providerName, ModelName: "gpt-5.1-codex"}, InputItems: []transport.InputItem{{Role: "system", Content: "operator prompt"}, {Role: "user", Content: "hello"}}, ToolDefinitions: []transport.ToolDefinition{{Name: "http.request", Description: "Fetch URL", SchemaJSON: `{"type":"object","properties":{"storage_state":{"type":"object","properties":{"cookies":{"type":"array"}}}}}`}}, StreamingEnabled: true})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	var toolCall *transport.TransportEvent
	for event := range stream {
		if event.EventType == transport.EventTypeToolCallRequested {
			copy := event
			toolCall = &copy
		}
	}
	if toolCall == nil {
		t.Fatal("expected tool call event")
	}
	if toolCall.ToolCall.ToolName != "http.request" {
		t.Fatalf("expected original tool name, got %+v", toolCall.ToolCall)
	}
}

func TestSubmitToolResultReplaysFunctionCallStateless(t *testing.T) {
	t.Parallel()
	requestBody := make(chan map[string]any, 2)
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		requestBody <- payload
		w.Header().Set("Content-Type", "text/event-stream")
		if requestCount == 1 {
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"http_request\",\"arguments\":\"{}\"}}\n\n")
			return
		}
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\",\"status\":\"completed\",\"output_text\":\"done\"}}\n\n")
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5.1-codex", BaseURL: server.URL, Timeout: 5 * time.Second, AuthSource: staticAuth{}}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	startStream, err := provider.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run-1", SessionKey: "telegram:chat:1", ProviderName: providerName, ModelName: "gpt-5.1-codex"}, InputItems: []transport.InputItem{{Role: "system", Content: "operator prompt"}, {Role: "user", Content: "hello"}}, ToolDefinitions: []transport.ToolDefinition{{Name: "http.request", Description: "Fetch URL", SchemaJSON: `{"type":"object","properties":{}}`}}, StreamingEnabled: true})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	for range startStream {
	}
	<-requestBody
	stream, err := provider.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "run-1", ProviderSessionRef: &transport.ProviderSessionRef{ProviderName: providerName, SessionRef: "telegram:chat:1", ResponseRef: "resp_prev"}, ToolCallRef: "call_1", ToolResultJSON: `{"ok":true}`})
	if err != nil {
		t.Fatalf("SubmitToolResult returned error: %v", err)
	}
	for range stream {
	}
	payload := <-requestBody
	if payload["store"] != false {
		t.Fatalf("expected store=false, got %+v", payload)
	}
	if _, ok := payload["previous_response_id"]; ok {
		t.Fatalf("expected previous_response_id to be omitted, got %+v", payload)
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) != 2 {
		t.Fatalf("expected two input items (function_call + function_call_output), got %+v", payload["input"])
	}
	funcCall, ok := input[0].(map[string]any)
	if !ok || funcCall["type"] != "function_call" {
		t.Fatalf("expected first input to be function_call, got %+v", input[0])
	}
	if funcCall["call_id"] != "call_1" {
		t.Fatalf("expected function_call call_id=call_1, got %+v", funcCall["call_id"])
	}
	if funcCall["name"] != "http_request" {
		t.Fatalf("expected function_call name=http_request, got %+v", funcCall["name"])
	}
	funcOutput, ok := input[1].(map[string]any)
	if !ok || funcOutput["type"] != "function_call_output" {
		t.Fatalf("expected second input to be function_call_output, got %+v", input[1])
	}
	if funcOutput["output"] != `{"ok":true}` {
		t.Fatalf("expected tool result output string, got %+v", funcOutput["output"])
	}
	if payload["instructions"] != "operator prompt" {
		t.Fatalf("expected submit tool result to reuse instructions, got %+v", payload)
	}
}

func TestContinueRunOmitsPreviousResponseID(t *testing.T) {
	t.Parallel()
	requestBody := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		requestBody <- payload
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\",\"status\":\"completed\",\"output_text\":\"done\"}}\n\n")
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5.1-codex", BaseURL: server.URL, Timeout: 5 * time.Second, AuthSource: staticAuth{}}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	stream, err := provider.ContinueRun(context.Background(), transport.ContinueRunRequest{RunID: "run-1", ProviderSessionRef: &transport.ProviderSessionRef{ProviderName: providerName, SessionRef: "telegram:chat:1", ResponseRef: "resp_prev"}, InputItems: []transport.InputItem{{Role: "system", Content: "updated prompt"}, {Role: "user", Content: "next"}}})
	if err != nil {
		t.Fatalf("ContinueRun returned error: %v", err)
	}
	for range stream {
	}
	payload := <-requestBody
	if _, ok := payload["previous_response_id"]; ok {
		t.Fatalf("expected previous_response_id to be omitted, got %+v", payload)
	}
	if payload["instructions"] != "updated prompt" {
		t.Fatalf("expected instructions to include system prompt, got %+v", payload)
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("expected only user input to be forwarded, got %+v", payload["input"])
	}
}

func TestContinueRunMovesSystemMessagesIntoInstructions(t *testing.T) {
	t.Parallel()
	requestBody := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		requestBody <- payload
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_2\",\"status\":\"completed\",\"output_text\":\"done\"}}\n\n")
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5.1-codex", BaseURL: server.URL, Timeout: 5 * time.Second, AuthSource: staticAuth{}}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	stream, err := provider.ContinueRun(context.Background(), transport.ContinueRunRequest{RunID: "run-1", ProviderSessionRef: &transport.ProviderSessionRef{ProviderName: providerName, SessionRef: "telegram:chat:1", ResponseRef: "resp_prev"}, InputItems: []transport.InputItem{{Role: "system", Content: "updated prompt"}, {Role: "user", Content: "next"}}})
	if err != nil {
		t.Fatalf("ContinueRun returned error: %v", err)
	}
	for range stream {
	}
	payload := <-requestBody
	if payload["store"] != false {
		t.Fatalf("expected store=false, got %+v", payload)
	}
	if payload["instructions"] != "updated prompt" {
		t.Fatalf("expected instructions to include system prompt, got %+v", payload)
	}
	input, ok := payload["input"].([]any)
	if !ok || len(input) != 1 {
		t.Fatalf("expected only user input to be forwarded, got %+v", payload["input"])
	}
}

func TestStartRunUsesPendingCodexToolMetadata(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_item.added\",\"item\":{\"id\":\"fc_abc123\",\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"http_request\"}}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.function_call_arguments.delta\",\"item_id\":\"fc_abc123\",\"delta\":\"{\\\"url\\\":\"}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.function_call_arguments.done\",\"item_id\":\"fc_abc123\",\"arguments\":{\"url\":\"https://example.com\"}}\n\n")
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5.1-codex", BaseURL: server.URL, Timeout: 5 * time.Second, AuthSource: staticAuth{}}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	stream, err := provider.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run-1", SessionKey: "telegram:chat:1", ProviderName: providerName, ModelName: "gpt-5.1-codex"}, InputItems: []transport.InputItem{{Role: "user", Content: "hello"}}, ToolDefinitions: []transport.ToolDefinition{{Name: "http.request", Description: "Fetch URL", SchemaJSON: `{"type":"object","properties":{}}`}}, StreamingEnabled: true})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	var warnings []transport.TransportEvent
	var toolCall *transport.TransportEvent
	for event := range stream {
		if event.EventType == transport.EventTypeTransportWarning {
			warnings = append(warnings, event)
		}
		if event.EventType == transport.EventTypeToolCallRequested {
			copy := event
			toolCall = &copy
		}
	}
	if len(warnings) != 0 {
		t.Fatalf("expected intermediate codex events to be ignored, got warnings %+v", warnings)
	}
	if toolCall == nil {
		t.Fatal("expected tool call event")
	}
	if toolCall.ToolCall.ToolCallRef != "call_1" {
		t.Fatalf("expected cached call id, got %q", toolCall.ToolCall.ToolCallRef)
	}
	if toolCall.ToolCall.ToolName != "http.request" {
		t.Fatalf("expected cached tool name, got %q", toolCall.ToolCall.ToolName)
	}
	if toolCall.ToolCall.ArgsJSON != `{"url":"https://example.com"}` {
		t.Fatalf("expected arguments from done event, got %q", toolCall.ToolCall.ArgsJSON)
	}
}

func TestStartRunExtractsToolCallRefFromIDFieldFallback(t *testing.T) {
	t.Parallel()
	// When a function_call item has "id" but no "call_id", the provider should
	// fall back to "id" for the ToolCallRef.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"id\":\"fc_abc123\",\"name\":\"http_request\",\"arguments\":\"{}\"}}\n\n")
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5.1-codex", BaseURL: server.URL, Timeout: 5 * time.Second, AuthSource: staticAuth{}}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	stream, err := provider.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run-1", SessionKey: "telegram:chat:1", ProviderName: providerName, ModelName: "gpt-5.1-codex"}, InputItems: []transport.InputItem{{Role: "user", Content: "hello"}}, ToolDefinitions: []transport.ToolDefinition{{Name: "http.request", Description: "Fetch URL", SchemaJSON: `{"type":"object","properties":{}}`}}, StreamingEnabled: true})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	var toolCall *transport.TransportEvent
	for event := range stream {
		if event.EventType == transport.EventTypeToolCallRequested {
			copy := event
			toolCall = &copy
		}
	}
	if toolCall == nil {
		t.Fatal("expected tool call event")
	}
	if toolCall.ToolCall.ToolCallRef != "fc_abc123" {
		t.Fatalf("expected ToolCallRef from id field fallback, got %q", toolCall.ToolCall.ToolCallRef)
	}
	if toolCall.ToolCall.ToolName != "http.request" {
		t.Fatalf("expected original tool name, got %q", toolCall.ToolCall.ToolName)
	}
}

func TestSubmitToolResultPreservesToolAliasesAcrossResume(t *testing.T) {
	t.Parallel()
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "text/event-stream")
		switch requestCount {
		case 1:
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"call_id\":\"call_1\",\"name\":\"http_request\",\"arguments\":\"{}\"}}\n\n")
		case 2:
			_, _ = fmt.Fprint(w, "data: {\"type\":\"response.output_item.done\",\"item\":{\"type\":\"function_call\",\"call_id\":\"call_2\",\"name\":\"http_request\",\"arguments\":\"{}\"}}\n\n")
		default:
			t.Fatalf("unexpected request count %d", requestCount)
		}
	}))
	defer server.Close()

	provider, err := NewProvider(Config{Model: "gpt-5.1-codex", BaseURL: server.URL, Timeout: 5 * time.Second, AuthSource: staticAuth{}}, server.Client())
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}
	startStream, err := provider.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run-1", SessionKey: "telegram:chat:1", ProviderName: providerName, ModelName: "gpt-5.1-codex"}, InputItems: []transport.InputItem{{Role: "system", Content: "operator prompt"}, {Role: "user", Content: "hello"}}, ToolDefinitions: []transport.ToolDefinition{{Name: "http.request", Description: "Fetch URL", SchemaJSON: `{"type":"object","properties":{}}`}}, StreamingEnabled: true})
	if err != nil {
		t.Fatalf("StartRun returned error: %v", err)
	}
	for range startStream {
	}
	resumeStream, err := provider.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{RunID: "run-1", ProviderSessionRef: &transport.ProviderSessionRef{ProviderName: providerName, SessionRef: "telegram:chat:1", ResponseRef: "resp_prev"}, ToolCallRef: "call_1", ToolResultJSON: `{"ok":true}`})
	if err != nil {
		t.Fatalf("SubmitToolResult returned error: %v", err)
	}
	var toolCall *transport.TransportEvent
	for event := range resumeStream {
		if event.EventType == transport.EventTypeToolCallRequested {
			copy := event
			toolCall = &copy
		}
	}
	if toolCall == nil {
		t.Fatal("expected tool call event after resume")
	}
	if toolCall.ToolCall.ToolName != "http.request" {
		t.Fatalf("expected original tool name after resume, got %q", toolCall.ToolCall.ToolName)
	}
}

func TestSplitInstructionsUsesOutputTextForAssistant(t *testing.T) {
	t.Parallel()

	items := []transport.InputItem{
		{Role: "system", Content: "be helpful"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
		{Role: "user", Content: "next question"},
	}
	instructions, encoded := splitInstructions(items)
	if instructions != "be helpful" {
		t.Fatalf("expected instructions 'be helpful', got %q", instructions)
	}
	if len(encoded) != 3 {
		t.Fatalf("expected 3 encoded items (excluding system), got %d", len(encoded))
	}

	// user → input_text
	userContent := encoded[0]["content"].([]map[string]any)
	if userContent[0]["type"] != "input_text" {
		t.Fatalf("expected input_text for user role, got %v", userContent[0]["type"])
	}

	// assistant → output_text
	assistantContent := encoded[1]["content"].([]map[string]any)
	if assistantContent[0]["type"] != "output_text" {
		t.Fatalf("expected output_text for assistant role, got %v", assistantContent[0]["type"])
	}

	// user → input_text
	user2Content := encoded[2]["content"].([]map[string]any)
	if user2Content[0]["type"] != "input_text" {
		t.Fatalf("expected input_text for second user role, got %v", user2Content[0]["type"])
	}
}

func TestContentTypeForRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		role string
		want string
	}{
		{"user", "input_text"},
		{"assistant", "output_text"},
		{"system", "input_text"},
		{"Assistant", "output_text"},
		{"", "input_text"},
	}
	for _, tt := range tests {
		got := contentTypeForRole(tt.role)
		if got != tt.want {
			t.Errorf("contentTypeForRole(%q) = %q, want %q", tt.role, got, tt.want)
		}
	}
}
