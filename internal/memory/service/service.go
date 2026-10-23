package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/butler/butler/internal/logger"
	memorydiag "github.com/butler/butler/internal/memory/diagnostics"
	"github.com/butler/butler/internal/memory/embeddings"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/metrics"
	"github.com/butler/butler/internal/security/content"
)

type ProfileStore interface {
	GetByScope(context.Context, string, string) ([]ProfileEntry, error)
}

type EpisodeStore interface {
	Search(context.Context, string, string, []float32, int) ([]Episode, error)
	FindBySummary(context.Context, string, string, string) ([]Episode, error)
}

type WorkingStore interface {
	Get(context.Context, string) (WorkingSnapshot, error)
}

type ChunkStore interface {
	Search(context.Context, string, string, []float32, int) ([]Chunk, error)
	FindByTitle(context.Context, string, string, string, int) ([]Chunk, error)
}

type SummaryReader interface {
	GetSummary(context.Context, string) (string, error)
}

type EmbeddingProvider interface {
	EmbedQuery(context.Context, string) ([]float32, error)
}

// SemanticReader is an optional separately versioned local retrieval path.
// Its caller must provide trusted admission; retrieved text is evidence, never
// an authorization or a replacement for current structured owner constraints.
type SemanticReader interface {
	Search(context.Context, []Scope, string, int) ([]SemanticHit, error)
}

// QueryPolicy governs use of the query, independently of provider data release.
// Local allows PostgreSQL and a proven internal inference backend. External
// permits the legacy configured embedding path; source/run release guards still
// govern every result and generation request.
type QueryPolicy struct{ Local, External bool }

type SearchResult struct {
	Hits        []SemanticHit
	Diagnostics []string
	// Corpus is empty/present only when established by an authorized read.
	Corpus string
}
type PolicySemanticReader interface {
	SearchWithPolicy(context.Context, []Scope, string, int, QueryPolicy) (SearchResult, error)
}
type SemanticHit struct {
	Title, Summary                                                   string
	Distance                                                         float64
	SourceRef, OriginType, OriginID, Revision, GenerationID, SpaceID string
	Origin                                                           content.Origin
	ExcerptClipped                                                   bool
}

func (h SemanticHit) MemoryOrigin() content.Origin {
	return content.Derive(content.MemoryDerived, h.Origin)
}

type ProfileEntry interface {
	ProfileKey() string
	ProfileSummary() string
}

type Episode interface {
	EpisodeSummary() string
	EpisodeDistance() float64
}

type WorkingSnapshot struct {
	Goal             string
	EntitiesJSON     string
	PendingStepsJSON string
	ScratchJSON      string
	Status           string
}

var ErrWorkingNotFound = errors.New("working memory snapshot not found")

type Chunk interface {
	ChunkTitle() string
	ChunkSummary() string
	ChunkDistance() float64
}

type Config struct {
	ProfileStore        ProfileStore
	EpisodeStore        EpisodeStore
	ChunkStore          ChunkStore
	WorkingStore        WorkingStore
	SummaryReader       SummaryReader
	Embeddings          EmbeddingProvider
	Semantic            SemanticReader
	ProfileLimit        int
	EpisodeLimit        int
	ChunkLimit          int
	BundleBudget        int
	ContextBudgetTokens int
	KeywordLimit        int
	ScopeOrder          []string
	// ScopeResolver is trusted composition derived from current task ownership
	// and exact owner grants; request/model text cannot populate extra scopes.
	ScopeResolver func(context.Context) ([]Scope, error)
	Log           *slog.Logger
	Metrics       *metrics.Registry
}

type Service struct {
	config Config
	log    *slog.Logger
}

type BundleRequest struct {
	SessionKey  string
	UserID      string
	UserMessage string
	// ProfileTaskGoal is an experimental ranking hint from current typed task
	// state. Empty preserves the production policy. It never grants scope or
	// instruction authority and is not sent to an embedding backend.
	ProfileTaskGoal string
	IncludeQuery    bool
	// QueryPolicy supersedes IncludeQuery. Nil preserves explicit legacy callers.
	QueryPolicy *QueryPolicy
	// Adaptive Broker reads replace optional episode/document lookup, never
	// current scoped accepted preferences or mandatory working/history state.
	SkipOptionalSearch bool
}

type Bundle struct {
	Items       map[string]any
	Prompt      string
	Working     WorkingContext
	Diagnostics SelectionDiagnostics
}

type Scope struct {
	Type string
	ID   string
}

type WorkingContext struct {
	Goal           string
	ActiveEntities any
	PendingSteps   any
	Scratch        map[string]any
	Status         string
}

type episodeItem struct {
	Summary  string
	Distance float64
	Source   string
	Origin   content.Origin
}

func New(cfg Config) *Service {
	if cfg.ProfileLimit <= 0 {
		cfg.ProfileLimit = 20
	}
	if cfg.EpisodeLimit <= 0 {
		cfg.EpisodeLimit = 3
	}
	if cfg.BundleBudget <= 0 {
		cfg.BundleBudget = 12
	}
	if cfg.ChunkLimit <= 0 {
		cfg.ChunkLimit = 4
	}
	if cfg.ContextBudgetTokens <= 0 {
		cfg.ContextBudgetTokens = 65536
	}
	if cfg.KeywordLimit <= 0 {
		cfg.KeywordLimit = 2
	}
	if len(cfg.ScopeOrder) == 0 {
		cfg.ScopeOrder = []string{"session", "user", "global"}
	}
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	return &Service{config: cfg, log: logger.WithComponent(log, "memory-bundle")}
}

func (s *Service) BuildBundle(ctx context.Context, req BundleRequest) (bundle Bundle, buildErr error) {
	observations := []OperationObservation{}
	ctx = context.WithValue(ctx, observationKey{}, &observations)
	defer func() { bundle.Diagnostics.Operations = append([]OperationObservation(nil), observations...) }()
	started := time.Now()
	defer func() { s.observeOperation(ctx, "bundle", started, buildErr, "success", nil) }()
	searchStarted := time.Now()
	searchFinished := false
	defer func() {
		if !searchFinished {
			s.observeOperation(ctx, "search", searchStarted, buildErr, "unknown", nil)
		}
	}()
	req.UserMessage = sanitize.Text(req.UserMessage)
	scopes := s.scopes(req.SessionKey, req.UserID)
	if s.config.ScopeResolver != nil {
		extra, e := s.config.ScopeResolver(ctx)
		if e != nil {
			return Bundle{}, e
		}
		if len(extra) > 9 {
			return Bundle{}, fmt.Errorf("too many current memory scopes")
		}
		seen := map[Scope]bool{}
		combined := make([]Scope, 0, len(scopes)+len(extra))
		for _, scope := range append(extra, scopes...) {
			if !seen[scope] {
				combined = append(combined, scope)
				seen[scope] = true
			}
		}
		scopes = combined
	}
	policy := QueryPolicy{Local: req.IncludeQuery, External: req.IncludeQuery}
	if req.QueryPolicy != nil {
		policy = *req.QueryPolicy
	}
	working, err := s.loadWorking(ctx, req.SessionKey)
	if err != nil {
		return Bundle{}, err
	}
	// Legacy query embeddings have no locality proof. Local-only requests use
	// the policy-aware reader and never fall through to this backend.
	var episodes, chunks []map[string]any
	var diagnostics []string
	if !req.SkipOptionalSearch {
		episodes, err = s.loadEpisodes(ctx, scopes, req.UserMessage, policy.External)
		if err != nil {
			return Bundle{}, err
		}
		chunks, diagnostics, err = s.loadPolicyChunks(ctx, scopes, req.UserMessage, policy)
		if err != nil {
			return Bundle{}, err
		}
	} else {
		diagnostics = []string{"adaptive_broker_reads"}
	}
	// Read profile eligibility after potentially slow retrieval, not before it.
	profileEntries, filteredProfiles, err := s.loadProfile(ctx, scopes, req.UserMessage, sanitize.Text(req.ProfileTaskGoal))
	if err != nil {
		return Bundle{}, err
	}
	summary := s.loadSummary(ctx, req.SessionKey)
	found := len(profileEntries) + filteredProfiles + len(episodes) + len(chunks)
	s.observeOperation(ctx, "search", searchStarted, nil, memorydiag.SearchOutcome(found, "unknown", diagnostics), diagnostics)
	searchFinished = true
	selectionStarted := time.Now()
	items, promptProfile, promptEpisodes, promptChunks, selection, err := s.selectContext(summary, working, profileEntries, episodes, chunks)
	s.observeOperation(ctx, "context", selectionStarted, err, "success", nil)
	if err != nil {
		return Bundle{}, err
	}
	selection.Retrieval = diagnostics
	if strings.TrimSpace(req.ProfileTaskGoal) != "" {
		selection.Version = "memory-context-v3-task-goal-tiebreak-experiment-v1"
	}
	selection.Omitted["profile_relevance_or_limit"] = filteredProfiles
	s.observeSelection(found, selection)
	s.recordRetrievalMetric("profile", len(promptProfile) > 0)
	s.recordRetrievalMetric("episodic", len(promptEpisodes) > 0)
	s.recordRetrievalMetric("chunk", len(promptChunks) > 0)

	return Bundle{Items: items, Prompt: FormatPrompt(working, promptProfile, promptEpisodes, promptChunks, summary), Working: working, Diagnostics: selection}, nil
}

func (s *Service) scopes(sessionKey, userID string) []Scope {
	ids := map[string]string{"session": strings.TrimSpace(sessionKey), "user": strings.TrimSpace(userID), "global": "global"}
	seen := map[string]struct{}{}
	result := make([]Scope, 0, len(s.config.ScopeOrder))
	for _, scopeType := range s.config.ScopeOrder {
		scopeType = strings.ToLower(strings.TrimSpace(scopeType))
		scopeID := ids[scopeType]
		if scopeType == "" || scopeID == "" {
			continue
		}
		key := scopeType + ":" + scopeID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, Scope{Type: scopeType, ID: scopeID})
	}
	return result
}

func (s *Service) loadProfile(ctx context.Context, scopes []Scope, query, taskGoal string) (items []map[string]any, omitted int, loadErr error) {
	started := time.Now()
	outcome := "empty"
	restricted := false
	if len(scopes) == 0 {
		outcome = "unknown"
	}
	defer func() {
		var reasons []string
		if len(items)+omitted > 0 {
			outcome = "success"
		}
		if restricted {
			reasons = []string{"access"}
			if outcome == "empty" {
				outcome = "unknown"
			}
		}
		s.observeOperation(ctx, "profile_load", started, loadErr, outcome, reasons)
	}()
	if s.config.ProfileStore == nil || s.config.ProfileLimit <= 0 {
		outcome = "disabled"
		return nil, 0, nil
	}
	result := make([]map[string]any, 0, s.config.ProfileLimit)
	for _, scope := range scopes {
		entries, err := s.config.ProfileStore.GetByScope(ctx, scope.Type, scope.ID)
		if err != nil {
			return nil, 0, fmt.Errorf("load profile memory: %w", err)
		}
		for _, entry := range entries {
			origin := memoryOrigin(entry)
			if origin.Sensitivity == content.Secret {
				restricted = true
				continue
			}
			result = append(result, map[string]any{"key": entry.ProfileKey(), "summary": entry.ProfileSummary(), "scope_type": scope.Type, "scope_id": scope.ID, "origin": origin, "source_refs": origin.SourceRefs})
		}
	}
	selected := relevantProfilesForTask(result, query, taskGoal, s.config.ProfileLimit)
	return selected, len(result) - len(selected), nil
}

func (s *Service) loadPolicyChunks(ctx context.Context, scopes []Scope, query string, policy QueryPolicy) (items []map[string]any, reasons []string, searchErr error) {
	started := time.Now()
	corpus := "unknown"
	defer func() {
		s.observeOperation(ctx, "document_search", started, searchErr, memorydiag.SearchOutcome(len(items), corpus, reasons), reasons)
	}()
	if strings.TrimSpace(query) == "" || !policy.Local && !policy.External {
		return nil, []string{"query_search_disabled"}, nil
	}
	if reader, ok := s.config.Semantic.(PolicySemanticReader); ok && policy.Local {
		result, err := reader.SearchWithPolicy(ctx, scopes, query, min(4, s.config.ChunkLimit), policy)
		corpus = result.Corpus
		if err != nil {
			return nil, nil, fmt.Errorf("memory retrieval failed: %w", err)
		}
		return semanticItems(result.Hits), result.Diagnostics, nil
	}
	if !policy.External {
		return nil, []string{"local_query_backend_unavailable"}, nil
	}
	items, err := s.loadChunks(ctx, scopes, query, true)
	return items, nil, err
}

func semanticItems(hits []SemanticHit) []map[string]any {
	items := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		if h.MemoryOrigin().Sensitivity == content.Secret {
			continue
		}
		items = append(items, map[string]any{"title": h.Title, "summary": h.Summary, "distance": h.Distance, "source": "local_semantic", "source_ref": h.SourceRef, "origin_type": h.OriginType, "origin_id": h.OriginID, "source_revision": h.Revision, "index_generation": h.GenerationID, "index_space": h.SpaceID, "origin": h.MemoryOrigin(), "excerpt_clipped": h.ExcerptClipped})
	}
	return items
}

func (s *Service) loadEpisodes(ctx context.Context, scopes []Scope, userMessage string, includeQuery bool) (resultItems []map[string]any, loadErr error) {
	started := time.Now()
	outcome := "unknown"
	var observedErr error
	var reasons []string
	defer func() {
		if len(resultItems) > 0 {
			outcome = "success"
		}
		if loadErr != nil {
			observedErr = loadErr
		}
		s.observeOperation(ctx, "episodic_search", started, observedErr, outcome, reasons)
	}()
	if !includeQuery || s.config.EpisodeStore == nil || s.config.EpisodeLimit <= 0 || strings.TrimSpace(userMessage) == "" {
		outcome = "disabled"
		if !includeQuery {
			reasons = []string{"query_search_disabled"}
		}
		return nil, nil
	}
	if isNilEmbeddingProvider(s.config.Embeddings) {
		outcome = "disabled"
		reasons = []string{"embedding_unavailable"}
		s.log.Info("episodic retrieval skipped; embedding provider is not configured")
		return nil, nil
	}
	queryEmbedding, err := s.config.Embeddings.EmbedQuery(ctx, userMessage)
	if err != nil {
		observedErr = err
		s.log.Warn("episodic retrieval skipped; embedding query failed", slog.String("error_class", fmt.Sprintf("%T", err)))
		return nil, nil
	}
	if len(queryEmbedding) != embeddings.VectorDimensions() {
		observedErr = errors.New("invalid embedding dimensions")
		s.log.Warn("episodic retrieval skipped; embedding dimensions are invalid", slog.Int("dimensions", len(queryEmbedding)))
		return nil, nil
	}
	items := make([]episodeItem, 0, s.config.EpisodeLimit)
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		results, err := s.config.EpisodeStore.Search(ctx, scope.Type, scope.ID, queryEmbedding, s.config.EpisodeLimit)
		if err != nil {
			return nil, fmt.Errorf("load episodic memory: %w", err)
		}
		for _, item := range results {
			key := strings.ToLower(strings.TrimSpace(item.EpisodeSummary()))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			origin := memoryOrigin(item)
			if origin.Sensitivity == content.Secret {
				continue
			}
			items = append(items, episodeItem{Summary: item.EpisodeSummary(), Distance: item.EpisodeDistance(), Source: "vector", Origin: origin})
		}
		keywordMatches, keywordErr := s.loadKeywordEpisodes(ctx, scope, userMessage)
		if keywordErr != nil {
			reasons = append(reasons, "backend")
			s.log.Warn("episodic keyword retrieval skipped", slog.String("scope_type", scope.Type), slog.String("error_class", fmt.Sprintf("%T", keywordErr)))
			continue
		}
		for _, item := range keywordMatches {
			key := strings.ToLower(strings.TrimSpace(item.Summary))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Distance == items[j].Distance {
			return items[i].Source < items[j].Source
		}
		return items[i].Distance < items[j].Distance
	})
	if len(items) > s.config.EpisodeLimit {
		items = items[:s.config.EpisodeLimit]
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{"summary": item.Summary, "distance": item.Distance, "source": item.Source, "origin": item.Origin})
	}
	return result, nil
}

func (s *Service) loadKeywordEpisodes(ctx context.Context, scope Scope, userMessage string) ([]episodeItem, error) {
	if s.config.EpisodeStore == nil || s.config.KeywordLimit <= 0 {
		return nil, nil
	}
	terms := keywordTerms(userMessage)
	if len(terms) == 0 {
		return nil, nil
	}
	matches := make([]episodeItem, 0, s.config.KeywordLimit)
	seen := map[string]struct{}{}
	for _, term := range terms {
		entries, err := s.config.EpisodeStore.FindBySummary(ctx, scope.Type, scope.ID, term)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			key := strings.ToLower(strings.TrimSpace(entry.EpisodeSummary()))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			origin := memoryOrigin(entry)
			if origin.Sensitivity == content.Secret {
				continue
			}
			matches = append(matches, episodeItem{Summary: entry.EpisodeSummary(), Distance: 0.25, Source: "keyword", Origin: origin})
			if len(matches) >= s.config.KeywordLimit {
				return matches, nil
			}
		}
	}
	return matches, nil
}

func (s *Service) loadWorking(ctx context.Context, sessionKey string) (WorkingContext, error) {
	started := time.Now()
	if s.config.WorkingStore == nil {
		s.observeOperation(ctx, "working_load", started, nil, "disabled", nil)
		return WorkingContext{Status: "idle"}, nil
	}
	snapshot, err := s.config.WorkingStore.Get(ctx, sessionKey)
	if err != nil {
		if errors.Is(err, ErrWorkingNotFound) {
			s.observeOperation(ctx, "working_load", started, nil, "empty", nil)
		} else {
			s.observeOperation(ctx, "working_load", started, err, "unknown", nil)
		}
		return WorkingContext{Status: "idle"}, nil
	}
	s.observeOperation(ctx, "working_load", started, nil, "success", nil)
	return WorkingContext{
		Goal:           strings.TrimSpace(snapshot.Goal),
		Status:         normalizeWorkingStatus(snapshot.Status),
		ActiveEntities: decodeJSONValue(snapshot.EntitiesJSON, map[string]any{}),
		PendingSteps:   decodeJSONValue(snapshot.PendingStepsJSON, []any{}),
		Scratch:        decodeJSONObject(snapshot.ScratchJSON),
	}, nil
}

func (s *Service) loadChunks(ctx context.Context, scopes []Scope, userMessage string, includeQuery bool) ([]map[string]any, error) {
	if s.config.Semantic != nil && includeQuery && strings.TrimSpace(userMessage) != "" {
		hits, err := s.config.Semantic.Search(ctx, scopes, userMessage, min(4, s.config.ChunkLimit))
		if err != nil {
			return nil, fmt.Errorf("local semantic retrieval unavailable: %w", err)
		}
		items := make([]map[string]any, 0, len(hits))
		for _, h := range hits {
			if h.MemoryOrigin().Sensitivity == content.Secret {
				continue
			}
			items = append(items, map[string]any{"title": h.Title, "summary": h.Summary, "distance": h.Distance, "source": "local_semantic", "source_ref": h.SourceRef, "origin_type": h.OriginType, "origin_id": h.OriginID, "source_revision": h.Revision, "index_generation": h.GenerationID, "index_space": h.SpaceID, "origin": h.MemoryOrigin(), "excerpt_clipped": h.ExcerptClipped})
		}
		return items, nil
	}
	if !includeQuery || s.config.ChunkStore == nil || s.config.ChunkLimit <= 0 || strings.TrimSpace(userMessage) == "" {
		return nil, nil
	}
	if isNilEmbeddingProvider(s.config.Embeddings) {
		return nil, nil
	}
	queryEmbedding, err := s.config.Embeddings.EmbedQuery(ctx, userMessage)
	if err != nil || len(queryEmbedding) != embeddings.VectorDimensions() {
		return nil, nil
	}
	items := []map[string]any{}
	seen := map[string]struct{}{}
	for _, scope := range scopes {
		results, err := s.config.ChunkStore.Search(ctx, scope.Type, scope.ID, queryEmbedding, s.config.ChunkLimit)
		if err != nil {
			return nil, fmt.Errorf("load chunk memory: %w", err)
		}
		for _, item := range results {
			key := strings.ToLower(strings.TrimSpace(item.ChunkTitle()))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			origin := memoryOrigin(item)
			if origin.Sensitivity == content.Secret {
				continue
			}
			items = append(items, map[string]any{"title": item.ChunkTitle(), "summary": item.ChunkSummary(), "distance": item.ChunkDistance(), "source": "vector", "origin": origin})
			if len(items) >= s.config.ChunkLimit {
				return items, nil
			}
		}
		for _, term := range keywordTerms(userMessage) {
			matches, err := s.config.ChunkStore.FindByTitle(ctx, scope.Type, scope.ID, term, s.config.KeywordLimit)
			if err != nil {
				return nil, fmt.Errorf("load chunk titles: %w", err)
			}
			for _, match := range matches {
				key := strings.ToLower(strings.TrimSpace(match.ChunkTitle()))
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				origin := memoryOrigin(match)
				if origin.Sensitivity == content.Secret {
					continue
				}
				items = append(items, map[string]any{"title": match.ChunkTitle(), "summary": match.ChunkSummary(), "distance": 0.25, "source": "keyword", "origin": origin})
				if len(items) >= s.config.ChunkLimit {
					return items, nil
				}
			}
		}
	}
	return items, nil
}

func (s *Service) loadSummary(ctx context.Context, sessionKey string) string {
	started := time.Now()
	if s.config.SummaryReader == nil {
		s.observeOperation(ctx, "summary_load", started, nil, "disabled", nil)
		return ""
	}
	summary, err := s.config.SummaryReader.GetSummary(ctx, sessionKey)
	if err != nil {
		s.observeOperation(ctx, "summary_load", started, err, "unknown", nil)
		s.log.Warn("session summary retrieval failed", slog.String("session_key", sessionKey), slog.String("error_class", fmt.Sprintf("%T", err)))
		return ""
	}
	outcome := "success"
	if strings.TrimSpace(summary) == "" {
		outcome = "empty"
	}
	s.observeOperation(ctx, "summary_load", started, nil, outcome, nil)
	return strings.TrimSpace(summary)
}

func (w WorkingContext) BundleMap() map[string]any {
	return map[string]any{"goal": w.Goal, "active_entities": w.ActiveEntities, "pending_steps": w.PendingSteps, "working_status": w.Status}
}

func (w WorkingContext) IsEmpty() bool {
	if strings.TrimSpace(w.Goal) != "" {
		return false
	}
	if !isEmptyJSONValue(w.ActiveEntities) {
		return false
	}
	if !isEmptyJSONValue(w.PendingSteps) {
		return false
	}
	return strings.TrimSpace(normalizeWorkingStatus(w.Status)) == "idle"
}

func FormatPrompt(working WorkingContext, profileEntries, episodes, chunks []map[string]any, sessionSummary string) string {
	sections := make([]string, 0, 5)
	if strings.TrimSpace(sessionSummary) != "" {
		sections = append(sections, "Session summary:\n"+sessionSummary)
	}
	if lines := formatWorkingMemoryLines(working); len(lines) > 0 {
		sections = append(sections, "Working memory:\n"+strings.Join(lines, "\n"))
	}
	if profile := FormatProfileEntries(profileEntries); profile != "" {
		sections = append(sections, "Profile memory:\n"+profile)
	}
	if len(episodes) > 0 {
		lines := make([]string, 0, len(episodes))
		for _, entry := range episodes {
			lines = append(lines, fmt.Sprintf("- %s", entry["summary"]))
		}
		sections = append(sections, "Relevant episodes:\n"+strings.Join(lines, "\n"))
	}
	if len(chunks) > 0 {
		lines := make([]string, 0, len(chunks))
		for _, entry := range chunks {
			line := fmt.Sprintf("- %s: %s", entry["title"], entry["summary"])
			if ref, ok := entry["source_ref"].(string); ok && ref != "" {
				line += fmt.Sprintf(" (source %s; revision %s; retrieved evidence)", ref, entry["source_revision"])
			}
			lines = append(lines, line)
		}
		sections = append(sections, "Relevant document chunks:\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

func formatWorkingMemoryLines(working WorkingContext) []string {
	if working.IsEmpty() {
		return nil
	}
	lines := []string{}
	if goal := strings.TrimSpace(working.Goal); goal != "" {
		lines = append(lines, "- Goal: "+goal)
	}
	if entities := formatJSONValueForPrompt(working.ActiveEntities); entities != "" {
		lines = append(lines, "- Active entities: "+entities)
	}
	if pending := formatJSONValueForPrompt(working.PendingSteps); pending != "" {
		lines = append(lines, "- Pending steps: "+pending)
	}
	if status := normalizeWorkingStatus(working.Status); status != "idle" {
		lines = append(lines, "- Status: "+status)
	}
	return lines
}

func keywordTerms(input string) []string {
	normalized := strings.ToLower(strings.TrimSpace(input))
	if normalized == "" {
		return nil
	}
	parts := strings.Fields(strings.NewReplacer(",", " ", ".", " ", ":", " ", ";", " ", "-", " ", "_", " ", "\n", " ").Replace(normalized))
	seen := map[string]struct{}{}
	terms := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) < 4 {
			continue
		}
		if _, ok := seen[part]; ok {
			continue
		}
		seen[part] = struct{}{}
		terms = append(terms, part)
	}
	return terms
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func (s *Service) recordRetrievalMetric(memoryType string, hit bool) {
	if s.config.Metrics == nil {
		return
	}
	status := "miss"
	if hit {
		status = "hit"
	}
	_ = s.config.Metrics.IncrCounter(metrics.MetricMemoryRetrievalsTotal, map[string]string{"memory_type": memoryType, "service": "memory-service", "status": status})
}

func normalizeWorkingStatus(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "idle"
	}
	return trimmed
}

func decodeJSONValue(raw string, fallback any) any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return fallback
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return fallback
	}
	return decoded
}

func decodeJSONObject(raw string) map[string]any {
	decoded := decodeJSONValue(raw, map[string]any{})
	object, ok := decoded.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return object
}

func isEmptyJSONValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case map[string]any:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	case string:
		return strings.TrimSpace(typed) == ""
	default:
		return false
	}
}

func formatJSONValueForPrompt(value any) string {
	if isEmptyJSONValue(value) {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func isNilEmbeddingProvider(provider EmbeddingProvider) bool {
	if provider == nil {
		return true
	}
	v := reflect.ValueOf(provider)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
