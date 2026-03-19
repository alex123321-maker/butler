package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	activity "github.com/butler/butler/apps/orchestrator/internal/activity"
	artifacts "github.com/butler/butler/apps/orchestrator/internal/artifacts"
)

type artifactsStore interface {
	ListArtifacts(ctx context.Context, params artifacts.ListParams) ([]artifacts.Record, error)
	ListArtifactsByRun(ctx context.Context, runID string, limit int) ([]artifacts.Record, error)
	GetArtifactByID(ctx context.Context, artifactID string) (artifacts.Record, error)
}

type taskActivityStore interface {
	ListActivities(ctx context.Context, params activity.ListParams) ([]activity.Record, error)
}

type ArtifactsServer struct {
	store    artifactsStore
	activity taskActivityStore
}

func NewArtifactsServer(store artifactsStore, activityStore taskActivityStore) *ArtifactsServer {
	return &ArtifactsServer{store: store, activity: activityStore}
}

func (s *ArtifactsServer) HandleListArtifacts() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "artifacts store is not configured"})
			return
		}
		query := r.URL.Query()
		limit := 50
		offset := 0
		if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be integer"})
				return
			}
			limit = value
		}
		if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "offset must be integer"})
				return
			}
			offset = value
		}
		items, err := s.store.ListArtifacts(r.Context(), artifacts.ListParams{
			ArtifactType: strings.TrimSpace(query.Get("type")),
			RunID:        strings.TrimSpace(query.Get("run_id")),
			SessionKey:   strings.TrimSpace(query.Get("session_key")),
			Query:        strings.TrimSpace(query.Get("query")),
			Limit:        limit,
			Offset:       offset,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list artifacts"})
			return
		}
		payload := make([]artifactDTO, 0, len(items))
		for _, item := range items {
			payload = append(payload, toArtifactDTO(item))
		}
		writeJSON(w, http.StatusOK, map[string]any{"artifacts": payload})
	})
}

func (s *ArtifactsServer) HandleGetArtifact(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "artifacts store is not configured"})
			return
		}
		artifactID := extractPathParam(r.URL.Path, prefix)
		if artifactID == "" || strings.Contains(artifactID, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "artifact id is required"})
			return
		}
		rec, err := s.store.GetArtifactByID(r.Context(), artifactID)
		if err != nil {
			if err == artifacts.ErrArtifactNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "artifact not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get artifact"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"artifact": toArtifactDTO(rec)})
	})
}

func (s *ArtifactsServer) HandleListTaskArtifacts(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "artifacts store is not configured"})
			return
		}
		runID := extractPathParam(r.URL.Path, prefix)
		runID = strings.TrimSuffix(runID, "/artifacts")
		if runID == "" || strings.Contains(runID, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task id is required"})
			return
		}
		items, err := s.store.ListArtifactsByRun(r.Context(), runID, 100)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list task artifacts"})
			return
		}
		payload := make([]artifactDTO, 0, len(items))
		for _, item := range items {
			payload = append(payload, toArtifactDTO(item))
		}
		writeJSON(w, http.StatusOK, map[string]any{"artifacts": payload})
	})
}

type artifactDTO struct {
	ArtifactID    string `json:"artifact_id"`
	RunID         string `json:"run_id"`
	SessionKey    string `json:"session_key"`
	ArtifactType  string `json:"artifact_type"`
	Title         string `json:"title"`
	Summary       string `json:"summary"`
	ContentText   string `json:"content_text"`
	ContentJSON   string `json:"content_json"`
	ContentFormat string `json:"content_format"`
	SourceType    string `json:"source_type"`
	SourceRef     string `json:"source_ref"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

func toArtifactDTO(rec artifacts.Record) artifactDTO {
	return artifactDTO{
		ArtifactID:    rec.ArtifactID,
		RunID:         rec.RunID,
		SessionKey:    rec.SessionKey,
		ArtifactType:  rec.ArtifactType,
		Title:         rec.Title,
		Summary:       rec.Summary,
		ContentText:   rec.ContentText,
		ContentJSON:   rec.ContentJSON,
		ContentFormat: rec.ContentFormat,
		SourceType:    rec.SourceType,
		SourceRef:     rec.SourceRef,
		CreatedAt:     rec.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:     rec.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *ArtifactsServer) HandleListTaskActivity(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.activity == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "activity store is not configured"})
			return
		}
		runID := extractPathParam(r.URL.Path, prefix)
		runID = strings.TrimSuffix(runID, "/activity")
		if runID == "" || strings.Contains(runID, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "task id is required"})
			return
		}
		items, err := s.activity.ListActivities(r.Context(), activity.ListParams{RunID: runID, Limit: 200, Offset: 0})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list task activity"})
			return
		}
		payload := make([]activityDTO, 0, len(items))
		for _, item := range items {
			payload = append(payload, toActivityDTO(item))
		}
		writeJSON(w, http.StatusOK, map[string]any{"activity": payload})
	})
}
