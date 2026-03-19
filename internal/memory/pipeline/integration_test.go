package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/chunks"
	"github.com/butler/butler/internal/memory/embeddings"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/metrics"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	redisstore "github.com/butler/butler/internal/storage/redis"
)

// fakeSessionSummaryStore implements SessionSummaryWriter and can be queried.
type fakeSessionSummaryStore struct {
	updated map[string]string
}

func (f *fakeSessionSummaryStore) UpdateSummary(_ context.Context, sessionKey, summary string) error {
	if f.updated == nil {
		f.updated = make(map[string]string)
	}
	f.updated[sessionKey] = summary
	return nil
}

// fakeEmbeddingProviderIntegration returns a deterministic 1536-dim vector.
type fakeEmbeddingProviderIntegration struct{}

func (f *fakeEmbeddingProviderIntegration) EmbedQuery(_ context.Context, _ string) ([]float32, error) {
	v := make([]float32, embeddings.VectorDimensions())
	for i := range v {
		v[i] = 0.01 * float32(i%100)
	}
	return v, nil
}

// TestPipelineWorkerIntegration verifies the full async memory pipeline:
// enqueue a post-run job → worker dequeues → reads transcript → extracts →
// classifies → resolves → writes profile + episode + session summary + chunks.
//
// Requires BUTLER_TEST_POSTGRES_URL and BUTLER_TEST_REDIS_URL.
func TestPipelineWorkerIntegration(t *testing.T) {
	postgresURL := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	redisURL := os.Getenv("BUTLER_TEST_REDIS_URL")
	if postgresURL == "" || redisURL == "" {
		t.Skip("set BUTLER_TEST_POSTGRES_URL and BUTLER_TEST_REDIS_URL to run integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// --- Infrastructure ---
	pg, err := postgresstore.Open(ctx, postgresstore.Config{
		URL: postgresURL, MaxConns: 4, MinConns: 1, MaxConnLifetime: time.Minute,
	}, nil)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer pg.Close()

	migrationsDir := filepath.Clean(filepath.Join("..", "..", "..", "migrations"))
	if err := pg.RunMigrations(ctx, migrationsDir); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	redis, err := redisstore.Open(ctx, redisstore.Config{URL: redisURL}, nil)
	if err != nil {
		t.Fatalf("open redis: %v", err)
	}
	defer func() { _ = redis.Close() }()

	// --- Test IDs ---
	sessionKey := "integration:pipeline:session"
	runID := "integration:pipeline:run-1"

	// --- Cleanup ---
	cleanup := func() {
		_, _ = pg.Pool().Exec(context.Background(), `DELETE FROM memory_chunks WHERE scope_type='session' AND scope_id=$1`, sessionKey)
		_, _ = pg.Pool().Exec(context.Background(), `DELETE FROM memory_episodes WHERE scope_type='session' AND scope_id=$1`, sessionKey)
		_, _ = pg.Pool().Exec(context.Background(), `DELETE FROM memory_profile WHERE scope_type='session' AND scope_id=$1`, sessionKey)
		_, _ = pg.Pool().Exec(context.Background(), `DELETE FROM tool_calls WHERE run_id=$1`, runID)
		_, _ = pg.Pool().Exec(context.Background(), `DELETE FROM messages WHERE session_key=$1`, sessionKey)
		// Drain the queue to avoid cross-test pollution.
		for {
			n, _ := redis.Client().LLen(context.Background(), QueueKey).Result()
			if n <= 0 {
				break
			}
			redis.Client().RPop(context.Background(), QueueKey)
		}
	}
	cleanup()
	defer cleanup()

	// --- Seed transcript (the worker reads real transcript from Postgres) ---
	transcriptStore := transcript.NewStore(pg.Pool())

	// We need a session + run row to satisfy FK constraints on messages.
	_, _ = pg.Pool().Exec(ctx, `INSERT INTO sessions (session_key, channel, created_at, updated_at) VALUES ($1, 'test', NOW(), NOW()) ON CONFLICT (session_key) DO NOTHING`, sessionKey)
	_, _ = pg.Pool().Exec(ctx, `INSERT INTO runs (run_id, session_key, current_state, created_at, updated_at) VALUES ($1, $2, 'completed', NOW(), NOW()) ON CONFLICT (run_id) DO NOTHING`, runID, sessionKey)
	defer func() {
		_, _ = pg.Pool().Exec(context.Background(), `DELETE FROM runs WHERE run_id=$1`, runID)
		_, _ = pg.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, sessionKey)
	}()

	if _, err := transcriptStore.AppendMessage(ctx, transcript.Message{
		SessionKey: sessionKey, RunID: runID, Role: "user",
		Content: "My name is Alice and I prefer Go for all backend work",
	}); err != nil {
		t.Fatalf("append user message: %v", err)
	}
	if _, err := transcriptStore.AppendMessage(ctx, transcript.Message{
		SessionKey: sessionKey, RunID: runID, Role: "assistant",
		Content: "Got it, Alice! I will remember your preference for Go on the backend.",
	}); err != nil {
		t.Fatalf("append assistant message: %v", err)
	}

	// --- Build fake extractor that returns deterministic extraction ---
	extractionResult := ExtractionResult{
		ProfileUpdates: []ProfileCandidate{
			{ScopeType: "session", ScopeID: sessionKey, Key: "user_name", Value: `{"value":"Alice"}`, Summary: "User name is Alice", Confidence: 0.95},
			{ScopeType: "session", ScopeID: sessionKey, Key: "backend_language", Value: `{"value":"Go"}`, Summary: "User prefers Go for backend", Confidence: 0.9},
		},
		Episodes: []EpisodeCandidate{
			{ScopeType: "session", ScopeID: sessionKey, Summary: "User introduced herself as Alice and stated Go preference", Content: "Alice said she prefers Go for all backend work", Confidence: 0.85},
		},
		DocumentChunks: []DocumentCandidate{
			{ScopeType: "session", ScopeID: sessionKey, Title: "Go preference note", Content: "Alice prefers Go for backend services", Confidence: 0.8},
		},
		SessionSummary: "Alice introduced herself and stated her preference for Go-based backends.",
	}
	extractionJSON, _ := json.Marshal(extractionResult)
	fakeLLM := &fakeLLMCaller{response: string(extractionJSON)}
	extractor := NewLLMExtractor(fakeLLM)

	// --- Build stores ---
	profileStore := profile.NewStore(pg.Pool())
	episodicStore := episodic.NewStore(pg.Pool())
	chunkStore := chunks.NewStore(pg.Pool())
	summaryStore := &fakeSessionSummaryStore{}
	embeddingProvider := &fakeEmbeddingProviderIntegration{}

	// --- Build queue and enqueue the job ---
	queue := NewQueue(redis.Client())
	if err := queue.Enqueue(ctx, Job{
		JobType:    JobTypePostRun,
		RunID:      runID,
		SessionKey: sessionKey,
	}); err != nil {
		t.Fatalf("enqueue job: %v", err)
	}

	// Verify job is in the queue.
	depth, err := queue.Depth(ctx)
	if err != nil {
		t.Fatalf("queue depth: %v", err)
	}
	if depth != 1 {
		t.Fatalf("expected queue depth 1, got %d", depth)
	}

	// --- Build and run worker ---
	m := metrics.New()
	worker := NewWorker(
		queue,
		transcriptStore,
		extractor,
		profileStore,
		episodicStore,
		embeddingProvider,
		summaryStore,
		WorkerConfig{
			PollTimeout: 2 * time.Second,
			MaxRetries:  1,
		},
		m,
		nil,
	)
	worker.SetChunkStore(chunkStore)

	// Dequeue and process the job directly (not in a loop) to avoid blocking.
	job, err := queue.Dequeue(ctx, 2*time.Second)
	if err != nil {
		t.Fatalf("dequeue job: %v", err)
	}
	if job == nil {
		t.Fatal("expected a job, got nil")
	}
	worker.processJob(ctx, job)

	// --- Verify: profile entries were written ---
	profiles, err := profileStore.GetByScope(ctx, "session", sessionKey)
	if err != nil {
		t.Fatalf("GetByScope returned error: %v", err)
	}
	if len(profiles) < 2 {
		t.Fatalf("expected at least 2 profile entries, got %d", len(profiles))
	}
	profileKeys := map[string]bool{}
	for _, p := range profiles {
		profileKeys[p.Key] = true
		if p.Status != profile.StatusActive {
			t.Errorf("expected active profile, got %q for key %q", p.Status, p.Key)
		}
		if p.SourceType != "memory_pipeline" {
			t.Errorf("expected source_type 'memory_pipeline', got %q", p.SourceType)
		}
	}
	if !profileKeys["user_name"] || !profileKeys["backend_language"] {
		t.Fatalf("expected keys user_name and backend_language, got %v", profileKeys)
	}

	// --- Verify: episodic memory was written ---
	episodes, err := episodicStore.GetByScope(ctx, "session", sessionKey)
	if err != nil {
		t.Fatalf("GetByScope episodes returned error: %v", err)
	}
	if len(episodes) < 1 {
		t.Fatalf("expected at least 1 episode, got %d", len(episodes))
	}
	if episodes[0].SourceType != "memory_pipeline" {
		t.Errorf("expected episode source_type 'memory_pipeline', got %q", episodes[0].SourceType)
	}

	// --- Verify: session summary was written ---
	if summaryStore.updated == nil || summaryStore.updated[sessionKey] == "" {
		t.Fatal("expected session summary to be updated")
	}
	if summaryStore.updated[sessionKey] != "Alice introduced herself and stated her preference for Go-based backends." {
		t.Errorf("unexpected session summary: %q", summaryStore.updated[sessionKey])
	}

	// --- Verify: document chunk was written ---
	chunkResults, err := chunkStore.FindByTitle(ctx, "session", sessionKey, "Go preference note", 5)
	if err != nil {
		t.Fatalf("FindByTitle returned error: %v", err)
	}
	if len(chunkResults) < 1 {
		t.Fatalf("expected at least 1 chunk, got %d", len(chunkResults))
	}

	// --- Verify: queue is empty after processing ---
	postDepth, err := queue.Depth(ctx)
	if err != nil {
		t.Fatalf("queue depth after processing: %v", err)
	}
	if postDepth != 0 {
		t.Fatalf("expected empty queue after processing, got depth %d", postDepth)
	}
}
