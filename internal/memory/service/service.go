package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/butler/butler/internal/logger"
	"github.com/butler/butler/internal/memory/embeddings"
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

type Chunk interface {
	ChunkTitle() string
	ChunkSummary() string
	ChunkDistance() float64
}

type Config struct {
	ProfileStore  ProfileStore
	EpisodeStore  EpisodeStore
	ChunkStore    ChunkStore
	WorkingStore  WorkingStore
	SummaryReader SummaryReader
	Embeddings    EmbeddingProvider
	ProfileLimit  int
	EpisodeLimit  int
	ChunkLimit    int
	BundleBudget  int
	KeywordLimit  int
	ScopeOrder    []string
	Log           *slog.Logger
}

type Service struct {
	config Config
	log    *slog.Logger
}

type BundleRequest struct {
	SessionKey   string
	UserID       string
	UserMessage  string
	IncludeQuery bool
}

type Bundle struct {
	Items   map[string]any
	Prompt  string
	Working WorkingContext
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
		cfg.ChunkLimit = 2
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

func (s *Service) BuildBundle(ctx context.Context, req BundleRequest) (Bundle, error) {
	scopes := s.scopes(req.SessionKey, req.UserID)
	profileEntries, err := s.loadProfile(ctx, scopes)
	if err != nil {
		return Bundle{}, err
	}
	working, err := s.loadWorking(ctx, req.SessionKey)
	if err != nil {
		return Bundle{}, err
	}
	episodes, err := s.loadEpisodes(ctx, scopes, req.UserMessage, req.IncludeQuery)
	if err != nil {
		return Bundle{}, err
	}
	chunks, err := s.loadChunks(ctx, scopes, req.UserMessage, req.IncludeQuery)
	if err != nil {
		return Bundle{}, err
	}
	summary := s.loadSummary(ctx, req.SessionKey)
	items, promptSummary, promptWorking, promptProfile, promptEpisodes, promptChunks := s.applyBudget(summary, working, profileEntries, episodes, chunks)

	return Bundle{Items: items, Prompt: FormatPrompt(promptWorking, promptProfile, promptEpisodes, promptChunks, promptSummary), Working: working}, nil
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

func (s *Service) loadProfile(ctx context.Context, scopes []Scope) ([]map[string]any, error) {
	if s.config.ProfileStore == nil || s.config.ProfileLimit <= 0 {
		return nil, nil
	}
	result := make([]map[string]any, 0, s.config.ProfileLimit)
	for _, scope := range scopes {
		entries, err := s.config.ProfileStore.GetByScope(ctx, scope.Type, scope.ID)
		if err != nil {
			return nil, fmt.Errorf("load profile memory: %w", err)
		}
		for _, entry := range entries {
			result = append(result, map[string]any{"key": entry.ProfileKey(), "summary": entry.ProfileSummary(), "scope_type": scope.Type})
			if len(result) >= s.config.ProfileLimit {
				return result, nil
			}
		}
	}
	return result, nil
}

func (s *Service) loadEpisodes(ctx context.Context, scopes []Scope, userMessage string, includeQuery bool) ([]map[string]any, error) {
	if !includeQuery || s.config.EpisodeStore == nil || s.config.EpisodeLimit <= 0 || strings.TrimSpace(userMessage) == "" {
		return nil, nil
	}
	if s.config.Embeddings == nil {
		s.log.Info("episodic retrieval skipped; embedding provider is not configured")
		return nil, nil
	}
	queryEmbedding, err := s.config.Embeddings.EmbedQuery(ctx, userMessage)
	if err != nil {
		s.log.Warn("episodic retrieval skipped; embedding query failed", slog.String("error", err.Error()))
		return nil, nil
	}
	if len(queryEmbedding) != embeddings.VectorDimensions {
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
			items = append(items, episodeItem{Summary: item.EpisodeSummary(), Distance: item.EpisodeDistance(), Source: "vector"})
		}
		keywordMatches, keywordErr := s.loadKeywordEpisodes(ctx, scope, userMessage)
		if keywordErr != nil {
			s.log.Warn("episodic keyword retrieval skipped", slog.String("scope_type", scope.Type), slog.String("error", keywordErr.Error()))
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
		result = append(result, map[string]any{"summary": item.Summary, "distance": item.Distance, "source": item.Source})
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
			matches = append(matches, episodeItem{Summary: entry.EpisodeSummary(), Distance: 0.25, Source: "keyword"})
			if len(matches) >= s.config.KeywordLimit {
				return matches, nil
			}
		}
	}
	return matches, nil
}

func (s *Service) loadWorking(ctx context.Context, sessionKey string) (WorkingContext, error) {
	if s.config.WorkingStore == nil {
		return WorkingContext{Status: "idle"}, nil
	}
	snapshot, err := s.config.WorkingStore.Get(ctx, sessionKey)
	if err != nil {
		return WorkingContext{Status: "idle"}, nil
	}
	return WorkingContext{
		Goal:           strings.TrimSpace(snapshot.Goal),
		Status:         normalizeWorkingStatus(snapshot.Status),
		ActiveEntities: decodeJSONValue(snapshot.EntitiesJSON, map[string]any{}),
		PendingSteps:   decodeJSONValue(snapshot.PendingStepsJSON, []any{}),
		Scratch:        decodeJSONObject(snapshot.ScratchJSON),
	}, nil
}

func (s *Service) loadChunks(ctx context.Context, scopes []Scope, userMessage string, includeQuery bool) ([]map[string]any, error) {
	if !includeQuery || s.config.ChunkStore == nil || s.config.ChunkLimit <= 0 || strings.TrimSpace(userMessage) == "" {
		return nil, nil
	}
	if s.config.Embeddings == nil {
		return nil, nil
	}
	queryEmbedding, err := s.config.Embeddings.EmbedQuery(ctx, userMessage)
	if err != nil || len(queryEmbedding) != embeddings.VectorDimensions {
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
			items = append(items, map[string]any{"title": item.ChunkTitle(), "summary": item.ChunkSummary(), "distance": item.ChunkDistance(), "source": "vector"})
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
				items = append(items, map[string]any{"title": match.ChunkTitle(), "summary": match.ChunkSummary(), "distance": 0.25, "source": "keyword"})
				if len(items) >= s.config.ChunkLimit {
					return items, nil
				}
			}
		}
	}
	return items, nil
}

func (s *Service) loadSummary(ctx context.Context, sessionKey string) string {
	if s.config.SummaryReader == nil {
		return ""
	}
	summary, err := s.config.SummaryReader.GetSummary(ctx, sessionKey)
	if err != nil {
		s.log.Warn("session summary retrieval failed", slog.String("session_key", sessionKey), slog.String("error", err.Error()))
		return ""
	}
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
	if len(profileEntries) > 0 {
		lines := make([]string, 0, len(profileEntries))
		for _, entry := range profileEntries {
			lines = append(lines, fmt.Sprintf("- %s: %s", entry["key"], entry["summary"]))
		}
		sections = append(sections, "Profile memory:\n"+strings.Join(lines, "\n"))
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
			lines = append(lines, fmt.Sprintf("- %s: %s", entry["title"], entry["summary"]))
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

func (s *Service) applyBudget(summary string, working WorkingContext, profileEntries, episodes, chunks []map[string]any) (map[string]any, string, WorkingContext, []map[string]any, []map[string]any, []map[string]any) {
	remaining := s.config.BundleBudget
	items := map[string]any{}
	selectedSummary := ""
	selectedWorking := WorkingContext{Status: "idle"}
	selectedProfile := []map[string]any{}
	selectedEpisodes := []map[string]any{}
	selectedChunks := []map[string]any{}
	if strings.TrimSpace(summary) != "" && remaining > 0 {
		selectedSummary = summary
		items["session_summary"] = summary
		remaining--
	}
	if !working.IsEmpty() && remaining > 0 {
		selectedWorking = working
		items["working"] = working.BundleMap()
		remaining--
	}
	if remaining > 0 && len(profileEntries) > 0 {
		count := min(remaining, len(profileEntries))
		selectedProfile = append(selectedProfile, profileEntries[:count]...)
		items["profile"] = selectedProfile
		remaining -= count
	}
	if remaining > 0 && len(episodes) > 0 {
		count := min(remaining, len(episodes))
		selectedEpisodes = append(selectedEpisodes, episodes[:count]...)
		items["episodes"] = selectedEpisodes
		remaining -= count
	}
	if remaining > 0 && len(chunks) > 0 {
		count := min(remaining, len(chunks))
		selectedChunks = append(selectedChunks, chunks[:count]...)
		items["chunks"] = selectedChunks
	}
	return items, selectedSummary, selectedWorking, selectedProfile, selectedEpisodes, selectedChunks
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
