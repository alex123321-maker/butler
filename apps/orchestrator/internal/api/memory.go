package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/internal/memory/chunks"
	"github.com/butler/butler/internal/memory/episodic"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/provenance"
	"github.com/butler/butler/internal/memory/working"
)

type workingMemoryReader interface {
	Get(ctx context.Context, sessionKey string) (working.Snapshot, error)
}

type profileMemoryReader interface {
	GetByScope(ctx context.Context, scopeType, scopeID string) ([]profile.Entry, error)
}

type episodicMemoryReader interface {
	GetByScope(ctx context.Context, scopeType, scopeID string) ([]episodic.Episode, error)
}

type chunkMemoryReader interface {
	GetByScope(ctx context.Context, scopeType, scopeID string, limit int) ([]chunks.Chunk, error)
}

type provenanceLinkReader interface {
	ListBySource(ctx context.Context, sourceMemoryType string, sourceMemoryID int64) ([]provenance.Link, error)
}

type MemoryServer struct {
	working  workingMemoryReader
	profile  profileMemoryReader
	episodic episodicMemoryReader
	chunks   chunkMemoryReader
	links    provenanceLinkReader
}

func NewMemoryServer(workingStore workingMemoryReader, profileStore profileMemoryReader, episodicStore episodicMemoryReader, chunkStore chunkMemoryReader, links provenanceLinkReader) *MemoryServer {
	return &MemoryServer{working: workingStore, profile: profileStore, episodic: episodicStore, chunks: chunkStore, links: links}
}

func (m *MemoryServer) HandleList() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		scopeType := strings.TrimSpace(r.URL.Query().Get("scope_type"))
		scopeID := strings.TrimSpace(r.URL.Query().Get("scope_id"))
		if scopeType == "" || scopeID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "scope_type and scope_id are required"})
			return
		}
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 {
			limit = 50
		}

		payload, err := m.buildScopeView(r.Context(), scopeType, scopeID, limit)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, payload)
	})
}

func (m *MemoryServer) buildScopeView(ctx context.Context, scopeType, scopeID string, limit int) (map[string]any, error) {
	response := map[string]any{"scope_type": scopeType, "scope_id": scopeID, "limit": limit}
	if m.working != nil && scopeType == "session" {
		snapshot, err := m.working.Get(ctx, scopeID)
		switch {
		case err == nil:
			response["working"] = toWorkingMemoryDTO(snapshot)
		case errors.Is(err, working.ErrSnapshotNotFound), errors.Is(err, working.ErrStoreNotConfigured):
			// no working snapshot for this scope
		default:
			return nil, err
		}
	}
	if m.profile != nil {
		entries, err := m.profile.GetByScope(ctx, scopeType, scopeID)
		switch {
		case err == nil:
			items := make([]map[string]any, 0, len(entries))
			for _, entry := range entries {
				items = append(items, toProfileMemoryDTO(ctx, m.links, entry))
			}
			response["profile"] = items
		case errors.Is(err, profile.ErrStoreNotConfigured):
			// profile memory unavailable
		default:
			return nil, err
		}
	}
	if m.episodic != nil {
		entries, err := m.episodic.GetByScope(ctx, scopeType, scopeID)
		switch {
		case err == nil:
			items := make([]map[string]any, 0, len(entries))
			for _, entry := range entries {
				items = append(items, toEpisodeMemoryDTO(ctx, m.links, entry))
			}
			response["episodic"] = items
		case errors.Is(err, episodic.ErrStoreNotConfigured):
			// episodic memory unavailable
		default:
			return nil, err
		}
	}
	if m.chunks != nil {
		entries, err := m.chunks.GetByScope(ctx, scopeType, scopeID, limit)
		switch {
		case err == nil:
			items := make([]map[string]any, 0, len(entries))
			for _, entry := range entries {
				items = append(items, toChunkMemoryDTO(ctx, m.links, entry))
			}
			response["chunks"] = items
		case errors.Is(err, chunks.ErrStoreNotConfigured):
			// chunk memory unavailable
		default:
			return nil, err
		}
	}
	return response, nil
}

func toWorkingMemoryDTO(snapshot working.Snapshot) map[string]any {
	return map[string]any{
		"memory_type":        snapshot.MemoryType,
		"session_key":        snapshot.SessionKey,
		"run_id":             snapshot.RunID,
		"goal":               snapshot.Goal,
		"entities_json":      prettyJSON(snapshot.EntitiesJSON),
		"pending_steps_json": prettyJSON(snapshot.PendingStepsJSON),
		"scratch_json":       prettyJSON(snapshot.ScratchJSON),
		"status":             snapshot.Status,
		"source_type":        snapshot.SourceType,
		"source_id":          snapshot.SourceID,
		"provenance":         prettyJSON(snapshot.ProvenanceJSON),
		"created_at":         snapshot.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":         snapshot.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func toProfileMemoryDTO(ctx context.Context, links provenanceLinkReader, entry profile.Entry) map[string]any {
	return map[string]any{
		"id":          entry.ID,
		"memory_type": entry.MemoryType,
		"scope_type":  entry.ScopeType,
		"scope_id":    entry.ScopeID,
		"key":         entry.Key,
		"value_json":  prettyJSON(entry.ValueJSON),
		"summary":     entry.Summary,
		"source_type": entry.SourceType,
		"source_id":   entry.SourceID,
		"provenance":  prettyJSON(entry.ProvenanceJSON),
		"confidence":  entry.Confidence,
		"status":      entry.Status,
		"created_at":  entry.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  entry.UpdatedAt.UTC().Format(time.RFC3339),
		"links":       provenanceLinks(ctx, links, profile.MemoryType, entry.ID),
	}
}

func toEpisodeMemoryDTO(ctx context.Context, links provenanceLinkReader, entry episodic.Episode) map[string]any {
	return map[string]any{
		"id":          entry.ID,
		"memory_type": entry.MemoryType,
		"scope_type":  entry.ScopeType,
		"scope_id":    entry.ScopeID,
		"summary":     entry.Summary,
		"content":     entry.Content,
		"source_type": entry.SourceType,
		"source_id":   entry.SourceID,
		"provenance":  prettyJSON(entry.ProvenanceJSON),
		"confidence":  entry.Confidence,
		"status":      entry.Status,
		"tags_json":   prettyJSON(entry.TagsJSON),
		"created_at":  entry.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  entry.UpdatedAt.UTC().Format(time.RFC3339),
		"links":       provenanceLinks(ctx, links, episodic.MemoryType, entry.ID),
	}
}

func toChunkMemoryDTO(ctx context.Context, links provenanceLinkReader, entry chunks.Chunk) map[string]any {
	return map[string]any{
		"id":          entry.ID,
		"memory_type": entry.MemoryType,
		"scope_type":  entry.ScopeType,
		"scope_id":    entry.ScopeID,
		"title":       entry.Title,
		"summary":     entry.Summary,
		"content":     entry.Content,
		"source_type": entry.SourceType,
		"source_id":   entry.SourceID,
		"provenance":  prettyJSON(entry.ProvenanceJSON),
		"confidence":  entry.Confidence,
		"status":      entry.Status,
		"tags_json":   prettyJSON(entry.TagsJSON),
		"created_at":  entry.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":  entry.UpdatedAt.UTC().Format(time.RFC3339),
		"links":       provenanceLinks(ctx, links, chunks.MemoryType, entry.ID),
	}
}

func provenanceLinks(ctx context.Context, store provenanceLinkReader, memoryType string, id int64) []map[string]any {
	if store == nil {
		return []map[string]any{}
	}
	items, err := store.ListBySource(ctx, memoryType, id)
	if err != nil {
		return []map[string]any{}
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		result = append(result, map[string]any{"link_type": item.LinkType, "target_type": item.TargetType, "target_id": item.TargetID, "metadata": prettyJSON(item.MetadataJSON)})
	}
	return result
}

func prettyJSON(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "{}"
	}
	var decoded any
	if json.Unmarshal([]byte(trimmed), &decoded) != nil {
		return trimmed
	}
	pretty, err := json.MarshalIndent(decoded, "", "  ")
	if err != nil {
		return trimmed
	}
	return string(pretty)
}
