package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/owneridentity"
)

func GeneratedConverters(auth *OwnerControlAuth, client *generatedtools.Client) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a, ok := auth.Authenticate(r)
		if !ok {
			http.Error(w, "owner authorization required", 401)
			return
		}
		if r.URL.RawQuery != "" {
			http.Error(w, "fixed converter route required", 400)
			return
		}
		req := generatedtools.ControlRequest{}
		if r.Method == "GET" {
			req.Operation = "list"
		} else if r.Method == "POST" {
			raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
			if e != nil || generatedtools.Decode(raw, &req) != nil {
				http.Error(w, "invalid converter request", 400)
				return
			}
		} else {
			http.Error(w, "GET or POST required", 405)
			return
		}
		ctx := owneridentity.WithActor(r.Context(), owneridentity.Actor{OwnerID: a.OwnerID, Channel: a.Channel, SessionKey: a.SessionKey, DeliveryID: a.DeliveryID})
		res, e := client.Control(ctx, req)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if e != nil {
			w.WriteHeader(409)
			res.Error = generatedtools.SafeError(e)
		}
		_ = json.NewEncoder(w).Encode(res)
	})
}
