package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/butler/butler/internal/browsercontrol"
)

// ServerBrowser authenticates a direct human action before reading any input.
// No tool call, transcript, artifact or model continuation contains this body.
func ServerBrowser(auth *OwnerControlAuth, control browsercontrol.Controller) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		actor, ok := auth.Authenticate(r)
		if !ok || len(r.Header.Values("Authorization")) != 1 {
			w.WriteHeader(401)
			return
		}
		if r.Method != http.MethodPost || r.URL.Path != "/api/v4/server-browser" || r.URL.RawQuery != "" {
			w.WriteHeader(400)
			return
		}
		if control == nil {
			w.WriteHeader(503)
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
		if err != nil {
			w.WriteHeader(400)
			return
		}
		command, err := browsercontrol.Decode(raw)
		if err != nil || command.OwnerID != "" || command.RunID != "" {
			w.WriteHeader(400)
			return
		}
		command.OwnerID = actor.OwnerID
		reply, err := control.Control(r.Context(), command)
		if err != nil {
			writeJSON(w, 409, map[string]string{"error": "Browser unavailable or page changed. Reload before another action."})
			return
		}
		_ = json.NewEncoder(w).Encode(reply)
	})
}
