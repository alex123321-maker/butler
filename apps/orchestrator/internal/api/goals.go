package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/owneridentity"
)

type GoalServer struct {
	manager *goal.Manager
	auth    *OwnerControlAuth
}

func NewGoalServer(manager *goal.Manager, auth *OwnerControlAuth) *GoalServer {
	return &GoalServer{manager, auth}
}
func (s *GoalServer) Handle() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := s.auth.Authenticate(r)
		if !ok {
			writeJSON(w, 401, map[string]string{"error": "authenticated owner required"})
			return
		}
		if s.manager == nil {
			writeJSON(w, 503, map[string]string{"error": "owner authority unavailable"})
			return
		}
		if r.Method != "GET" && strings.TrimSpace(r.Header.Get("X-Butler-Delivery-ID")) == "" {
			writeJSON(w, 400, map[string]string{"error": "unique owner delivery identity required"})
			return
		}
		ctx, cancel := context.WithTimeout(owneridentity.WithActor(r.Context(), a), 5*time.Second)
		defer cancel()
		if r.Method == "GET" && r.URL.Path == "/api/v3/owner-authority" {
			v, err := s.manager.View(ctx)
			if err != nil {
				writeGoalError(w, err)
				return
			}
			writeJSON(w, 200, v)
			return
		}
		if r.Method != "POST" {
			w.WriteHeader(405)
			return
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256*1024))
		decoder.DisallowUnknownFields()
		switch r.URL.Path {
		case "/api/v3/owner-goals":
			var req goal.GoalRequest
			if decodeOwnerRequest(decoder, &req) != nil {
				writeGoalError(w, goal.ErrInvalid)
				return
			}
			v, err := s.manager.PutGoal(ctx, req)
			if err != nil {
				writeGoalError(w, err)
				return
			}
			writeJSON(w, 200, v)
		case "/api/v3/owner-mandates":
			var req goal.MandateRequest
			if decodeOwnerRequest(decoder, &req) != nil {
				writeGoalError(w, goal.ErrInvalid)
				return
			}
			v, err := s.manager.PutMandate(ctx, req)
			if err != nil {
				writeGoalError(w, err)
				return
			}
			writeJSON(w, 200, v)
		case "/api/v3/owner-goals/tasks":
			var req goal.TaskRequest
			if decodeOwnerRequest(decoder, &req) != nil {
				writeGoalError(w, goal.ErrInvalid)
				return
			}
			v, err := s.manager.CreateOwnerTask(ctx, req)
			if err != nil {
				writeGoalError(w, err)
				return
			}
			writeJSON(w, 201, map[string]any{"task": v})
		default:
			w.WriteHeader(404)
		}
	})
}
func decodeOwnerRequest(decoder *json.Decoder, target any) error {
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return goal.ErrInvalid
	}
	return nil
}
func writeGoalError(w http.ResponseWriter, err error) {
	status := 409
	message := "owner authority request rejected"
	if errors.Is(err, goal.ErrUnauthorized) {
		status = 403
		message = "authenticated owner required"
	}
	if errors.Is(err, goal.ErrInvalid) {
		status = 400
		message = "invalid owner authority request"
	}
	if errors.Is(err, actionauth.ErrOwnerAuthority) {
		message = "goal or mandate is stale, inactive or outside scope"
	}
	writeJSON(w, status, map[string]string{"error": message})
}
