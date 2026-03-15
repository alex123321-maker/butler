package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/memory/transcript"
)

// --- fakes ---

type fakeSessionLister struct {
	sessions []session.SessionRecord
	single   *session.SessionRecord
	err      error
}

func (f *fakeSessionLister) ListSessions(_ context.Context, limit, offset int) ([]session.SessionRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sessions, nil
}

func (f *fakeSessionLister) GetSessionByKey(_ context.Context, key string) (session.SessionRecord, error) {
	if f.err != nil {
		return session.SessionRecord{}, f.err
	}
	if f.single != nil {
		return *f.single, nil
	}
	return session.SessionRecord{}, session.ErrSessionNotFound
}

type fakeRunLister struct {
	runs   []run.Record
	single *run.Record
	err    error
}

func (f *fakeRunLister) ListRunsBySessionKey(_ context.Context, sessionKey string) ([]run.Record, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.runs, nil
}

func (f *fakeRunLister) GetRun(_ context.Context, runID string) (run.Record, error) {
	if f.err != nil {
		return run.Record{}, f.err
	}
	if f.single != nil {
		return *f.single, nil
	}
	return run.Record{}, run.ErrRunNotFound
}

type fakeTranscriptReader struct {
	transcript transcript.Transcript
	err        error
}

func (f *fakeTranscriptReader) GetRunTranscript(_ context.Context, runID string) (transcript.Transcript, error) {
	if f.err != nil {
		return transcript.Transcript{}, f.err
	}
	return f.transcript, nil
}

// --- tests ---

func TestHandleListSessions(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	server := NewViewServer(
		&fakeSessionLister{sessions: []session.SessionRecord{
			{SessionKey: "telegram:chat:1", UserID: "user1", Channel: "telegram", CreatedAt: now, UpdatedAt: now},
			{SessionKey: "web:user:2", UserID: "user2", Channel: "web", CreatedAt: now, UpdatedAt: now},
		}},
		&fakeRunLister{},
		&fakeTranscriptReader{},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	rr := httptest.NewRecorder()
	server.HandleListSessions().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	sessions, ok := resp["sessions"].([]any)
	if !ok {
		t.Fatal("expected sessions array in response")
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestHandleListSessionsEmpty(t *testing.T) {
	t.Parallel()

	server := NewViewServer(
		&fakeSessionLister{sessions: nil},
		&fakeRunLister{},
		&fakeTranscriptReader{},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	rr := httptest.NewRecorder()
	server.HandleListSessions().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	sessions := resp["sessions"].([]any)
	if len(sessions) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestHandleGetSession(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	server := NewViewServer(
		&fakeSessionLister{single: &session.SessionRecord{SessionKey: "telegram:chat:1", UserID: "user1", Channel: "telegram", CreatedAt: now, UpdatedAt: now}},
		&fakeRunLister{runs: []run.Record{
			{RunID: "run-1", SessionKey: "telegram:chat:1", Status: "completed", CurrentState: "completed", ModelProvider: "openai", StartedAt: now, UpdatedAt: now},
		}},
		&fakeTranscriptReader{},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/telegram:chat:1", nil)
	rr := httptest.NewRecorder()
	server.HandleGetSession().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	sessionData, ok := resp["session"].(map[string]any)
	if !ok {
		t.Fatal("expected session object in response")
	}
	if sessionData["session_key"] != "telegram:chat:1" {
		t.Fatalf("expected session_key telegram:chat:1, got %v", sessionData["session_key"])
	}
	runs, ok := resp["runs"].([]any)
	if !ok {
		t.Fatal("expected runs array in response")
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
}

func TestHandleGetSessionNotFound(t *testing.T) {
	t.Parallel()

	server := NewViewServer(
		&fakeSessionLister{},
		&fakeRunLister{},
		&fakeTranscriptReader{},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/nonexistent", nil)
	rr := httptest.NewRecorder()
	server.HandleGetSession().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleGetRun(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	server := NewViewServer(
		&fakeSessionLister{},
		&fakeRunLister{single: &run.Record{RunID: "run-1", SessionKey: "telegram:chat:1", Status: "completed", CurrentState: "completed", ModelProvider: "openai", StartedAt: now, UpdatedAt: now}},
		&fakeTranscriptReader{},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1", nil)
	rr := httptest.NewRecorder()
	server.HandleGetRun().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	runData, ok := resp["run"].(map[string]any)
	if !ok {
		t.Fatal("expected run object in response")
	}
	if runData["run_id"] != "run-1" {
		t.Fatalf("expected run_id run-1, got %v", runData["run_id"])
	}
}

func TestHandleGetRunNotFound(t *testing.T) {
	t.Parallel()

	server := NewViewServer(
		&fakeSessionLister{},
		&fakeRunLister{},
		&fakeTranscriptReader{},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/nonexistent", nil)
	rr := httptest.NewRecorder()
	server.HandleGetRun().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandleGetRunTranscript(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	server := NewViewServer(
		&fakeSessionLister{},
		&fakeRunLister{single: &run.Record{RunID: "run-1", SessionKey: "telegram:chat:1", Status: "completed", CurrentState: "completed", ModelProvider: "openai", StartedAt: now, UpdatedAt: now}},
		&fakeTranscriptReader{transcript: transcript.Transcript{
			Messages: []transcript.Message{
				{MessageID: "msg-1", RunID: "run-1", Role: "user", Content: "hello", CreatedAt: now},
				{MessageID: "msg-2", RunID: "run-1", Role: "assistant", Content: "hi there", CreatedAt: now},
			},
			ToolCalls: []transcript.ToolCall{
				{ToolCallID: "tool-1", RunID: "run-1", ToolName: "search", ArgsJSON: `{"q":"test"}`, Status: "completed", RuntimeTarget: "default", StartedAt: now, ResultJSON: `{"result":"ok"}`},
			},
		}},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/transcript", nil)
	rr := httptest.NewRecorder()
	server.HandleGetRunTranscript().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	messages, ok := resp["messages"].([]any)
	if !ok {
		t.Fatal("expected messages array in response")
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	toolCalls, ok := resp["tool_calls"].([]any)
	if !ok {
		t.Fatal("expected tool_calls array in response")
	}
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
}

func TestHandleGetRunTranscriptRunNotFound(t *testing.T) {
	t.Parallel()

	server := NewViewServer(
		&fakeSessionLister{},
		&fakeRunLister{},
		&fakeTranscriptReader{},
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/nonexistent/transcript", nil)
	rr := httptest.NewRecorder()
	server.HandleGetRunTranscript().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestViewEndpointsRejectNonGet(t *testing.T) {
	t.Parallel()

	server := NewViewServer(
		&fakeSessionLister{},
		&fakeRunLister{},
		&fakeTranscriptReader{},
		nil,
	)

	for _, tc := range []struct {
		handler http.Handler
		path    string
	}{
		{server.HandleListSessions(), "/api/v1/sessions"},
		{server.HandleGetSession(), "/api/v1/sessions/foo"},
		{server.HandleGetRun(), "/api/v1/runs/run-1"},
		{server.HandleGetRunTranscript(), "/api/v1/runs/run-1/transcript"},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, nil)
		rr := httptest.NewRecorder()
		tc.handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for POST %s, got %d", tc.path, rr.Code)
		}
	}
}
