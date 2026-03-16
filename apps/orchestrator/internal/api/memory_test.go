package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/chunks"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/provenance"
	"github.com/butler/butler/internal/memory/working"
)

type fakeWorkingMemoryReader struct {
	snapshot working.Snapshot
	err      error
}

func (f fakeWorkingMemoryReader) Get(_ context.Context, _ string) (working.Snapshot, error) {
	if f.err != nil {
		return working.Snapshot{}, f.err
	}
	return f.snapshot, nil
}

type fakeProfileMemoryReader struct {
	entries []profile.Entry
	err     error
}

func (f fakeProfileMemoryReader) GetByScope(_ context.Context, _, _ string) ([]profile.Entry, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.entries, nil
}

type fakeEpisodicMemoryReader struct {
	entries []episodic.Episode
	err     error
}

func (f fakeEpisodicMemoryReader) GetByScope(_ context.Context, _, _ string) ([]episodic.Episode, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.entries, nil
}

type fakeChunkMemoryReader struct {
	entries []chunks.Chunk
	err     error
	limit   int
}

func (f *fakeChunkMemoryReader) GetByScope(_ context.Context, _, _ string, limit int) ([]chunks.Chunk, error) {
	f.limit = limit
	if f.err != nil {
		return nil, f.err
	}
	return f.entries, nil
}

type fakeProvenanceLinkReader struct {
	items map[string][]provenance.Link
	err   error
}

func (f fakeProvenanceLinkReader) ListBySource(_ context.Context, sourceMemoryType string, sourceMemoryID int64) ([]provenance.Link, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.items[sourceMemoryType+":"+jsonNumber(sourceMemoryID)], nil
}

func jsonNumber(value int64) string {
	return strconv.FormatInt(value, 10)
}

func TestMemoryHandleListRequiresScope(t *testing.T) {
	t.Parallel()
	server := NewMemoryServer(nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/memory", nil)
	recorder := httptest.NewRecorder()
	server.HandleList().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}

func TestMemoryHandleListReturnsMethodNotAllowed(t *testing.T) {
	t.Parallel()
	server := NewMemoryServer(nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/memory?scope_type=session&scope_id=session-1", nil)
	recorder := httptest.NewRecorder()
	server.HandleList().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", recorder.Code)
	}
}

func TestMemoryHandleListReturnsEmptyScopeView(t *testing.T) {
	t.Parallel()
	server := NewMemoryServer(nil, nil, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/memory?scope_type=session&scope_id=session-1", nil)
	recorder := httptest.NewRecorder()
	server.HandleList().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["scope_type"] != "session" || body["scope_id"] != "session-1" {
		t.Fatalf("unexpected body %+v", body)
	}
	if int(body["limit"].(float64)) != 50 {
		t.Fatalf("expected default limit 50, got %+v", body["limit"])
	}
	for _, key := range []string{"profile", "episodic", "chunks", "working"} {
		if _, ok := body[key]; ok {
			t.Fatalf("did not expect populated %s in %+v", key, body)
		}
	}
}

func TestMemoryHandleListReturnsAllMemoryClasses(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 16, 10, 0, 0, 0, time.UTC)
	chunkReader := &fakeChunkMemoryReader{entries: []chunks.Chunk{{
		ID:             99,
		MemoryType:     chunks.MemoryType,
		ScopeType:      "session",
		ScopeID:        "session-1",
		Title:          "doctor summary",
		Summary:        "Doctor found a redis timeout",
		Content:        "redis ping timed out once",
		SourceType:     "doctor_report",
		SourceID:       "doctor-1",
		ProvenanceJSON: `{"source_type":"doctor_report"}`,
		TagsJSON:       `["doctor"]`,
		Confidence:     0.91,
		Status:         chunks.StatusActive,
		CreatedAt:      now,
		UpdatedAt:      now,
	}}}
	server := NewMemoryServer(
		fakeWorkingMemoryReader{snapshot: working.Snapshot{
			MemoryType:       "working",
			SessionKey:       "session-1",
			RunID:            "run-1",
			Goal:             "finish memory browser",
			EntitiesJSON:     `{"ticket":"TM-12"}`,
			PendingStepsJSON: `["api","ui"]`,
			ScratchJSON:      `{"notes":"safe"}`,
			Status:           "active",
			SourceType:       "run",
			SourceID:         "run-1",
			ProvenanceJSON:   `{"source_type":"run","source_id":"run-1"}`,
			CreatedAt:        now,
			UpdatedAt:        now,
		}},
		fakeProfileMemoryReader{entries: []profile.Entry{{
			ID:             42,
			MemoryType:     profile.MemoryType,
			ScopeType:      "session",
			ScopeID:        "session-1",
			Key:            "language",
			ValueJSON:      `{"value":"ru"}`,
			Summary:        "User prefers Russian",
			SourceType:     "message",
			SourceID:       "msg-1",
			ProvenanceJSON: `{"source_type":"message"}`,
			Confidence:     0.9,
			Status:         profile.StatusActive,
			CreatedAt:      now,
			UpdatedAt:      now,
		}}},
		fakeEpisodicMemoryReader{entries: []episodic.Episode{{
			ID:             7,
			MemoryType:     episodic.MemoryType,
			ScopeType:      "session",
			ScopeID:        "session-1",
			Summary:        "Completed TM-11",
			Content:        "Added memory observability and doctor checks",
			SourceType:     "run",
			SourceID:       "run-11",
			ProvenanceJSON: `{"source_type":"run"}`,
			Confidence:     0.88,
			Status:         episodic.StatusActive,
			TagsJSON:       `["memory","doctor"]`,
			CreatedAt:      now,
			UpdatedAt:      now,
		}}},
		chunkReader,
		fakeProvenanceLinkReader{items: map[string][]provenance.Link{
			"profile:42": {{LinkType: "source", TargetType: "message", TargetID: "msg-1", MetadataJSON: `{"safe":true}`}},
			"episodic:7": {{LinkType: "related", TargetType: "run", TargetID: "run-11", MetadataJSON: `{"status":"completed"}`}},
			"chunk:99":   {{LinkType: "source", TargetType: "doctor_report", TargetID: "doctor-1", MetadataJSON: `{"kind":"memory"}`}},
		}},
	)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/memory?scope_type=session&scope_id=session-1&limit=3", nil)
	recorder := httptest.NewRecorder()
	server.HandleList().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if chunkReader.limit != 3 {
		t.Fatalf("expected chunk limit 3, got %d", chunkReader.limit)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	workingBody := body["working"].(map[string]any)
	if workingBody["goal"] != "finish memory browser" {
		t.Fatalf("unexpected working goal %+v", workingBody)
	}
	if workingBody["entities_json"] != "{\n  \"ticket\": \"TM-12\"\n}" {
		t.Fatalf("expected pretty entities json, got %q", workingBody["entities_json"])
	}
	profileItems := body["profile"].([]any)
	if len(profileItems) != 1 {
		t.Fatalf("expected 1 profile item, got %d", len(profileItems))
	}
	profileItem := profileItems[0].(map[string]any)
	if profileItem["summary"] != "User prefers Russian" {
		t.Fatalf("unexpected profile summary %+v", profileItem)
	}
	if links := profileItem["links"].([]any); len(links) != 1 {
		t.Fatalf("expected profile links, got %+v", profileItem["links"])
	}
	chunkItems := body["chunks"].([]any)
	if len(chunkItems) != 1 {
		t.Fatalf("expected 1 chunk item, got %d", len(chunkItems))
	}
	chunkItem := chunkItems[0].(map[string]any)
	if chunkItem["title"] != "doctor summary" {
		t.Fatalf("unexpected chunk title %+v", chunkItem)
	}
}

func TestMemoryHandleListPropagatesUnexpectedStoreErrors(t *testing.T) {
	t.Parallel()
	server := NewMemoryServer(nil, fakeProfileMemoryReader{err: errors.New("boom")}, nil, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/memory?scope_type=session&scope_id=session-1", nil)
	recorder := httptest.NewRecorder()
	server.HandleList().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", recorder.Code)
	}
	if body := recorder.Body.String(); body == "" || !contains(body, "boom") {
		t.Fatalf("expected error body to mention boom, got %q", body)
	}
}

func TestPrettyJSONFallbacks(t *testing.T) {
	t.Parallel()
	if got := prettyJSON(""); got != "{}" {
		t.Fatalf("expected empty JSON fallback, got %q", got)
	}
	if got := prettyJSON("not-json"); got != "not-json" {
		t.Fatalf("expected raw fallback, got %q", got)
	}
	if got := prettyJSON(`{"a":1}`); got != "{\n  \"a\": 1\n}" {
		t.Fatalf("expected pretty json, got %q", got)
	}
}

func contains(value, needle string) bool {
	return strings.Contains(value, needle)
}
