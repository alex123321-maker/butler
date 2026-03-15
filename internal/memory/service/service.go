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
}

type WorkingStore interface {
	Get(context.Context, string) (WorkingSnapshot, error)
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

type Config struct {
	ProfileStore  ProfileStore
	EpisodeStore  EpisodeStore
	WorkingStore  WorkingStore
	SummaryReader SummaryReader
	Embeddings    EmbeddingProvider
	ProfileLimit  int
	EpisodeLimit  int
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
}

func New(cfg Config) *Service {
	if cfg.ProfileLimit <= 0 {
		cfg.ProfileLimit = 20
	}
	if cfg.EpisodeLimit <= 0 {
		cfg.EpisodeLimit = 3
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
	summary := s.loadSummary(ctx, req.SessionKey)

	items := map[string]any{}
	if !working.IsEmpty() {
		items["working"] = working.BundleMap()
	}
	if len(profileEntries) > 0 {
		items["profile"] = profileEntries
	}
	if len(episodes) > 0 {
		items["episodes"] = episodes
	}
	if summary != "" {
		items["session_summary"] = summary
	}
	return Bundle{Items: items, Prompt: FormatPrompt(working, profileEntries, episodes, summary), Working: working}, nil
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
	for _, scope := range scopes {
		results, err := s.config.EpisodeStore.Search(ctx, scope.Type, scope.ID, queryEmbedding, s.config.EpisodeLimit)
		if err != nil {
			return nil, fmt.Errorf("load episodic memory: %w", err)
		}
		for _, item := range results {
			items = append(items, episodeItem{Summary: item.EpisodeSummary(), Distance: item.EpisodeDistance()})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Distance < items[j].Distance })
	if len(items) > s.config.EpisodeLimit {
		items = items[:s.config.EpisodeLimit]
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{"summary": item.Summary, "distance": item.Distance})
	}
	return result, nil
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

func FormatPrompt(working WorkingContext, profileEntries, episodes []map[string]any, sessionSummary string) string {
	sections := make([]string, 0, 4)
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
