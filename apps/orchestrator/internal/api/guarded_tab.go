package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/owneridentity"
)

// GuardedTabDecisions accepts existing owner choices, never a worker binding,
// host, tab, new request, execution grant or continuation instruction.
type GuardedTabDecisions interface {
	Select(context.Context, string, string) (string, error)
	Revoke(context.Context, string) error
}

const guardedTabPrefix = "/api/v3/tab-choices/"

func GuardedTabOwnerControls(auth *OwnerControlAuth, decisions GuardedTabDecisions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		actor, ok := auth.Authenticate(r)
		if !ok || len(r.Header.Values("Authorization")) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "owner control authentication required"})
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, guardedTabPrefix), "/")
		if !strings.HasPrefix(r.URL.Path, guardedTabPrefix) || len(parts) != 2 || len(parts[0]) != 48 || !validChoiceID(parts[0]) || (parts[1] != "select" && parts[1] != "revoke") || r.URL.RawQuery != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "fixed tab choice action required"})
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256))
		// The shared canonical parser rejects duplicate keys before Go decoding
		// can discard one interpretation. The second shape check is exact-case.
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bounded tab choice required"})
			return
		}
		if _, err = actionauth.ArgumentsHash(string(raw)); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "fixed tab choice body required"})
			return
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "fixed tab choice body required"})
			return
		}
		var token string
		if parts[1] == "select" {
			if len(fields) != 1 || json.Unmarshal(fields["candidate_token"], &token) != nil || len(token) != 48 || !validChoiceID(token) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "candidate_token required"})
				return
			}
		} else if len(fields) != 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty revoke body required"})
			return
		}
		if decisions == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "guarded tab owner controls unavailable"})
			return
		}
		ctx := owneridentity.WithActor(r.Context(), actor)
		if parts[1] == "revoke" {
			if decisions.Revoke(ctx, parts[0]) != nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "current tab choice authority required"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"revoked": true})
			return
		}
		session, err := decisions.Select(ctx, parts[0], token)
		if err != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "current tab choice authority required"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"single_tab_session_id": session})
	})
}

func validChoiceID(v string) bool {
	for _, c := range v {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
