package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/observability"
	"github.com/butler/butler/apps/orchestrator/internal/run"
)

// --- fakes for event tests ---

type fakeTransitionLister struct {
	mu          sync.Mutex
	transitions []run.StateTransition
	err         error
}

func (f *fakeTransitionLister) ListTransitions(_ context.Context, runID string) ([]run.StateTransition, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	var result []run.StateTransition
	for _, t := range f.transitions {
		if t.RunID == runID {
			result = append(result, t)
		}
	}
	return result, nil
}

// --- SSE parsing helpers ---

type sseEvent struct {
	EventType string
	Data      string
}

// parseSSEEvents reads SSE events from a body string.
func parseSSEEvents(body string) []sseEvent {
	var events []sseEvent
	scanner := bufio.NewScanner(strings.NewReader(body))
	var current sseEvent
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			current.EventType = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			current.Data = strings.TrimPrefix(line, "data: ")
		} else if line == "" && current.EventType != "" {
			events = append(events, current)
			current = sseEvent{}
		}
	}
	// Handle last event if no trailing blank line.
	if current.EventType != "" {
		events = append(events, current)
	}
	return events
}

func parseEventPayload(data string) map[string]any {
	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		return nil
	}
	return payload
}

// --- SSE endpoint tests ---

func TestHandleSSE_RunNotFound(t *testing.T) {
	t.Parallel()

	hub := observability.NewHub()
	server := NewEventServer(nil, &fakeRunLister{}, hub, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/nonexistent/events", nil)
	rr := httptest.NewRecorder()
	server.HandleSSE().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleSSE_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	hub := observability.NewHub()
	server := NewEventServer(nil, &fakeRunLister{}, hub, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs/run-1/events", nil)
	rr := httptest.NewRecorder()
	server.HandleSSE().ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestHandleSSE_MissingRunID(t *testing.T) {
	t.Parallel()

	hub := observability.NewHub()
	server := NewEventServer(nil, &fakeRunLister{}, hub, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs//events", nil)
	rr := httptest.NewRecorder()
	server.HandleSSE().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleSSE_TerminalRunReturnsReplayAndStreamEnd(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	transitions := &fakeTransitionLister{
		transitions: []run.StateTransition{
			{ID: 1, RunID: "run-1", FromState: "created", ToState: "queued", TriggeredBy: "orchestrator", MetadataJSON: "{}", TransitionedAt: now},
			{ID: 2, RunID: "run-1", FromState: "queued", ToState: "acquired", TriggeredBy: "orchestrator", MetadataJSON: "{}", TransitionedAt: now.Add(time.Second)},
			{ID: 3, RunID: "run-1", FromState: "acquired", ToState: "completed", TriggeredBy: "orchestrator", MetadataJSON: "{}", TransitionedAt: now.Add(2 * time.Second)},
		},
	}
	runs := &fakeRunLister{single: &run.Record{
		RunID:        "run-1",
		SessionKey:   "session-1",
		CurrentState: "completed",
		Status:       "completed",
		StartedAt:    now,
		UpdatedAt:    now,
	}}

	hub := observability.NewHub()
	server := NewEventServer(transitions, runs, hub, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/events", nil)
	rr := httptest.NewRecorder()
	server.HandleSSE().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Verify SSE headers.
	if ct := rr.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", ct)
	}
	if cc := rr.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %s", cc)
	}

	events := parseSSEEvents(rr.Body.String())
	if len(events) == 0 {
		t.Fatal("expected at least one SSE event")
	}

	// Should have 3 replay state_transition events + 1 stream_end.
	stateEvents := 0
	streamEndEvents := 0
	for _, evt := range events {
		switch evt.EventType {
		case "state_transition":
			stateEvents++
			payload := parseEventPayload(evt.Data)
			if payload == nil {
				t.Errorf("could not parse state_transition data: %s", evt.Data)
				continue
			}
			// Replay events should have "replay": true in payload.
			payloadMap := payload["payload"].(map[string]any)
			if replay, ok := payloadMap["replay"]; !ok || replay != true {
				t.Errorf("expected replay=true in state_transition, got %v", payloadMap)
			}
		case "stream_end":
			streamEndEvents++
			payload := parseEventPayload(evt.Data)
			if payload == nil {
				t.Errorf("could not parse stream_end data: %s", evt.Data)
				continue
			}
			payloadMap := payload["payload"].(map[string]any)
			if reason, ok := payloadMap["reason"]; !ok || reason != "run_terminal" {
				t.Errorf("expected stream_end reason=run_terminal, got %v", payloadMap)
			}
		}
	}

	if stateEvents != 3 {
		t.Errorf("expected 3 state_transition replay events, got %d", stateEvents)
	}
	if streamEndEvents != 1 {
		t.Errorf("expected 1 stream_end event, got %d", streamEndEvents)
	}
}

func TestHandleSSE_NoHubReturnsStreamEnd(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	runs := &fakeRunLister{single: &run.Record{
		RunID:        "run-1",
		SessionKey:   "session-1",
		CurrentState: "model_running",
		Status:       "active",
		StartedAt:    now,
		UpdatedAt:    now,
	}}

	// No hub — should return stream_end immediately.
	server := NewEventServer(nil, runs, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/events", nil)
	rr := httptest.NewRecorder()
	server.HandleSSE().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	events := parseSSEEvents(rr.Body.String())
	foundStreamEnd := false
	for _, evt := range events {
		if evt.EventType == "stream_end" {
			foundStreamEnd = true
			payload := parseEventPayload(evt.Data)
			if payload != nil {
				payloadMap := payload["payload"].(map[string]any)
				if reason := payloadMap["reason"]; reason != "no_event_hub" {
					t.Errorf("expected reason=no_event_hub, got %v", reason)
				}
			}
		}
	}
	if !foundStreamEnd {
		t.Error("expected stream_end event when hub is nil")
	}
}

func TestHandleSSE_LiveStreamingPublishAndTerminalClose(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	runs := &fakeRunLister{single: &run.Record{
		RunID:        "run-1",
		SessionKey:   "session-1",
		CurrentState: "model_running",
		Status:       "active",
		StartedAt:    now,
		UpdatedAt:    now,
	}}

	hub := observability.NewHub()
	server := NewEventServer(nil, runs, hub, nil)

	// Start SSE in a real test server to get true HTTP streaming.
	ts := httptest.NewServer(server.HandleSSE())
	defer ts.Close()

	// Connect to SSE endpoint.
	sseURL := ts.URL + "/api/v1/runs/run-1/events"
	resp, err := http.Get(sseURL)
	if err != nil {
		t.Fatalf("failed to connect to SSE: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", ct)
	}

	// Wait for SSE subscription to be established.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.SubscriberCount("run-1") > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if hub.SubscriberCount("run-1") == 0 {
		t.Fatal("SSE endpoint did not subscribe to EventHub")
	}

	// Publish some events.
	hub.Publish("run-1", observability.NewEvent("run-1", "session-1", observability.EventMemoryLoaded, map[string]any{
		"bundle_count": 3,
	}))
	hub.Publish("run-1", observability.NewEvent("run-1", "session-1", observability.EventToolStarted, map[string]any{
		"tool_name": "http.request",
	}))
	hub.Publish("run-1", observability.NewEvent("run-1", "session-1", observability.EventToolCompleted, map[string]any{
		"tool_name":   "http.request",
		"duration_ms": 150,
	}))
	// Send terminal event.
	hub.Publish("run-1", observability.NewEvent("run-1", "session-1", observability.EventRunCompleted, map[string]any{
		"response_length": 42,
	}))

	// Read all SSE events from the stream.
	scanner := bufio.NewScanner(resp.Body)
	var receivedEvents []sseEvent
	readDeadline := time.After(5 * time.Second)
	var current sseEvent

	for {
		select {
		case <-readDeadline:
			goto done
		default:
		}

		// Use a channel to handle scanner blocking.
		lineCh := make(chan string, 1)
		errCh := make(chan error, 1)
		go func() {
			if scanner.Scan() {
				lineCh <- scanner.Text()
			} else {
				errCh <- scanner.Err()
			}
		}()

		select {
		case line := <-lineCh:
			if strings.HasPrefix(line, "event: ") {
				current.EventType = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				current.Data = strings.TrimPrefix(line, "data: ")
			} else if line == "" && current.EventType != "" {
				receivedEvents = append(receivedEvents, current)
				if current.EventType == "stream_end" {
					goto done
				}
				current = sseEvent{}
			}
		case <-errCh:
			goto done
		case <-readDeadline:
			goto done
		}
	}
done:

	// Verify we received the events we published.
	eventTypes := make([]string, 0, len(receivedEvents))
	for _, e := range receivedEvents {
		eventTypes = append(eventTypes, e.EventType)
	}

	expectedTypes := []string{"memory_loaded", "tool_started", "tool_completed", "run_completed", "stream_end"}
	for _, expected := range expectedTypes {
		found := false
		for _, got := range eventTypes {
			if got == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected event type %q in stream, got types: %v", expected, eventTypes)
		}
	}
}

func TestHandleSSE_CleanupRunClosesSSEStream(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	runs := &fakeRunLister{single: &run.Record{
		RunID:        "run-cleanup",
		SessionKey:   "session-1",
		CurrentState: "model_running",
		Status:       "active",
		StartedAt:    now,
		UpdatedAt:    now,
	}}

	hub := observability.NewHub()
	server := NewEventServer(nil, runs, hub, nil)

	ts := httptest.NewServer(server.HandleSSE())
	defer ts.Close()

	sseURL := ts.URL + "/api/v1/runs/run-cleanup/events"
	resp, err := http.Get(sseURL)
	if err != nil {
		t.Fatalf("failed to connect to SSE: %v", err)
	}
	defer resp.Body.Close()

	// Wait for subscription.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.SubscriberCount("run-cleanup") > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Cleanup the run — should close subscriber channel.
	hub.CleanupRun("run-cleanup")

	// Read events — should get stream_end with reason "channel_closed".
	scanner := bufio.NewScanner(resp.Body)
	var receivedEvents []sseEvent
	readDeadline := time.After(3 * time.Second)
	var current sseEvent

	for {
		lineCh := make(chan string, 1)
		errCh := make(chan error, 1)
		go func() {
			if scanner.Scan() {
				lineCh <- scanner.Text()
			} else {
				errCh <- scanner.Err()
			}
		}()

		select {
		case line := <-lineCh:
			if strings.HasPrefix(line, "event: ") {
				current.EventType = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				current.Data = strings.TrimPrefix(line, "data: ")
			} else if line == "" && current.EventType != "" {
				receivedEvents = append(receivedEvents, current)
				if current.EventType == "stream_end" {
					goto done
				}
				current = sseEvent{}
			}
		case <-errCh:
			goto done
		case <-readDeadline:
			goto done
		}
	}
done:

	foundChannelClosed := false
	for _, evt := range receivedEvents {
		if evt.EventType == "stream_end" {
			payload := parseEventPayload(evt.Data)
			if payload != nil {
				payloadMap, ok := payload["payload"].(map[string]any)
				if ok && payloadMap["reason"] == "channel_closed" {
					foundChannelClosed = true
				}
			}
		}
	}
	if !foundChannelClosed {
		t.Error("expected stream_end with reason=channel_closed after CleanupRun")
	}
}

func TestHandleSSE_ClientDisconnectUnsubscribes(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	runs := &fakeRunLister{single: &run.Record{
		RunID:        "run-disconnect",
		SessionKey:   "session-1",
		CurrentState: "model_running",
		Status:       "active",
		StartedAt:    now,
		UpdatedAt:    now,
	}}

	hub := observability.NewHub()
	server := NewEventServer(nil, runs, hub, nil)

	ts := httptest.NewServer(server.HandleSSE())
	defer ts.Close()

	sseURL := ts.URL + "/api/v1/runs/run-disconnect/events"

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, sseURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to connect to SSE: %v", err)
	}

	// Wait for subscription.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.SubscriberCount("run-disconnect") > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if hub.SubscriberCount("run-disconnect") == 0 {
		t.Fatal("SSE endpoint did not subscribe")
	}

	// Cancel the client context (simulates disconnect).
	cancel()
	resp.Body.Close()

	// Wait for the SSE handler to clean up.
	cleanupDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(cleanupDeadline) {
		if hub.SubscriberCount("run-disconnect") == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if hub.SubscriberCount("run-disconnect") != 0 {
		t.Errorf("expected 0 subscribers after client disconnect, got %d", hub.SubscriberCount("run-disconnect"))
	}
}

func TestHandleSSE_CatchUpReplayWithLiveTransition(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	transitions := &fakeTransitionLister{
		transitions: []run.StateTransition{
			{ID: 1, RunID: "run-replay", FromState: "created", ToState: "queued", TriggeredBy: "orchestrator", MetadataJSON: "{}", TransitionedAt: now},
		},
	}
	runs := &fakeRunLister{single: &run.Record{
		RunID:        "run-replay",
		SessionKey:   "session-1",
		CurrentState: "model_running",
		Status:       "active",
		StartedAt:    now,
		UpdatedAt:    now,
	}}

	hub := observability.NewHub()
	server := NewEventServer(transitions, runs, hub, nil)

	ts := httptest.NewServer(server.HandleSSE())
	defer ts.Close()

	sseURL := ts.URL + "/api/v1/runs/run-replay/events"
	resp, err := http.Get(sseURL)
	if err != nil {
		t.Fatalf("failed to connect to SSE: %v", err)
	}
	defer resp.Body.Close()

	// Wait for subscription.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.SubscriberCount("run-replay") > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Send a terminal event.
	hub.Publish("run-replay", observability.NewEvent("run-replay", "session-1", observability.EventRunCompleted, map[string]any{
		"response_length": 100,
	}))

	// Read events.
	scanner := bufio.NewScanner(resp.Body)
	var receivedEvents []sseEvent
	readDeadline := time.After(5 * time.Second)
	var current sseEvent

	for {
		lineCh := make(chan string, 1)
		errCh := make(chan error, 1)
		go func() {
			if scanner.Scan() {
				lineCh <- scanner.Text()
			} else {
				errCh <- scanner.Err()
			}
		}()

		select {
		case line := <-lineCh:
			if strings.HasPrefix(line, "event: ") {
				current.EventType = strings.TrimPrefix(line, "event: ")
			} else if strings.HasPrefix(line, "data: ") {
				current.Data = strings.TrimPrefix(line, "data: ")
			} else if line == "" && current.EventType != "" {
				receivedEvents = append(receivedEvents, current)
				if current.EventType == "stream_end" {
					goto done
				}
				current = sseEvent{}
			}
		case <-errCh:
			goto done
		case <-readDeadline:
			goto done
		}
	}
done:

	// We expect: replay state_transition, live run_completed, stream_end.
	eventTypes := make([]string, 0)
	for _, e := range receivedEvents {
		eventTypes = append(eventTypes, e.EventType)
	}

	if len(eventTypes) < 3 {
		t.Fatalf("expected at least 3 events (replay + live + stream_end), got %d: %v", len(eventTypes), eventTypes)
	}

	// First should be state_transition (replay).
	if eventTypes[0] != "state_transition" {
		t.Errorf("expected first event to be state_transition (replay), got %s", eventTypes[0])
	}

	// Should contain run_completed.
	foundCompleted := false
	for _, et := range eventTypes {
		if et == "run_completed" {
			foundCompleted = true
		}
	}
	if !foundCompleted {
		t.Error("expected run_completed event in stream")
	}

	// Last should be stream_end.
	if eventTypes[len(eventTypes)-1] != "stream_end" {
		t.Errorf("expected last event to be stream_end, got %s", eventTypes[len(eventTypes)-1])
	}
}

func TestHandleSSE_MultipleSubscribersReceiveEvents(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	runs := &fakeRunLister{single: &run.Record{
		RunID:        "run-multi",
		SessionKey:   "session-1",
		CurrentState: "model_running",
		Status:       "active",
		StartedAt:    now,
		UpdatedAt:    now,
	}}

	hub := observability.NewHub()
	server := NewEventServer(nil, runs, hub, nil)

	ts := httptest.NewServer(server.HandleSSE())
	defer ts.Close()

	sseURL := ts.URL + "/api/v1/runs/run-multi/events"

	// Connect two SSE clients.
	resp1, err := http.Get(sseURL)
	if err != nil {
		t.Fatalf("client 1 failed to connect: %v", err)
	}
	defer resp1.Body.Close()

	resp2, err := http.Get(sseURL)
	if err != nil {
		t.Fatalf("client 2 failed to connect: %v", err)
	}
	defer resp2.Body.Close()

	// Wait for both subscriptions.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if hub.SubscriberCount("run-multi") >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if hub.SubscriberCount("run-multi") < 2 {
		t.Fatalf("expected 2 subscribers, got %d", hub.SubscriberCount("run-multi"))
	}

	// Publish a terminal event.
	hub.Publish("run-multi", observability.NewEvent("run-multi", "session-1", observability.EventRunCompleted, nil))

	// Both clients should receive the event.
	readEvents := func(resp *http.Response) []string {
		scanner := bufio.NewScanner(resp.Body)
		var types []string
		readDeadline := time.After(3 * time.Second)
		var current sseEvent
		for {
			lineCh := make(chan string, 1)
			errCh := make(chan error, 1)
			go func() {
				if scanner.Scan() {
					lineCh <- scanner.Text()
				} else {
					errCh <- scanner.Err()
				}
			}()
			select {
			case line := <-lineCh:
				if strings.HasPrefix(line, "event: ") {
					current.EventType = strings.TrimPrefix(line, "event: ")
				} else if strings.HasPrefix(line, "data: ") {
					current.Data = strings.TrimPrefix(line, "data: ")
				} else if line == "" && current.EventType != "" {
					types = append(types, current.EventType)
					if current.EventType == "stream_end" {
						return types
					}
					current = sseEvent{}
				}
			case <-errCh:
				return types
			case <-readDeadline:
				return types
			}
		}
	}

	var wg sync.WaitGroup
	var types1, types2 []string
	wg.Add(2)
	go func() {
		defer wg.Done()
		types1 = readEvents(resp1)
	}()
	go func() {
		defer wg.Done()
		types2 = readEvents(resp2)
	}()
	wg.Wait()

	for i, types := range [][]string{types1, types2} {
		foundCompleted := false
		for _, t := range types {
			if t == "run_completed" {
				foundCompleted = true
			}
		}
		if !foundCompleted {
			t.Errorf("client %d did not receive run_completed, got: %v", i+1, types)
		}
	}
}

// --- Transitions REST endpoint tests ---

func TestHandleListTransitions_ReturnsTransitions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	transitions := &fakeTransitionLister{
		transitions: []run.StateTransition{
			{ID: 1, RunID: "run-1", FromState: "created", ToState: "queued", TriggeredBy: "orchestrator", MetadataJSON: `{"lease_id":"lease-1"}`, TransitionedAt: now},
			{ID: 2, RunID: "run-1", FromState: "queued", ToState: "acquired", TriggeredBy: "orchestrator", MetadataJSON: "{}", TransitionedAt: now.Add(time.Second)},
		},
	}
	runs := &fakeRunLister{single: &run.Record{
		RunID:        "run-1",
		SessionKey:   "session-1",
		CurrentState: "completed",
		Status:       "completed",
		StartedAt:    now,
		UpdatedAt:    now,
	}}

	server := NewEventServer(transitions, runs, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/transitions", nil)
	rr := httptest.NewRecorder()
	server.HandleListTransitions().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	items, ok := resp["transitions"].([]any)
	if !ok {
		t.Fatal("expected transitions array")
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 transitions, got %d", len(items))
	}

	// Verify DTO fields.
	first := items[0].(map[string]any)
	if first["from_state"] != "created" {
		t.Errorf("expected from_state=created, got %v", first["from_state"])
	}
	if first["to_state"] != "queued" {
		t.Errorf("expected to_state=queued, got %v", first["to_state"])
	}
	if first["triggered_by"] != "orchestrator" {
		t.Errorf("expected triggered_by=orchestrator, got %v", first["triggered_by"])
	}
}

func TestHandleListTransitions_RunNotFound(t *testing.T) {
	t.Parallel()

	server := NewEventServer(nil, &fakeRunLister{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/nonexistent/transitions", nil)
	rr := httptest.NewRecorder()
	server.HandleListTransitions().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleListTransitions_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	server := NewEventServer(nil, &fakeRunLister{}, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs/run-1/transitions", nil)
	rr := httptest.NewRecorder()
	server.HandleListTransitions().ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
}

func TestHandleListTransitions_NoTransitionRepository(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	runs := &fakeRunLister{single: &run.Record{
		RunID:        "run-1",
		SessionKey:   "session-1",
		CurrentState: "completed",
		Status:       "completed",
		StartedAt:    now,
		UpdatedAt:    now,
	}}

	server := NewEventServer(nil, runs, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/transitions", nil)
	rr := httptest.NewRecorder()
	server.HandleListTransitions().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	items := resp["transitions"].([]any)
	if len(items) != 0 {
		t.Fatalf("expected 0 transitions, got %d", len(items))
	}
}

func TestHandleListTransitions_MissingRunID(t *testing.T) {
	t.Parallel()

	server := NewEventServer(nil, &fakeRunLister{}, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs//transitions", nil)
	rr := httptest.NewRecorder()
	server.HandleListTransitions().ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestHandleListTransitions_TransitionErrorReturnsInternalError(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	transitions := &fakeTransitionLister{
		err: fmt.Errorf("database connection lost"),
	}
	runs := &fakeRunLister{single: &run.Record{
		RunID:        "run-1",
		SessionKey:   "session-1",
		CurrentState: "completed",
		Status:       "completed",
		StartedAt:    now,
		UpdatedAt:    now,
	}}

	server := NewEventServer(transitions, runs, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/transitions", nil)
	rr := httptest.NewRecorder()
	server.HandleListTransitions().ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// --- writeSSEEvent tests ---

func TestWriteSSEEvent_FormatsCorrectly(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	evt := observability.Event{
		EventID:    "test-1",
		RunID:      "run-1",
		SessionKey: "session-1",
		EventType:  observability.EventStateTransition,
		Timestamp:  time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC),
		Payload:    map[string]any{"from_state": "created", "to_state": "queued"},
	}

	writeSSEEvent(rr, evt)
	body := rr.Body.String()

	if !strings.HasPrefix(body, "event: state_transition\n") {
		t.Errorf("expected event line prefix, got: %s", body)
	}
	if !strings.Contains(body, "data: ") {
		t.Error("expected data line in SSE output")
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Error("expected trailing double newline in SSE output")
	}

	// Verify the data is valid JSON.
	events := parseSSEEvents(body)
	if len(events) != 1 {
		t.Fatalf("expected 1 SSE event, got %d", len(events))
	}
	var parsed observability.Event
	if err := json.Unmarshal([]byte(events[0].Data), &parsed); err != nil {
		t.Fatalf("SSE data is not valid JSON: %v", err)
	}
	if parsed.EventID != "test-1" {
		t.Errorf("expected event_id=test-1, got %s", parsed.EventID)
	}
	if parsed.EventType != observability.EventStateTransition {
		t.Errorf("expected event_type=state_transition, got %s", parsed.EventType)
	}
}

func TestWriteSSEEvent_EmptyPayload(t *testing.T) {
	t.Parallel()

	rr := httptest.NewRecorder()
	evt := observability.Event{
		EventID:   "test-2",
		RunID:     "run-1",
		EventType: observability.EventRunCompleted,
		Timestamp: time.Now().UTC(),
	}

	writeSSEEvent(rr, evt)

	events := parseSSEEvents(rr.Body.String())
	if len(events) != 1 {
		t.Fatalf("expected 1 SSE event, got %d", len(events))
	}
	if events[0].EventType != "run_completed" {
		t.Errorf("expected event type run_completed, got %s", events[0].EventType)
	}
}

// --- toTransitionDTO tests ---

func TestToTransitionDTO_FieldMapping(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 12, 30, 45, 123456789, time.UTC)
	st := run.StateTransition{
		ID:             42,
		RunID:          "run-99",
		FromState:      "preparing",
		ToState:        "model_running",
		TriggeredBy:    "orchestrator",
		MetadataJSON:   `{"lease_id":"lease-abc"}`,
		TransitionedAt: now,
	}

	dto := toTransitionDTO(st)

	if dto.ID != 42 {
		t.Errorf("expected ID=42, got %d", dto.ID)
	}
	if dto.RunID != "run-99" {
		t.Errorf("expected RunID=run-99, got %s", dto.RunID)
	}
	if dto.FromState != "preparing" {
		t.Errorf("expected FromState=preparing, got %s", dto.FromState)
	}
	if dto.ToState != "model_running" {
		t.Errorf("expected ToState=model_running, got %s", dto.ToState)
	}
	if dto.TriggeredBy != "orchestrator" {
		t.Errorf("expected TriggeredBy=orchestrator, got %s", dto.TriggeredBy)
	}
	if dto.MetadataJSON != `{"lease_id":"lease-abc"}` {
		t.Errorf("expected MetadataJSON with lease_id, got %s", dto.MetadataJSON)
	}
	// Verify timestamp is RFC3339Nano.
	parsed, err := time.Parse(time.RFC3339Nano, dto.TransitionedAt)
	if err != nil {
		t.Fatalf("failed to parse TransitionedAt: %v", err)
	}
	if !parsed.Equal(now) {
		t.Errorf("expected TransitionedAt=%v, got %v", now, parsed)
	}
}
