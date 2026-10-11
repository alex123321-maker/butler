package api

import (
	"encoding/json"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/skills"
	"io"
	"net/http"
)

func ReviewedSkills(auth *OwnerControlAuth, client *skills.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := auth.Authenticate(r)
		if !ok {
			http.Error(w, "owner authorization required", http.StatusUnauthorized)
			return
		}
		request := skills.ControlRequest{}
		if r.Method == "GET" {
			request.Operation = "list"
		} else if r.Method == "POST" {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, skills.MaxBytes))
			if err != nil || skills.Decode(body, &request) != nil {
				http.Error(w, "invalid reviewed skill request", http.StatusBadRequest)
				return
			}
		} else {
			http.Error(w, "GET or POST required", http.StatusMethodNotAllowed)
			return
		}
		ctx := owneridentity.WithActor(r.Context(), owneridentity.Actor{OwnerID: actor.OwnerID, Channel: actor.Channel, SessionKey: actor.SessionKey, DeliveryID: actor.DeliveryID})
		result, err := client.Control(ctx, request)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err != nil {
			w.WriteHeader(http.StatusConflict)
			if result.Error == "" {
				result.Error = "reviewed skill control unavailable"
			}
		}
		_ = json.NewEncoder(w).Encode(result)
	})
}
