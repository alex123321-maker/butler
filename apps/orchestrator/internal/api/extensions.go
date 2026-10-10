package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/butler/butler/internal/extensions"
	"github.com/butler/butler/internal/owneridentity"
)

func ReviewedExtensions(auth *OwnerControlAuth, client *extensions.ControlClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor, ok := auth.Authenticate(r)
		if !ok {
			http.Error(w, "owner authorization required", http.StatusUnauthorized)
			return
		}
		request := extensions.ControlRequest{}
		if r.Method == "GET" {
			request.Operation = "list"
		} else if r.Method == "POST" {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, extensions.MaxManifestBytes+128*1024))
			if err != nil {
				http.Error(w, "reviewed extension request too large", http.StatusBadRequest)
				return
			}
			if extensions.DecodeControlRequest(body, &request) != nil {
				http.Error(w, "invalid reviewed extension request", http.StatusBadRequest)
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
				result.Error = "reviewed extension control unavailable"
			}
		}
		_ = json.NewEncoder(w).Encode(result)
	})
}
