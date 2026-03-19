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
	"github.com/butler/butler/internal/memory/policy"
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

type profileMemoryWriter interface {
	GetByID(ctx context.Context, id int64) (profile.Entry, error)
	UpdateValue(ctx context.Context, id int64, valueJSON, summary, editedBy string) (profile.Entry, error)
	Confirm(ctx context.Context, id int64, editedBy string) (profile.Entry, error)
	Reject(ctx context.Context, id int64, editedBy string) (profile.Entry, error)
	Suppress(ctx context.Context, id int64, editedBy string) (profile.Entry, error)
	Unsuppress(ctx context.Context, id int64, editedBy string) (profile.Entry, error)
	SoftDelete(ctx context.Context, id int64, editedBy string) (profile.Entry, error)
}

type episodicMemoryReader interface {
	GetByScope(ctx context.Context, scopeType, scopeID string) ([]episodic.Episode, error)
}

type episodicMemoryWriter interface {
	GetByID(ctx context.Context, id int64) (episodic.Episode, error)
	Confirm(ctx context.Context, id int64, editedBy string) (episodic.Episode, error)
	Reject(ctx context.Context, id int64, editedBy string) (episodic.Episode, error)
	Suppress(ctx context.Context, id int64, editedBy string) (episodic.Episode, error)
	Unsuppress(ctx context.Context, id int64, editedBy string) (episodic.Episode, error)
	SoftDelete(ctx context.Context, id int64, editedBy string) (episodic.Episode, error)
}

type chunkMemoryReader interface {
	GetByScope(ctx context.Context, scopeType, scopeID string, limit int) ([]chunks.Chunk, error)
}

type chunkMemoryWriter interface {
	GetByID(ctx context.Context, id int64) (chunks.Chunk, error)
	Suppress(ctx context.Context, id int64, editedBy string) (chunks.Chunk, error)
	Unsuppress(ctx context.Context, id int64, editedBy string) (chunks.Chunk, error)
	HardDelete(ctx context.Context, id int64) error
}

type provenanceLinkReader interface {
	ListBySource(ctx context.Context, sourceMemoryType string, sourceMemoryID int64) ([]provenance.Link, error)
}

type MemoryServer struct {
	working        workingMemoryReader
	profile        profileMemoryReader
	profileWriter  profileMemoryWriter
	episodic       episodicMemoryReader
	episodicWriter episodicMemoryWriter
	chunks         chunkMemoryReader
	chunksWriter   chunkMemoryWriter
	links          provenanceLinkReader
}

func NewMemoryServer(workingStore workingMemoryReader, profileStore profileMemoryReader, episodicStore episodicMemoryReader, chunkStore chunkMemoryReader, links provenanceLinkReader) *MemoryServer {
	return &MemoryServer{working: workingStore, profile: profileStore, episodic: episodicStore, chunks: chunkStore, links: links}
}

// SetWriters sets the writer interfaces for memory management operations.
// This is optional - if not called, write operations will return 501 Not Implemented.
func (m *MemoryServer) SetWriters(profileWriter profileMemoryWriter, episodicWriter episodicMemoryWriter, chunksWriter chunkMemoryWriter) {
	m.profileWriter = profileWriter
	m.episodicWriter = episodicWriter
	m.chunksWriter = chunksWriter
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
	dto := map[string]any{
		"id":                 entry.ID,
		"memory_type":        entry.MemoryType,
		"scope_type":         entry.ScopeType,
		"scope_id":           entry.ScopeID,
		"key":                entry.Key,
		"value_json":         prettyJSON(entry.ValueJSON),
		"summary":            entry.Summary,
		"source_type":        entry.SourceType,
		"source_id":          entry.SourceID,
		"provenance":         prettyJSON(entry.ProvenanceJSON),
		"confidence":         entry.Confidence,
		"status":             entry.Status,
		"confirmation_state": entry.ConfirmationState,
		"effective_status":   entry.EffectiveStatus,
		"suppressed":         entry.Suppressed,
		"created_at":         entry.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":         entry.UpdatedAt.UTC().Format(time.RFC3339),
		"links":              provenanceLinks(ctx, links, profile.MemoryType, entry.ID),
		"capabilities":       capabilitiesDTO(policy.GetCapabilities(policy.MemoryTypeProfile)),
	}
	if entry.ExpiresAt != nil {
		dto["expires_at"] = entry.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if entry.EditedBy != "" {
		dto["edited_by"] = entry.EditedBy
	}
	if entry.EditedAt != nil {
		dto["edited_at"] = entry.EditedAt.UTC().Format(time.RFC3339)
	}
	return dto
}

func toEpisodeMemoryDTO(ctx context.Context, links provenanceLinkReader, entry episodic.Episode) map[string]any {
	dto := map[string]any{
		"id":                 entry.ID,
		"memory_type":        entry.MemoryType,
		"scope_type":         entry.ScopeType,
		"scope_id":           entry.ScopeID,
		"summary":            entry.Summary,
		"content":            entry.Content,
		"source_type":        entry.SourceType,
		"source_id":          entry.SourceID,
		"provenance":         prettyJSON(entry.ProvenanceJSON),
		"confidence":         entry.Confidence,
		"status":             entry.Status,
		"tags_json":          prettyJSON(entry.TagsJSON),
		"confirmation_state": entry.ConfirmationState,
		"effective_status":   entry.EffectiveStatus,
		"suppressed":         entry.Suppressed,
		"created_at":         entry.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":         entry.UpdatedAt.UTC().Format(time.RFC3339),
		"links":              provenanceLinks(ctx, links, episodic.MemoryType, entry.ID),
		"capabilities":       capabilitiesDTO(policy.GetCapabilities(policy.MemoryTypeEpisodic)),
	}
	if entry.ExpiresAt != nil {
		dto["expires_at"] = entry.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if entry.EditedBy != "" {
		dto["edited_by"] = entry.EditedBy
	}
	if entry.EditedAt != nil {
		dto["edited_at"] = entry.EditedAt.UTC().Format(time.RFC3339)
	}
	return dto
}

func toChunkMemoryDTO(ctx context.Context, links provenanceLinkReader, entry chunks.Chunk) map[string]any {
	dto := map[string]any{
		"id":               entry.ID,
		"memory_type":      entry.MemoryType,
		"scope_type":       entry.ScopeType,
		"scope_id":         entry.ScopeID,
		"title":            entry.Title,
		"summary":          entry.Summary,
		"content":          entry.Content,
		"source_type":      entry.SourceType,
		"source_id":        entry.SourceID,
		"provenance":       prettyJSON(entry.ProvenanceJSON),
		"confidence":       entry.Confidence,
		"status":           entry.Status,
		"tags_json":        prettyJSON(entry.TagsJSON),
		"effective_status": entry.EffectiveStatus,
		"suppressed":       entry.Suppressed,
		"created_at":       entry.CreatedAt.UTC().Format(time.RFC3339),
		"updated_at":       entry.UpdatedAt.UTC().Format(time.RFC3339),
		"links":            provenanceLinks(ctx, links, chunks.MemoryType, entry.ID),
		"capabilities":     capabilitiesDTO(policy.GetCapabilities(policy.MemoryTypeChunk)),
	}
	if entry.ExpiresAt != nil {
		dto["expires_at"] = entry.ExpiresAt.UTC().Format(time.RFC3339)
	}
	if entry.EditedBy != "" {
		dto["edited_by"] = entry.EditedBy
	}
	if entry.EditedAt != nil {
		dto["edited_at"] = entry.EditedAt.UTC().Format(time.RFC3339)
	}
	return dto
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

func capabilitiesDTO(c policy.Capabilities) map[string]any {
	return map[string]any{
		"editable":       c.Editable,
		"confirmable":    c.Confirmable,
		"suppressible":   c.Suppressible,
		"deletable":      c.Deletable,
		"hard_deletable": c.HardDeletable,
	}
}

// toMemoryType converts a string to policy.MemoryType
func toMemoryType(s string) policy.MemoryType {
	return policy.MemoryType(s)
}

// Memory type string constants for switch cases
const (
	memoryTypeProfile  = "profile"
	memoryTypeEpisodic = "episodic"
	memoryTypeChunk    = "chunk"
)

// HandleGet returns a single memory entry by type and ID.
func (m *MemoryServer) HandleGet() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		memoryType := strings.TrimSpace(r.URL.Query().Get("memory_type"))
		idStr := strings.TrimSpace(r.URL.Query().Get("id"))
		if memoryType == "" || idStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory_type and id are required"})
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}

		ctx := r.Context()
		switch memoryType {
		case memoryTypeProfile:
			if m.profileWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "profile memory not available"})
				return
			}
			entry, err := m.profileWriter.GetByID(ctx, id)
			if err != nil {
				if errors.Is(err, profile.ErrEntryNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toProfileMemoryDTO(ctx, m.links, entry))

		case memoryTypeEpisodic:
			if m.episodicWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "episodic memory not available"})
				return
			}
			entry, err := m.episodicWriter.GetByID(ctx, id)
			if err != nil {
				if errors.Is(err, episodic.ErrEpisodeNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toEpisodeMemoryDTO(ctx, m.links, entry))

		case memoryTypeChunk:
			if m.chunksWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "chunk memory not available"})
				return
			}
			entry, err := m.chunksWriter.GetByID(ctx, id)
			if err != nil {
				if errors.Is(err, chunks.ErrChunkNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toChunkMemoryDTO(ctx, m.links, entry))

		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unknown memory_type"})
		}
	})
}

// MemoryPatchRequest is the request body for PATCH /api/memory
type MemoryPatchRequest struct {
	ValueJSON string `json:"value_json,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

// HandlePatch updates a memory entry (only for editable types like profile).
func (m *MemoryServer) HandlePatch() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		memoryType := strings.TrimSpace(r.URL.Query().Get("memory_type"))
		idStr := strings.TrimSpace(r.URL.Query().Get("id"))
		if memoryType == "" || idStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory_type and id are required"})
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}

		caps := policy.GetCapabilities(toMemoryType(memoryType))
		if !caps.Editable {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this memory type is not editable"})
			return
		}

		var req MemoryPatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		ctx := r.Context()
		editedBy := r.Header.Get("X-Butler-User")
		if editedBy == "" {
			editedBy = "web_ui"
		}

		switch memoryType {
		case memoryTypeProfile:
			if m.profileWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "profile memory not available"})
				return
			}
			entry, err := m.profileWriter.UpdateValue(ctx, id, req.ValueJSON, req.Summary, editedBy)
			if err != nil {
				if errors.Is(err, profile.ErrEntryNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toProfileMemoryDTO(ctx, m.links, entry))

		default:
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this memory type is not editable"})
		}
	})
}

// HandleDelete deletes a memory entry (soft or hard depending on type).
func (m *MemoryServer) HandleDelete() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		memoryType := strings.TrimSpace(r.URL.Query().Get("memory_type"))
		idStr := strings.TrimSpace(r.URL.Query().Get("id"))
		if memoryType == "" || idStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory_type and id are required"})
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}

		caps := policy.GetCapabilities(toMemoryType(memoryType))
		if !caps.Deletable {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this memory type is not deletable"})
			return
		}

		ctx := r.Context()
		editedBy := r.Header.Get("X-Butler-User")
		if editedBy == "" {
			editedBy = "web_ui"
		}

		switch memoryType {
		case memoryTypeProfile:
			if m.profileWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "profile memory not available"})
				return
			}
			// Profile uses soft delete
			entry, err := m.profileWriter.SoftDelete(ctx, id, editedBy)
			if err != nil {
				if errors.Is(err, profile.ErrEntryNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toProfileMemoryDTO(ctx, m.links, entry))

		case memoryTypeEpisodic:
			if m.episodicWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "episodic memory not available"})
				return
			}
			// Episodic uses soft delete
			entry, err := m.episodicWriter.SoftDelete(ctx, id, editedBy)
			if err != nil {
				if errors.Is(err, episodic.ErrEpisodeNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toEpisodeMemoryDTO(ctx, m.links, entry))

		case memoryTypeChunk:
			if m.chunksWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "chunk memory not available"})
				return
			}
			// Chunks use hard delete
			if err := m.chunksWriter.HardDelete(ctx, id); err != nil {
				if errors.Is(err, chunks.ErrChunkNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

		default:
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this memory type is not deletable"})
		}
	})
}

// HandleConfirm confirms a pending memory entry.
func (m *MemoryServer) HandleConfirm() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		memoryType := strings.TrimSpace(r.URL.Query().Get("memory_type"))
		idStr := strings.TrimSpace(r.URL.Query().Get("id"))
		if memoryType == "" || idStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory_type and id are required"})
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}

		caps := policy.GetCapabilities(toMemoryType(memoryType))
		if !caps.Confirmable {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this memory type does not support confirmation"})
			return
		}

		ctx := r.Context()
		editedBy := r.Header.Get("X-Butler-User")
		if editedBy == "" {
			editedBy = "web_ui"
		}

		switch memoryType {
		case memoryTypeProfile:
			if m.profileWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "profile memory not available"})
				return
			}
			entry, err := m.profileWriter.Confirm(ctx, id, editedBy)
			if err != nil {
				if errors.Is(err, profile.ErrEntryNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found or not pending"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toProfileMemoryDTO(ctx, m.links, entry))

		case memoryTypeEpisodic:
			if m.episodicWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "episodic memory not available"})
				return
			}
			entry, err := m.episodicWriter.Confirm(ctx, id, editedBy)
			if err != nil {
				if errors.Is(err, episodic.ErrEpisodeNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found or not pending"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toEpisodeMemoryDTO(ctx, m.links, entry))

		default:
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this memory type does not support confirmation"})
		}
	})
}

// HandleReject rejects a pending memory entry.
func (m *MemoryServer) HandleReject() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		memoryType := strings.TrimSpace(r.URL.Query().Get("memory_type"))
		idStr := strings.TrimSpace(r.URL.Query().Get("id"))
		if memoryType == "" || idStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory_type and id are required"})
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}

		caps := policy.GetCapabilities(toMemoryType(memoryType))
		if !caps.Confirmable {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this memory type does not support confirmation"})
			return
		}

		ctx := r.Context()
		editedBy := r.Header.Get("X-Butler-User")
		if editedBy == "" {
			editedBy = "web_ui"
		}

		switch memoryType {
		case memoryTypeProfile:
			if m.profileWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "profile memory not available"})
				return
			}
			entry, err := m.profileWriter.Reject(ctx, id, editedBy)
			if err != nil {
				if errors.Is(err, profile.ErrEntryNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found or not pending"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toProfileMemoryDTO(ctx, m.links, entry))

		case memoryTypeEpisodic:
			if m.episodicWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "episodic memory not available"})
				return
			}
			entry, err := m.episodicWriter.Reject(ctx, id, editedBy)
			if err != nil {
				if errors.Is(err, episodic.ErrEpisodeNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found or not pending"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toEpisodeMemoryDTO(ctx, m.links, entry))

		default:
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this memory type does not support confirmation"})
		}
	})
}

// HandleSuppress suppresses a memory entry (hidden from retrieval but kept for audit).
func (m *MemoryServer) HandleSuppress() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		memoryType := strings.TrimSpace(r.URL.Query().Get("memory_type"))
		idStr := strings.TrimSpace(r.URL.Query().Get("id"))
		if memoryType == "" || idStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory_type and id are required"})
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}

		caps := policy.GetCapabilities(toMemoryType(memoryType))
		if !caps.Suppressible {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this memory type does not support suppression"})
			return
		}

		ctx := r.Context()
		editedBy := r.Header.Get("X-Butler-User")
		if editedBy == "" {
			editedBy = "web_ui"
		}

		switch memoryType {
		case memoryTypeProfile:
			if m.profileWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "profile memory not available"})
				return
			}
			entry, err := m.profileWriter.Suppress(ctx, id, editedBy)
			if err != nil {
				if errors.Is(err, profile.ErrEntryNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found or already suppressed"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toProfileMemoryDTO(ctx, m.links, entry))

		case memoryTypeEpisodic:
			if m.episodicWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "episodic memory not available"})
				return
			}
			entry, err := m.episodicWriter.Suppress(ctx, id, editedBy)
			if err != nil {
				if errors.Is(err, episodic.ErrEpisodeNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found or already suppressed"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toEpisodeMemoryDTO(ctx, m.links, entry))

		case memoryTypeChunk:
			if m.chunksWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "chunk memory not available"})
				return
			}
			entry, err := m.chunksWriter.Suppress(ctx, id, editedBy)
			if err != nil {
				if errors.Is(err, chunks.ErrChunkNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found or already suppressed"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toChunkMemoryDTO(ctx, m.links, entry))

		default:
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this memory type does not support suppression"})
		}
	})
}

// HandleUnsuppress restores a suppressed memory entry.
func (m *MemoryServer) HandleUnsuppress() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		memoryType := strings.TrimSpace(r.URL.Query().Get("memory_type"))
		idStr := strings.TrimSpace(r.URL.Query().Get("id"))
		if memoryType == "" || idStr == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "memory_type and id are required"})
			return
		}
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid id"})
			return
		}

		caps := policy.GetCapabilities(toMemoryType(memoryType))
		if !caps.Suppressible {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this memory type does not support suppression"})
			return
		}

		ctx := r.Context()
		editedBy := r.Header.Get("X-Butler-User")
		if editedBy == "" {
			editedBy = "web_ui"
		}

		switch memoryType {
		case memoryTypeProfile:
			if m.profileWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "profile memory not available"})
				return
			}
			entry, err := m.profileWriter.Unsuppress(ctx, id, editedBy)
			if err != nil {
				if errors.Is(err, profile.ErrEntryNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found or not suppressed"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toProfileMemoryDTO(ctx, m.links, entry))

		case memoryTypeEpisodic:
			if m.episodicWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "episodic memory not available"})
				return
			}
			entry, err := m.episodicWriter.Unsuppress(ctx, id, editedBy)
			if err != nil {
				if errors.Is(err, episodic.ErrEpisodeNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found or not suppressed"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toEpisodeMemoryDTO(ctx, m.links, entry))

		case memoryTypeChunk:
			if m.chunksWriter == nil {
				writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "chunk memory not available"})
				return
			}
			entry, err := m.chunksWriter.Unsuppress(ctx, id, editedBy)
			if err != nil {
				if errors.Is(err, chunks.ErrChunkNotFound) {
					writeJSON(w, http.StatusNotFound, map[string]string{"error": "entry not found or not suppressed"})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, toChunkMemoryDTO(ctx, m.links, entry))

		default:
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "this memory type does not support suppression"})
		}
	})
}
