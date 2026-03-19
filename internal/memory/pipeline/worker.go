package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/butler/butler/internal/memory/chunks"
	"github.com/butler/butler/internal/memory/embeddings"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/metrics"
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

type ChunkStore interface {
	Save(context.Context, chunks.Chunk) (chunks.Chunk, error)
}

type DoctorReportReader interface {
	LatestReport(context.Context) (DoctorIngestionReport, error)
}

type DoctorIngestionReport struct {
	ID         string
	Status     string
	Summary    string
	ReportJSON string
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
	classifier    *Classifier
	resolver      *ConflictResolver
	chunkStore    ChunkStore
	doctorReports DoctorReportReader
	profileStore  *profile.Store
	episodicStore *episodic.Store
	embeddings    EmbeddingProvider
	summaryWriter SessionSummaryWriter
	config        WorkerConfig
	log           *slog.Logger
	metrics       *metrics.Registry
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
	registry *metrics.Registry,
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
		classifier:    NewClassifier(),
		resolver:      NewConflictResolver(),
		chunkStore:    nil,
		doctorReports: nil,
		profileStore:  profileStore,
		episodicStore: episodicStore,
		embeddings:    embeddingProvider,
		summaryWriter: summaryWriter,
		config:        cfg,
		log:           log,
		metrics:       registry,
	}
}

// SetChunkStore sets the chunk store for document chunk persistence.
func (w *Worker) SetChunkStore(store ChunkStore) {
	w.chunkStore = store
}

// SetDoctorReportReader sets the doctor report reader for report ingestion.
func (w *Worker) SetDoctorReportReader(reader DoctorReportReader) {
	w.doctorReports = reader
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
	w.recordMemoryJobMetric(job.JobType, "started")
	started := time.Now()

	switch job.JobType {
	case JobTypePostRun:
		if err := w.processPostRun(ctx, jobLog, job); err != nil {
			w.recordMemoryJobMetric(job.JobType, "error")
			jobLog.Error("post-run extraction failed",
				slog.String("error", err.Error()),
				slog.Duration("duration", time.Since(started)),
			)
			job.Retries++
			if job.Retries <= w.config.MaxRetries {
				if enqErr := w.queue.Enqueue(context.Background(), *job); enqErr != nil {
					jobLog.Error("failed to re-enqueue job", slog.String("error", enqErr.Error()))
				} else {
					jobLog.Info("job re-enqueued", slog.Int("retries", job.Retries))
				}
			} else {
				jobLog.Error("max retries exceeded, dropping job")
			}
			return
		}
	default:
		jobLog.Warn("unknown job type, skipping")
		return
	}

	jobLog.Info("memory pipeline job completed",
		slog.Duration("duration", time.Since(started)),
	)
	w.recordMemoryJobMetric(job.JobType, "completed")
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

	classified := w.classifier.Classify(job.SessionKey, result)
	resolved := w.resolver.Resolve(classified)
	log.Info("memory pipeline classification complete",
		slog.Int("profile_candidates", len(classified.Profiles)),
		slog.Int("episode_candidates", len(classified.Episodes)),
		slog.Int("working_candidates", len(classified.Working)),
		slog.Int("document_candidates", len(classified.Documents)),
		slog.Int("ignored_candidates", len(resolved.Ignored)),
	)

	// 3. Write profile updates (with conflict resolution via Supersede).
	profileCount, err := w.writeProfileUpdates(ctx, log, job, resolved.Profiles)
	if err != nil {
		return fmt.Errorf("write profiles: %w", err)
	}

	// 4. Write episodic memories (with embeddings).
	episodeCount, err := w.writeEpisodes(ctx, log, job, resolved.Episodes)
	if err != nil {
		return fmt.Errorf("write episodes: %w", err)
	}

	// 5. Update session summary.
	if err := w.updateSessionSummary(ctx, log, job, resolved.Summary); err != nil {
		return fmt.Errorf("update session summary: %w", err)
	}

	chunkCount, err := w.writeDocumentChunks(ctx, log, job, classified.Documents, t)
	if err != nil {
		return fmt.Errorf("write chunks: %w", err)
	}

	log.Info("extraction complete",
		slog.Int("profiles_written", profileCount),
		slog.Int("episodes_written", episodeCount),
		slog.Int("chunks_written", chunkCount),
		slog.Bool("summary_updated", strings.TrimSpace(resolved.Summary) != ""),
	)
	w.recordMemoryWriteMetric("profile", profileCount)
	w.recordMemoryWriteMetric("episodic", episodeCount)
	w.recordMemoryWriteMetric("chunk", chunkCount)
	w.recordMemoryQueueDepth(ctx)
	return nil
}

func (w *Worker) writeProfileUpdates(ctx context.Context, log *slog.Logger, job *Job, candidates []ResolvedProfile) (int, error) {
	if w.profileStore == nil {
		return 0, nil
	}
	written := 0
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Candidate.Key) == "" || strings.TrimSpace(candidate.Candidate.Summary) == "" {
			continue
		}

		scopeType := candidate.ScopeType
		scopeID := candidate.ScopeID

		valueJSON := candidate.Candidate.Value
		if strings.TrimSpace(valueJSON) == "" {
			valueJSON = mustJSON(map[string]string{"value": sanitize.Text(candidate.Candidate.Summary)})
		} else if !json.Valid([]byte(valueJSON)) {
			valueJSON = mustJSON(map[string]string{"value": sanitize.Text(valueJSON)})
		}
		valueJSON = sanitize.JSON(valueJSON)

		entry := profile.Entry{
			ScopeType:  scopeType,
			ScopeID:    scopeID,
			Key:        candidate.Candidate.Key,
			ValueJSON:  valueJSON,
			Summary:    sanitize.Text(candidate.Candidate.Summary),
			SourceType: "memory_pipeline",
			SourceID:   job.RunID,
			Confidence: candidate.Candidate.Confidence,
		}

		// Conflict resolution: check if an active entry with this key exists.
		existing, err := w.profileStore.Get(ctx, scopeType, scopeID, candidate.Candidate.Key)
		if err == nil {
			// Supersede the existing entry.
			if candidate.Policy == "same_value_higher_confidence" {
				log.Info("profile entry reaffirmed",
					slog.String("key", candidate.Candidate.Key),
					slog.String("scope_type", scopeType),
					slog.String("resolution_action", candidate.Action),
				)
				written++
				continue
			}
			if _, err := w.profileStore.Supersede(ctx, existing.ID, entry); err != nil {
				log.Warn("supersede profile entry failed",
					slog.String("key", candidate.Candidate.Key),
					slog.String("error", err.Error()),
				)
				continue
			}
			log.Info("profile entry superseded",
				slog.String("key", candidate.Candidate.Key),
				slog.String("scope_type", scopeType),
				slog.String("resolution_action", candidate.Action),
				slog.String("resolution_policy", candidate.Policy),
			)
		} else if err == profile.ErrEntryNotFound {
			// Create new entry.
			if _, err := w.profileStore.Save(ctx, entry); err != nil {
				log.Warn("save profile entry failed",
					slog.String("key", candidate.Candidate.Key),
					slog.String("error", err.Error()),
				)
				continue
			}
			log.Info("profile entry created",
				slog.String("key", candidate.Candidate.Key),
				slog.String("scope_type", scopeType),
				slog.String("resolution_action", candidate.Action),
				slog.String("resolution_policy", candidate.Policy),
			)
		} else {
			log.Warn("check existing profile entry failed",
				slog.String("key", candidate.Candidate.Key),
				slog.String("error", err.Error()),
			)
			continue
		}
		written++
	}
	return written, nil
}

func (w *Worker) writeEpisodes(ctx context.Context, log *slog.Logger, job *Job, candidates []ResolvedEpisode) (int, error) {
	if w.episodicStore == nil {
		return 0, nil
	}
	written := 0
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Candidate.Summary) == "" {
			continue
		}

		scopeType := candidate.ScopeType
		scopeID := candidate.ScopeID

		// Generate embedding for the episode summary.
		var embedding []float32
		if w.embeddings != nil {
			sanitizedSummary := sanitize.Text(candidate.Candidate.Summary)
			emb, err := w.embeddings.EmbedQuery(ctx, sanitizedSummary)
			if err != nil {
				log.Warn("episode embedding failed, skipping",
					slog.String("summary", truncate(candidate.Candidate.Summary, 80)),
					slog.String("error", err.Error()),
				)
				continue
			}
			if len(emb) != embeddings.VectorDimensions() {
				log.Warn("episode embedding has wrong dimensions, skipping",
					slog.Int("dimensions", len(emb)),
					slog.Int("expected", embeddings.VectorDimensions()),
				)
				continue
			}
			embedding = emb
		} else {
			log.Warn("no embedding provider configured, skipping episode",
				slog.String("summary", truncate(candidate.Candidate.Summary, 80)),
			)
			continue
		}

		now := time.Now().UTC()
		ep := episodic.Episode{
			ScopeType:      scopeType,
			ScopeID:        scopeID,
			Summary:        sanitize.Text(candidate.Candidate.Summary),
			Content:        sanitize.Text(candidate.Candidate.Content),
			SourceType:     "memory_pipeline",
			SourceID:       job.RunID,
			Confidence:     candidate.Candidate.Confidence,
			Embedding:      embedding,
			EpisodeStartAt: &now,
		}

		if _, err := w.episodicStore.Save(ctx, ep); err != nil {
			log.Warn("save episode failed",
				slog.String("summary", truncate(candidate.Candidate.Summary, 80)),
				slog.String("error", err.Error()),
			)
			continue
		}
		log.Info("episode created",
			slog.String("summary", truncate(candidate.Candidate.Summary, 80)),
			slog.String("scope_type", scopeType),
			slog.String("resolution_action", candidate.Action),
			slog.Bool("variant_link", candidate.LinkVariant),
		)
		if candidate.LinkVariant {
			matches, matchErr := w.episodicStore.FindBySummary(ctx, scopeType, scopeID, candidate.CanonicalRef)
			if matchErr == nil && len(matches) > 0 {
				log.Info("episode variant linked",
					slog.String("summary", truncate(candidate.Candidate.Summary, 80)),
					slog.String("canonical_ref", truncate(candidate.CanonicalRef, 80)),
				)
			}
		}
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

func (w *Worker) writeDocumentChunks(ctx context.Context, log *slog.Logger, job *Job, candidates []ClassifiedDocument, t transcript.Transcript) (int, error) {
	if w.chunkStore == nil {
		return 0, nil
	}
	written := 0
	for _, candidate := range candidates {
		content := strings.TrimSpace(candidate.Candidate.Content)
		if content == "" {
			continue
		}
		embedding, err := w.embedChunkContent(ctx, candidate.Candidate.Title, content)
		if err != nil {
			log.Warn("chunk embedding failed", slog.String("title", candidate.Candidate.Title), slog.String("error", err.Error()))
			continue
		}
		if _, err := w.chunkStore.Save(ctx, chunks.Chunk{
			ScopeType:      candidate.ScopeType,
			ScopeID:        candidate.ScopeID,
			Title:          sanitize.Text(candidate.Candidate.Title),
			Content:        sanitize.Text(content),
			Summary:        sanitize.Text(candidate.Candidate.Content),
			SourceType:     "memory_pipeline",
			SourceID:       job.RunID,
			ProvenanceJSON: mustJSON(map[string]any{"source_type": "memory_pipeline", "source_id": job.RunID}),
			TagsJSON:       tagsForChunk(candidate.Candidate.Title, t),
			Confidence:     candidate.Candidate.Confidence,
			Embedding:      embedding,
		}); err != nil {
			log.Warn("save chunk failed", slog.String("title", candidate.Candidate.Title), slog.String("error", err.Error()))
			continue
		}
		written++
	}
	if doctorChunk, ok := doctorChunkCandidate(job.SessionKey, t); ok {
		embedding, err := w.embedChunkContent(ctx, doctorChunk.Title, doctorChunk.Content)
		if err == nil {
			if _, err := w.chunkStore.Save(ctx, chunks.Chunk{ScopeType: "session", ScopeID: job.SessionKey, Title: doctorChunk.Title, Content: doctorChunk.Content, Summary: doctorChunk.Summary, SourceType: doctorChunk.SourceType, SourceID: doctorChunk.SourceID, TagsJSON: doctorChunk.TagsJSON, Confidence: 0.9, Embedding: embedding}); err == nil {
				written++
			}
		}
	}
	return written, nil
}

func (w *Worker) embedChunkContent(ctx context.Context, title, content string) ([]float32, error) {
	if w.embeddings == nil {
		return nil, fmt.Errorf("embedding provider is not configured")
	}
	vector, err := w.embeddings.EmbedQuery(ctx, sanitize.Text(strings.TrimSpace(title+"\n"+content)))
	if err != nil {
		return nil, err
	}
	if len(vector) != embeddings.VectorDimensions() {
		return nil, fmt.Errorf("embedding must contain %d dimensions", embeddings.VectorDimensions())
	}
	return vector, nil
}

func tagsForChunk(title string, t transcript.Transcript) string {
	tags := []string{"document"}
	lowered := strings.ToLower(strings.TrimSpace(title))
	if strings.Contains(lowered, "doctor") {
		tags = append(tags, "doctor")
	}
	for _, call := range t.ToolCalls {
		tool := strings.ToLower(strings.TrimSpace(call.ToolName))
		if tool == "" {
			continue
		}
		if strings.Contains(strings.ToLower(call.ResultJSON), lowered) {
			tags = append(tags, tool)
		}
	}
	return mustJSON(tags)
}

type syntheticChunkCandidate struct {
	Title      string
	Content    string
	Summary    string
	SourceType string
	SourceID   string
	TagsJSON   string
}

func doctorChunkCandidate(sessionKey string, t transcript.Transcript) (syntheticChunkCandidate, bool) {
	for _, call := range t.ToolCalls {
		if !strings.EqualFold(strings.TrimSpace(call.ToolName), "doctor.check_system") {
			continue
		}
		content := sanitize.TranscriptToolResultJSON(call.ResultJSON)
		if strings.TrimSpace(content) == "{}" || strings.TrimSpace(content) == "" {
			continue
		}
		return syntheticChunkCandidate{Title: "Doctor report", Content: content, Summary: "Doctor report summary", SourceType: "doctor_report", SourceID: sessionKey, TagsJSON: `["doctor","tool_output"]`}, true
	}
	return syntheticChunkCandidate{}, false
}

func (w *Worker) recordMemoryJobMetric(jobType, status string) {
	if w.metrics == nil {
		return
	}
	_ = w.metrics.IncrCounter(metrics.MetricMemoryJobsTotal, map[string]string{"job_type": jobType, "service": "memory-pipeline", "status": status})
}

func (w *Worker) recordMemoryWriteMetric(memoryType string, count int) {
	if w.metrics == nil || count <= 0 {
		return
	}
	for i := 0; i < count; i++ {
		_ = w.metrics.IncrCounter(metrics.MetricMemoryWritesTotal, map[string]string{"memory_type": memoryType, "service": "memory-pipeline", "status": "success"})
	}
}

func (w *Worker) recordMemoryQueueDepth(ctx context.Context) {
	if w.metrics == nil || w.queue == nil {
		return
	}
	if depth, err := w.queue.Depth(ctx); err == nil {
		_ = w.metrics.SetGauge(metrics.MetricMemoryQueueDepth, float64(depth), map[string]string{"queue": QueueKey, "service": "memory-pipeline"})
	}
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
		WorkingUpdates: make([]WorkingCandidate, 0, len(result.WorkingUpdates)),
		DocumentChunks: make([]DocumentCandidate, 0, len(result.DocumentChunks)),
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
	for _, candidate := range result.WorkingUpdates {
		candidate.Goal = sanitize.Text(candidate.Goal)
		candidate.Summary = sanitize.Text(candidate.Summary)
		copyResult.WorkingUpdates = append(copyResult.WorkingUpdates, candidate)
	}
	for _, candidate := range result.DocumentChunks {
		candidate.Title = sanitize.Text(candidate.Title)
		candidate.Content = sanitize.Text(candidate.Content)
		copyResult.DocumentChunks = append(copyResult.DocumentChunks, candidate)
	}
	return copyResult
}
