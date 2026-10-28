package api

import (
	"context"
	"io"
	"net/http"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
)

type OwnerOutcomeStore interface {
	Pending(context.Context) ([]actionauth.OwnerOutcomeReview, error)
	Historical(context.Context, string, string) (actionauth.OwnerOutcomeReview, error)
	Decide(context.Context, actionauth.OwnerOutcomeDecision) (actionauth.OwnerOutcomeReview, error)
}

// This protected control accepts factual owner reports, not an action approval.
// Actor comes exclusively from the existing owner-control authentication.
func OwnerOutcomes(auth *OwnerControlAuth, store OwnerOutcomeStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		a, ok := auth.Authenticate(r)
		if !ok || len(r.Header.Values("Authorization")) != 1 {
			writeJSON(w, 401, map[string]string{"error": "owner_auth_required"})
			return
		}
		if store == nil {
			writeJSON(w, 503, map[string]string{"error": "outcome_reviews_unavailable"})
			return
		}
		if r.URL.Path != "/api/v4/action-outcomes" || r.URL.RawQuery != "" || r.URL.RawPath != "" {
			w.WriteHeader(400)
			return
		}
		ctx := owneridentity.WithActor(r.Context(), owneridentity.Actor{OwnerID: a.OwnerID, Channel: a.Channel, DeliveryID: a.DeliveryID})
		if r.Method == http.MethodGet {
			rows, err := store.Pending(ctx)
			if err != nil {
				writeJSON(w, 503, map[string]string{"error": "outcome_reviews_unavailable"})
				return
			}
			writeJSON(w, 200, map[string]any{"reviews": rows})
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		var q struct {
			Action      string                           `json:"action"`
			EffectID    string                           `json:"effect_id"`
			Fingerprint string                           `json:"fingerprint"`
			Decision    *actionauth.OwnerOutcomeDecision `json:"decision"`
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
		if err != nil || communicationcontrol.StrictJSON(raw, &q) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_outcome_report"})
			return
		}
		var row actionauth.OwnerOutcomeReview
		switch q.Action {
		case "review_historical":
			if q.Decision != nil {
				w.WriteHeader(400)
				return
			}
			row, err = store.Historical(ctx, q.EffectID, q.Fingerprint)
		case "report":
			if q.Decision == nil || q.EffectID != "" || q.Fingerprint != "" {
				w.WriteHeader(400)
				return
			}
			row, err = store.Decide(ctx, *q.Decision)
		default:
			w.WriteHeader(400)
			return
		}
		if err != nil {
			writeJSON(w, 409, map[string]string{"error": "outcome_report_conflict"})
			return
		}
		writeJSON(w, 200, map[string]any{"review": row})
	})
}
