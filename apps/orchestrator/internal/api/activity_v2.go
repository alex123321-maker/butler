package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	activity "github.com/butler/butler/apps/orchestrator/internal/activity"
)

type activityStore interface {
	ListActivities(ctx context.Context, params activity.ListParams) ([]activity.Record, error)
}

type ActivityServer struct {
	store activityStore
}

func NewActivityServer(store activityStore) *ActivityServer {
	return &ActivityServer{store: store}
}

func (s *ActivityServer) HandleListActivity() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "activity store is not configured"})
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

		params := activity.ListParams{
			RunID:      strings.TrimSpace(query.Get("run_id")),
			SessionKey: strings.TrimSpace(query.Get("session_key")),
			Severity:   strings.TrimSpace(query.Get("severity")),
			ActorType:  strings.TrimSpace(query.Get("actor_type")),
			Limit:      limit,
			Offset:     offset,
		}
		if raw := strings.TrimSpace(query.Get("since")); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "since must be RFC3339"})
				return
			}
			params.Since = &parsed
		}
		if raw := strings.TrimSpace(query.Get("until")); raw != "" {
			parsed, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "until must be RFC3339"})
				return
			}
			params.Until = &parsed
		}

		items, err := s.store.ListActivities(r.Context(), params)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list activity"})
			return
		}
		payload := make([]activityDTO, 0, len(items))
		for _, item := range items {
			payload = append(payload, toActivityDTO(item))
		}
		writeJSON(w, http.StatusOK, map[string]any{"activity": payload})
	})
}

func toActivityDTO(item activity.Record) activityDTO {
	return activityDTO{
		ActivityID:   item.ActivityID,
		RunID:        item.RunID,
		SessionKey:   item.SessionKey,
		ActivityType: item.ActivityType,
		Title:        item.Title,
		Summary:      item.Summary,
		DetailsJSON:  item.DetailsJSON,
		ActorType:    item.ActorType,
		Severity:     item.Severity,
		CreatedAt:    item.CreatedAt.UTC().Format(time.RFC3339),
	}
}

type activityDTO struct {
	ActivityID   int64  `json:"activity_id"`
	RunID        string `json:"run_id"`
	SessionKey   string `json:"session_key"`
	ActivityType string `json:"activity_type"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	DetailsJSON  string `json:"details_json"`
	ActorType    string `json:"actor_type"`
	Severity     string `json:"severity"`
	CreatedAt    string `json:"created_at"`
}
