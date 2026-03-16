package pipeline

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/chunks"
	"github.com/butler/butler/internal/memory/transcript"
)

// --- Fake LLM Caller ---

type fakeLLMCaller struct {
	response string
	err      error
}

func (f *fakeLLMCaller) Complete(_ context.Context, _, _ string) (string, error) {
	return f.response, f.err
}

// --- Fake TranscriptReader ---

type fakeTranscriptReader struct {
	transcripts map[string]transcript.Transcript
}

func (f *fakeTranscriptReader) GetRunTranscript(_ context.Context, runID string) (transcript.Transcript, error) {
	if t, ok := f.transcripts[runID]; ok {
		return t, nil
	}
	return transcript.Transcript{}, nil
}

// --- Fake EmbeddingProvider ---

type fakeEmbeddingProvider struct {
	embedding []float32
	err       error
}

func (f *fakeEmbeddingProvider) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	return f.embedding, f.err
}

// --- Fake ProfileStore (for testing extraction flow) ---

// --- Fake SessionSummaryWriter ---

type fakeSessionSummaryWriter struct {
	summaries map[string]string
}

func (f *fakeSessionSummaryWriter) UpdateSummary(_ context.Context, sessionKey, summary string) error {
	if f.summaries == nil {
		f.summaries = make(map[string]string)
	}
	f.summaries[sessionKey] = summary
	return nil
}

type fakeChunkStore struct {
	chunks []chunks.Chunk
}

func (f *fakeChunkStore) Save(_ context.Context, chunk chunks.Chunk) (chunks.Chunk, error) {
	chunk.ID = int64(len(f.chunks) + 1)
	f.chunks = append(f.chunks, chunk)
	return chunk, nil
}

// --- Tests ---

func TestLLMExtractor_Extract(t *testing.T) {
	response := ExtractionResult{
		ProfileUpdates: []ProfileCandidate{
			{ScopeType: "user", ScopeID: "user-1", Key: "name", Value: "Alice", Summary: "User name is Alice", Confidence: 0.95},
		},
		Episodes: []EpisodeCandidate{
			{ScopeType: "session", ScopeID: "sess-1", Summary: "User asked about weather", Content: "Detailed weather conversation", Confidence: 0.8},
		},
		WorkingUpdates: []WorkingCandidate{{ScopeType: "session", ScopeID: "sess-1", Goal: "Track weather task", Summary: "Continue weather flow", Confidence: 0.7}},
		DocumentChunks: []DocumentCandidate{{ScopeType: "session", ScopeID: "sess-1", Title: "Weather API note", Content: "Endpoint docs", Confidence: 0.8}},
		SessionSummary: "User inquired about weather conditions",
	}
	responseJSON, _ := json.Marshal(response)

	extractor := NewLLMExtractor(&fakeLLMCaller{response: string(responseJSON)})
	tr := transcript.Transcript{
		Messages: []transcript.Message{
			{Role: "user", Content: "What is the weather like?"},
			{Role: "assistant", Content: "It is sunny today."},
		},
	}

	result, err := extractor.Extract(context.Background(), "sess-1", tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ProfileUpdates) != 1 {
		t.Fatalf("expected 1 profile update, got %d", len(result.ProfileUpdates))
	}
	if result.ProfileUpdates[0].Key != "name" {
		t.Errorf("expected key 'name', got %q", result.ProfileUpdates[0].Key)
	}
	if len(result.Episodes) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(result.Episodes))
	}
	if result.SessionSummary == "" || len(result.WorkingUpdates) != 1 || len(result.DocumentChunks) != 1 {
		t.Error("expected non-empty session summary")
	}
}

func TestLLMExtractor_EmptyTranscript(t *testing.T) {
	extractor := NewLLMExtractor(&fakeLLMCaller{response: `{}`})
	result, err := extractor.Extract(context.Background(), "sess-1", transcript.Transcript{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ProfileUpdates) != 0 || len(result.Episodes) != 0 || len(result.WorkingUpdates) != 0 || len(result.DocumentChunks) != 0 {
		t.Error("expected empty results for empty transcript")
	}
}

func TestLLMExtractor_MarkdownFencing(t *testing.T) {
	wrapped := "```json\n" + `{"profile_updates":[],"episodes":[],"session_summary":"test"}` + "\n```"
	extractor := NewLLMExtractor(&fakeLLMCaller{response: wrapped})
	tr := transcript.Transcript{
		Messages: []transcript.Message{{Role: "user", Content: "hello"}},
	}
	result, err := extractor.Extract(context.Background(), "sess-1", tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SessionSummary != "test" {
		t.Errorf("expected session summary 'test', got %q", result.SessionSummary)
	}
}

func TestFormatTranscriptForExtraction(t *testing.T) {
	tr := transcript.Transcript{
		Messages: []transcript.Message{
			{Role: "user", Content: "Hello token=secret-value"},
			{Role: "assistant", Content: "Hi there!"},
		},
		ToolCalls: []transcript.ToolCall{
			{ToolName: "web_search", Status: "completed", ArgsJSON: `{"authorization":"Bearer abc"}`, ResultJSON: `{"cookie":"session=abc"}`, ErrorJSON: `{"password":"secret"}`},
		},
	}
	output := formatTranscriptForExtraction("sess-1", tr)
	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if !contains(output, "Session: sess-1") {
		t.Error("expected session key in output")
	}
	if !contains(output, "[user] Hello") {
		t.Error("expected user message in output")
	}
	if !contains(output, "web_search") {
		t.Error("expected tool call in output")
	}
	for _, secret := range []string{"secret-value", "Bearer abc", "session=abc", `"password":"secret"`} {
		if strings.Contains(output, secret) {
			t.Fatalf("expected secret %q to be redacted in %q", secret, output)
		}
	}
	if !contains(output, "[REDACTED") {
		t.Fatalf("expected redaction markers in %q", output)
	}
}

func TestSanitizeExtractionResult(t *testing.T) {
	t.Parallel()
	result := sanitizeExtractionResult(&ExtractionResult{
		ProfileUpdates: []ProfileCandidate{{Key: "api_token", Value: `{"token":"abc"}`, Summary: "token=abc", Confidence: 0.9}},
		Episodes:       []EpisodeCandidate{{Summary: "password is secret", Content: "cookie value is abc123", Confidence: 0.8}},
		WorkingUpdates: []WorkingCandidate{{Goal: "Bearer abc", Summary: "cookie value is abc123", Confidence: 0.8}},
		DocumentChunks: []DocumentCandidate{{Title: "token", Content: "password is secret", Confidence: 0.8}},
		SessionSummary: "Bearer abc",
	})
	joined := result.ProfileUpdates[0].Value + result.ProfileUpdates[0].Summary + result.Episodes[0].Summary + result.Episodes[0].Content + result.WorkingUpdates[0].Goal + result.DocumentChunks[0].Content + result.SessionSummary
	for _, secret := range []string{"abc123", "secret", "Bearer abc"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("expected secret %q to be redacted in %q", secret, joined)
		}
	}
}

func TestClassifierSeparatesAcceptedAndIgnoredCandidates(t *testing.T) {
	t.Parallel()
	result := (&Classifier{}).Classify("sess-1", &ExtractionResult{
		ProfileUpdates: []ProfileCandidate{{ScopeType: "session", Key: "language", Summary: "prefers Go", Confidence: 0.9}, {ScopeType: "session", Key: "", Summary: "ok", Confidence: 0.9}},
		Episodes:       []EpisodeCandidate{{ScopeType: "session", Summary: "Resolved Redis issue", Content: "Restarted Redis", Confidence: 0.8}, {ScopeType: "session", Summary: "ok", Content: "ok", Confidence: 0.9}},
		WorkingUpdates: []WorkingCandidate{{ScopeType: "session", Goal: "Continue deploy", Summary: "active task", Confidence: 0.8}},
		DocumentChunks: []DocumentCandidate{{ScopeType: "session", Title: "Runbook", Content: "Useful docs", Confidence: 0.8}, {ScopeType: "session", Title: "Low confidence", Content: "skip", Confidence: 0.2}},
		SessionSummary: "Summary",
	})
	if len(result.Profiles) != 1 || len(result.Episodes) != 1 || len(result.Working) != 1 {
		if len(result.Documents) != 1 {
			t.Fatalf("expected document candidate to be classified, got %+v", result)
		}
		t.Fatalf("unexpected classified result %+v", result)
	}
	if len(result.Ignored) == 0 {
		t.Fatal("expected ignored candidates")
	}
}

func TestConflictResolverDeduplicatesCandidates(t *testing.T) {
	t.Parallel()
	classified := ClassificationResult{
		Profiles: []ClassifiedProfile{
			{Candidate: ProfileCandidate{Key: "language", Summary: "Go", Confidence: 0.6}, ScopeType: "session", ScopeID: "sess-1"},
			{Candidate: ProfileCandidate{Key: "language", Summary: "Go preferred", Confidence: 0.9}, ScopeType: "session", ScopeID: "sess-1"},
		},
		Episodes: []ClassifiedEpisode{
			{Candidate: EpisodeCandidate{Summary: "Redis fix", Content: "Restarted redis and verified leases", Confidence: 0.6}, ScopeType: "session", ScopeID: "sess-1"},
			{Candidate: EpisodeCandidate{Summary: "Redis fix", Content: "Restarted redis and verified leases again", Confidence: 0.8}, ScopeType: "session", ScopeID: "sess-1"},
		},
	}
	resolved := (&ConflictResolver{}).Resolve(classified)
	if len(resolved.Profiles) != 1 || len(resolved.Episodes) != 1 {
		t.Fatalf("expected deduplicated candidates, got %+v", resolved)
	}
	if len(resolved.Ignored) == 0 {
		t.Fatal("expected ignored duplicates")
	}
}

func TestConflictResolverPreservesContradictoryProfileVersions(t *testing.T) {
	t.Parallel()
	classified := ClassificationResult{
		Profiles: []ClassifiedProfile{
			{Candidate: ProfileCandidate{Key: "language", Value: `{"value":"ru"}`, Summary: "Prefers Russian", Confidence: 0.6}, ScopeType: "session", ScopeID: "sess-1"},
			{Candidate: ProfileCandidate{Key: "language", Value: `{"value":"en"}`, Summary: "Prefers English", Confidence: 0.9}, ScopeType: "session", ScopeID: "sess-1"},
		},
	}
	resolved := (&ConflictResolver{}).Resolve(classified)
	if len(resolved.Profiles) != 1 {
		t.Fatalf("expected one resolved profile action, got %+v", resolved)
	}
	if resolved.Profiles[0].Action != "supersede_profile" || resolved.Profiles[0].Policy != "higher_confidence_conflict" {
		t.Fatalf("unexpected profile resolution %+v", resolved.Profiles[0])
	}
}

func TestConflictResolverMarksEpisodeVariants(t *testing.T) {
	t.Parallel()
	classified := ClassificationResult{
		Episodes: []ClassifiedEpisode{
			{Candidate: EpisodeCandidate{Summary: "Redis fix", Content: "Restarted redis and checked leases", Confidence: 0.8}, ScopeType: "session", ScopeID: "sess-1", Reason: "accepted_episode_candidate"},
			{Candidate: EpisodeCandidate{Summary: "Redis fix", Content: "Investigated postgres auth mismatch and rotated credentials after network drift", Confidence: 0.9}, ScopeType: "session", ScopeID: "sess-1", Reason: "accepted_episode_candidate"},
		},
	}
	resolved := (&ConflictResolver{}).Resolve(classified)
	if len(resolved.Episodes) != 2 {
		t.Fatalf("expected canonical plus variant episode, got %+v", resolved)
	}
	variantFound := false
	for _, episode := range resolved.Episodes {
		if episode.LinkVariant {
			variantFound = true
			if episode.Action != "create_episode_variant" {
				t.Fatalf("unexpected variant action %+v", episode)
			}
		}
	}
	if !variantFound {
		t.Fatal("expected variant episode link")
	}
}

func TestWriteDocumentChunksPersistsDocumentCandidatesAndDoctorOutput(t *testing.T) {
	t.Parallel()
	worker := &Worker{chunkStore: &fakeChunkStore{}, embeddings: &fakeEmbeddingProvider{embedding: testWorkerVector(0.3)}}
	transcriptValue := transcript.Transcript{ToolCalls: []transcript.ToolCall{{ToolName: "doctor.check_system", ResultJSON: `{"status":"healthy","summary":"all good"}`}}}
	count, err := worker.writeDocumentChunks(context.Background(), nilLogger(), &Job{RunID: "run-1", SessionKey: "session-1"}, []ClassifiedDocument{{Candidate: DocumentCandidate{Title: "Runbook", Content: "Restart Redis and verify leases", Confidence: 0.9}, ScopeType: "session", ScopeID: "session-1"}}, transcriptValue)
	if err != nil {
		t.Fatalf("writeDocumentChunks returned error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected explicit doc + doctor chunk, got %d", count)
	}
	stored := worker.chunkStore.(*fakeChunkStore).chunks
	if len(stored) != 2 || stored[0].Title == "" || stored[1].SourceType != "doctor_report" {
		t.Fatalf("unexpected stored chunks %+v", stored)
	}
}

func testWorkerVector(value float32) []float32 {
	vector := make([]float32, 1536)
	for i := range vector {
		vector[i] = value
	}
	return vector
}

func nilLogger() *slog.Logger { return slog.Default() }

func TestParseExtractionResponse_ValidJSON(t *testing.T) {
	input := `{"profile_updates":[{"key":"test","summary":"test val","confidence":0.9}],"episodes":[],"session_summary":"ok"}`
	result, err := parseExtractionResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ProfileUpdates) != 1 {
		t.Errorf("expected 1 profile update, got %d", len(result.ProfileUpdates))
	}
}

func TestParseExtractionResponse_InvalidJSON(t *testing.T) {
	_, err := parseExtractionResponse("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestNormalizeScopeType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user", "user"},
		{"USER", "user"},
		{"session", "session"},
		{"global", "global"},
		{"unknown", "session"},
		{"", "session"},
	}
	for _, test := range tests {
		result := normalizeScopeType(test.input)
		if result != test.expected {
			t.Errorf("normalizeScopeType(%q) = %q, want %q", test.input, result, test.expected)
		}
	}
}

func TestScopeIDForType(t *testing.T) {
	if got := scopeIDForType("global", "sess-1"); got != "global" {
		t.Errorf("expected 'global', got %q", got)
	}
	if got := scopeIDForType("session", "sess-1"); got != "sess-1" {
		t.Errorf("expected 'sess-1', got %q", got)
	}
	if got := scopeIDForType("user", "sess-1"); got != "sess-1" {
		t.Errorf("expected 'sess-1', got %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
	if got := truncate("hello world", 5); got != "hello..." {
		t.Errorf("expected 'hello...', got %q", got)
	}
}

func TestJobMarshalRoundtrip(t *testing.T) {
	job := Job{
		JobType:    JobTypePostRun,
		RunID:      "run-123",
		SessionKey: "sess-456",
		EnqueuedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded Job
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.JobType != job.JobType {
		t.Errorf("job type mismatch: %q vs %q", decoded.JobType, job.JobType)
	}
	if decoded.RunID != job.RunID {
		t.Errorf("run id mismatch: %q vs %q", decoded.RunID, job.RunID)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsImpl(s, substr))
}

func containsImpl(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
