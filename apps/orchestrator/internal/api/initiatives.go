package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/initiative"
	"github.com/butler/butler/internal/owneridentity"
)

type InitiativeServer struct {
	store *initiative.Store
	auth  *OwnerControlAuth
}

func NewInitiativeServer(store *initiative.Store, auth *OwnerControlAuth) *InitiativeServer {
	return &InitiativeServer{store, auth}
}
func (s *InitiativeServer) Handle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := s.auth.Authenticate(r)
		if !ok {
			writeJSON(w, 401, map[string]string{"error": "authenticated owner required"})
			return
		}
		if s.store == nil {
			writeJSON(w, 503, map[string]string{"error": "initiative service unavailable"})
			return
		}
		if r.Method != "GET" && strings.TrimSpace(r.Header.Get("X-Butler-Delivery-ID")) == "" {
			writeJSON(w, 400, map[string]string{"error": "unique owner delivery identity required"})
			return
		}
		ctx, cancel := context.WithTimeout(owneridentity.WithActor(r.Context(), a), 5*time.Second)
		defer cancel()
		if r.Method == "GET" && r.URL.Path == "/api/v3/initiatives" {
			v, err := s.store.View(ctx, a.OwnerID)
			if err != nil {
				writeInitiativeError(w, err)
				return
			}
			writeJSON(w, 200, v)
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
		decoder.DisallowUnknownFields()
		decode := func(target any) bool { return decoder.Decode(target) == nil && decoder.Decode(new(any)) == io.EOF }
		switch r.URL.Path {
		case "/api/v3/initiatives/sources":
			var req struct {
				Source           initiative.Source `json:"source"`
				ExpectedRevision int64             `json:"expected_revision"`
			}
			if !decode(&req) {
				writeInitiativeError(w, initiative.ErrInvalid)
				return
			}
			v, err := s.store.SaveSource(ctx, req.Source, req.ExpectedRevision)
			if err != nil {
				writeInitiativeError(w, err)
				return
			}
			writeJSON(w, 200, v)
		case "/api/v3/initiatives/schedules":
			var req struct {
				Schedule         initiative.Schedule `json:"schedule"`
				ExpectedRevision int64               `json:"expected_revision"`
			}
			if !decode(&req) {
				writeInitiativeError(w, initiative.ErrInvalid)
				return
			}
			v, err := s.store.SaveSchedule(ctx, req.Schedule, req.ExpectedRevision, time.Now().UTC())
			if err != nil {
				writeInitiativeError(w, err)
				return
			}
			writeJSON(w, 200, v)
		case "/api/v3/initiatives/observations":
			// This initial adapter is an owner-authenticated manual/webhook
			// control-plane input. It accepts normalized identities only; no
			// source-provided text becomes execution instructions or authority.
			var req struct {
				Event          initiative.Event `json:"event"`
				SourceRevision int64            `json:"source_revision"`
			}
			if !decode(&req) || (req.Event.ID != "" && req.Event.ID != a.DeliveryID) {
				writeInitiativeError(w, initiative.ErrInvalid)
				return
			}
			req.Event.ID = a.DeliveryID
			v, created, err := s.store.Observe(ctx, a.OwnerID, req.Event, req.SourceRevision, time.Now().UTC())
			if err != nil {
				writeInitiativeError(w, err)
				return
			}
			writeJSON(w, 202, map[string]any{"initiative": v, "observation_recorded": created})
		default:
			w.WriteHeader(404)
		}
	})
}
func writeInitiativeError(w http.ResponseWriter, err error) {
	status, message := 500, "initiative service operation failed"
	switch {
	case errors.Is(err, initiative.ErrInvalid):
		status, message = 400, "invalid initiative request"
	case errors.Is(err, initiative.ErrConflict):
		status, message = 409, "initiative identity or revision conflict"
	case errors.Is(err, initiative.ErrNotFound):
		status, message = 404, "initiative or source unavailable"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
