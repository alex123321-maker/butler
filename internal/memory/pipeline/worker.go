package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/butler/butler/internal/memory/embeddings"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/memory/transcript"
)

// TranscriptReader reads transcripts for a given run.
type TranscriptReader interface {
	GetRunTranscript(ctx context.Context, runID string) (transcript.Transcript, error)
}

// EmbeddingProvider generates embeddings for text content.
type EmbeddingProvider interface {
	EmbedQuery(ctx context.Context, text string) ([]float32, error)
}

// SessionSummaryWriter persists session summaries.
type SessionSummaryWriter interface {
	UpdateSummary(ctx context.Context, sessionKey, summary string) error
}

// WorkerConfig holds configuration for the memory pipeline worker.
type WorkerConfig struct {
	// PollTimeout is how long to block-wait on the Redis queue per iteration.
	PollTimeout time.Duration
	// MaxRetries is the number of times to retry a failed job before dropping it.
	MaxRetries int
}

// Worker is the async memory pipeline worker that processes post-run jobs.
type Worker struct {
	queue         *Queue
	transcripts   TranscriptReader
	extractor     Extractor
	profileStore  *profile.Store
	episodicStore *episodic.Store
	embeddings    EmbeddingProvider
	summaryWriter SessionSummaryWriter
	config        WorkerConfig
	log           *slog.Logger
}

// NewWorker creates a new memory pipeline worker.
func NewWorker(
	queue *Queue,
	transcripts TranscriptReader,
	extractor Extractor,
	profileStore *profile.Store,
	episodicStore *episodic.Store,
	embeddingProvider EmbeddingProvider,
	summaryWriter SessionSummaryWriter,
	cfg WorkerConfig,
	log *slog.Logger,
) *Worker {
	if log == nil {
		log = slog.Default()
	}
	if cfg.PollTimeout <= 0 {
		cfg.PollTimeout = 5 * time.Second
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	return &Worker{
		queue:         queue,
		transcripts:   transcripts,
		extractor:     extractor,
		profileStore:  profileStore,
		episodicStore: episodicStore,
		embeddings:    embeddingProvider,
		summaryWriter: summaryWriter,
		config:        cfg,
		log:           log,
	}
}

// Run starts the worker loop. It blocks until the context is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	w.log.Info("memory pipeline worker started")
	for {
		select {
		case <-ctx.Done():
			w.log.Info("memory pipeline worker stopping")
			return ctx.Err()
		default:
		}

		job, err := w.queue.Dequeue(ctx, w.config.PollTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.log.Error("dequeue failed", slog.String("error", err.Error()))
			continue
		}
		if job == nil {
			continue
		}

		w.processJob(ctx, job)
	}
}

func (w *Worker) processJob(ctx context.Context, job *Job) {
	jobLog := w.log.With(
		slog.String("job_type", job.JobType),
		slog.String("run_id", job.RunID),
		slog.String("session_key", job.SessionKey),
	)

	jobLog.Info("processing memory pipeline job")
	started := time.Now()

	switch job.JobType {
	case JobTypePostRun:
		if err := w.processPostRun(ctx, jobLog, job); err != nil {
			jobLog.Error("post-run extraction failed",
				slog.String("error", err.Error()),
				slog.Duration("duration", time.Since(started)),
			)
			return
		}
	default:
		jobLog.Warn("unknown job type, skipping")
		return
	}

	jobLog.Info("memory pipeline job completed",
		slog.Duration("duration", time.Since(started)),
	)
}

func (w *Worker) processPostRun(ctx context.Context, log *slog.Logger, job *Job) error {
	// 1. Read the run transcript.
	t, err := w.transcripts.GetRunTranscript(ctx, job.RunID)
	if err != nil {
		return fmt.Errorf("read transcript: %w", err)
	}
	for idx := range t.Messages {
		t.Messages[idx].Content = sanitize.TranscriptMessageContent(t.Messages[idx].Content)
		t.Messages[idx].MetadataJSON = sanitize.TranscriptMetadataJSON(t.Messages[idx].MetadataJSON)
	}
	for idx := range t.ToolCalls {
		t.ToolCalls[idx].ArgsJSON = sanitize.TranscriptToolArgsJSON(t.ToolCalls[idx].ArgsJSON)
		t.ToolCalls[idx].ResultJSON = sanitize.TranscriptToolResultJSON(t.ToolCalls[idx].ResultJSON)
		t.ToolCalls[idx].ErrorJSON = sanitize.TranscriptToolErrorJSON(t.ToolCalls[idx].ErrorJSON)
	}
	if len(t.Messages) == 0 {
		log.Info("empty transcript, skipping extraction")
		return nil
	}

	// 2. Call LLM extractor.
	result, err := w.extractor.Extract(ctx, job.SessionKey, t)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	result = sanitizeExtractionResult(result)

	// 3. Write profile updates (with conflict resolution via Supersede).
	profileCount, err := w.writeProfileUpdates(ctx, log, job, result.ProfileUpdates)
	if err != nil {
		return fmt.Errorf("write profiles: %w", err)
	}

	// 4. Write episodic memories (with embeddings).
	episodeCount, err := w.writeEpisodes(ctx, log, job, result.Episodes)
	if err != nil {
		return fmt.Errorf("write episodes: %w", err)
	}

	// 5. Update session summary.
	if err := w.updateSessionSummary(ctx, log, job, result.SessionSummary); err != nil {
		return fmt.Errorf("update session summary: %w", err)
	}

	log.Info("extraction complete",
		slog.Int("profiles_written", profileCount),
		slog.Int("episodes_written", episodeCount),
		slog.Bool("summary_updated", strings.TrimSpace(result.SessionSummary) != ""),
	)
	return nil
}

func (w *Worker) writeProfileUpdates(ctx context.Context, log *slog.Logger, job *Job, candidates []ProfileCandidate) (int, error) {
	if w.profileStore == nil {
		return 0, nil
	}
	written := 0
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Key) == "" || strings.TrimSpace(candidate.Summary) == "" {
			continue
		}
		if candidate.Confidence < 0.5 {
			log.Debug("skipping low-confidence profile candidate",
				slog.String("key", candidate.Key),
				slog.Float64("confidence", candidate.Confidence),
			)
			continue
		}

		scopeType := normalizeScopeType(candidate.ScopeType)
		scopeID := candidate.ScopeID
		if scopeID == "" {
			scopeID = scopeIDForType(scopeType, job.SessionKey)
		}

		valueJSON := candidate.Value
		if strings.TrimSpace(valueJSON) == "" {
			valueJSON = mustJSON(map[string]string{"value": sanitize.Text(candidate.Summary)})
		} else if !json.Valid([]byte(valueJSON)) {
			valueJSON = mustJSON(map[string]string{"value": sanitize.Text(valueJSON)})
		}
		valueJSON = sanitize.JSON(valueJSON)

		entry := profile.Entry{
			ScopeType:  scopeType,
			ScopeID:    scopeID,
			Key:        candidate.Key,
			ValueJSON:  valueJSON,
			Summary:    sanitize.Text(candidate.Summary),
			SourceType: "memory_pipeline",
			SourceID:   job.RunID,
			Confidence: candidate.Confidence,
		}

		// Conflict resolution: check if an active entry with this key exists.
		existing, err := w.profileStore.Get(ctx, scopeType, scopeID, candidate.Key)
		if err == nil {
			// Supersede the existing entry.
			if _, err := w.profileStore.Supersede(ctx, existing.ID, entry); err != nil {
				log.Warn("supersede profile entry failed",
					slog.String("key", candidate.Key),
					slog.String("error", err.Error()),
				)
				continue
			}
			log.Info("profile entry superseded",
				slog.String("key", candidate.Key),
				slog.String("scope_type", scopeType),
			)
		} else if err == profile.ErrEntryNotFound {
			// Create new entry.
			if _, err := w.profileStore.Save(ctx, entry); err != nil {
				log.Warn("save profile entry failed",
					slog.String("key", candidate.Key),
					slog.String("error", err.Error()),
				)
				continue
			}
			log.Info("profile entry created",
				slog.String("key", candidate.Key),
				slog.String("scope_type", scopeType),
			)
		} else {
			log.Warn("check existing profile entry failed",
				slog.String("key", candidate.Key),
				slog.String("error", err.Error()),
			)
			continue
		}
		written++
	}
	return written, nil
}

func (w *Worker) writeEpisodes(ctx context.Context, log *slog.Logger, job *Job, candidates []EpisodeCandidate) (int, error) {
	if w.episodicStore == nil {
		return 0, nil
	}
	written := 0
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Summary) == "" {
			continue
		}
		if candidate.Confidence < 0.5 {
			log.Debug("skipping low-confidence episode candidate",
				slog.String("summary", truncate(candidate.Summary, 80)),
				slog.Float64("confidence", candidate.Confidence),
			)
			continue
		}

		scopeType := normalizeScopeType(candidate.ScopeType)
		scopeID := candidate.ScopeID
		if scopeID == "" {
			scopeID = scopeIDForType(scopeType, job.SessionKey)
		}

		// Generate embedding for the episode summary.
		var embedding []float32
		if w.embeddings != nil {
			sanitizedSummary := sanitize.Text(candidate.Summary)
			emb, err := w.embeddings.EmbedQuery(ctx, sanitizedSummary)
			if err != nil {
				log.Warn("episode embedding failed, skipping",
					slog.String("summary", truncate(candidate.Summary, 80)),
					slog.String("error", err.Error()),
				)
				continue
			}
			if len(emb) != embeddings.VectorDimensions {
				log.Warn("episode embedding has wrong dimensions, skipping",
					slog.Int("dimensions", len(emb)),
					slog.Int("expected", embeddings.VectorDimensions),
				)
				continue
			}
			embedding = emb
		} else {
			log.Warn("no embedding provider configured, skipping episode",
				slog.String("summary", truncate(candidate.Summary, 80)),
			)
			continue
		}

		now := time.Now().UTC()
		ep := episodic.Episode{
			ScopeType:      scopeType,
			ScopeID:        scopeID,
			Summary:        sanitize.Text(candidate.Summary),
			Content:        sanitize.Text(candidate.Content),
			SourceType:     "memory_pipeline",
			SourceID:       job.RunID,
			Confidence:     candidate.Confidence,
			Embedding:      embedding,
			EpisodeStartAt: &now,
		}

		if _, err := w.episodicStore.Save(ctx, ep); err != nil {
			log.Warn("save episode failed",
				slog.String("summary", truncate(candidate.Summary, 80)),
				slog.String("error", err.Error()),
			)
			continue
		}
		log.Info("episode created",
			slog.String("summary", truncate(candidate.Summary, 80)),
			slog.String("scope_type", scopeType),
		)
		written++
	}
	return written, nil
}

func (w *Worker) updateSessionSummary(ctx context.Context, log *slog.Logger, job *Job, summary string) error {
	summary = strings.TrimSpace(summary)
	summary = sanitize.Text(summary)
	if summary == "" {
		return nil
	}
	if w.summaryWriter == nil {
		log.Debug("no session summary writer configured, skipping")
		return nil
	}
	if err := w.summaryWriter.UpdateSummary(ctx, job.SessionKey, summary); err != nil {
		return fmt.Errorf("write session summary: %w", err)
	}
	log.Info("session summary updated", slog.String("session_key", job.SessionKey))
	return nil
}

func normalizeScopeType(scopeType string) string {
	scopeType = strings.ToLower(strings.TrimSpace(scopeType))
	switch scopeType {
	case "user", "session", "global":
		return scopeType
	default:
		return "session"
	}
}

func scopeIDForType(scopeType, sessionKey string) string {
	switch scopeType {
	case "global":
		return "global"
	default:
		return sessionKey
	}
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func sanitizeExtractionResult(result *ExtractionResult) *ExtractionResult {
	if result == nil {
		return &ExtractionResult{}
	}
	copyResult := &ExtractionResult{
		ProfileUpdates: make([]ProfileCandidate, 0, len(result.ProfileUpdates)),
		Episodes:       make([]EpisodeCandidate, 0, len(result.Episodes)),
		SessionSummary: sanitize.Text(result.SessionSummary),
	}
	for _, candidate := range result.ProfileUpdates {
		candidate.Value = sanitize.JSON(candidate.Value)
		candidate.Summary = sanitize.Text(candidate.Summary)
		candidate.Key = sanitize.Text(candidate.Key)
		copyResult.ProfileUpdates = append(copyResult.ProfileUpdates, candidate)
	}
	for _, candidate := range result.Episodes {
		candidate.Summary = sanitize.Text(candidate.Summary)
		candidate.Content = sanitize.Text(candidate.Content)
		copyResult.Episodes = append(copyResult.Episodes, candidate)
	}
	return copyResult
}
