package actionauth

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

// Effect endpoints are private service operations, absent from the model tool
// registry. Reconciliation requires a captured authorized observation digest.
func NewEffectHandler(token string, s *Service, store *PostgresStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(token) < 32 || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			http.Error(w, "private authority authentication required", http.StatusUnauthorized)
			return
		}
		if s == nil || store == nil {
			http.Error(w, "effect journal unavailable", http.StatusServiceUnavailable)
			return
		}
		if r.Method == http.MethodGet {
			records, err := store.GetRunEvidence(r.Context(), r.URL.Query().Get("run_id"))
			if err != nil {
				http.Error(w, "effect journal unavailable", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(records)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var request struct {
			ActionID                   string `json:"action_id"`
			ObservationAuthorizationID string `json:"observation_authorization_id"`
			ResultJSON                 string `json:"result_json"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512*1024))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || request.ActionID == "" || request.ObservationAuthorizationID == "" {
			http.Error(w, "invalid observation reference", http.StatusBadRequest)
			return
		}
		grant, err := store.GetGrant(r.Context(), request.ObservationAuthorizationID)
		if err != nil {
			http.Error(w, "observation unavailable", http.StatusConflict)
			return
		}
		record, err := s.Reconcile(r.Context(), request.ActionID, grant, &toolbrokerv1.ToolResult{RunId: grant.Binding.RunID, ToolCallId: grant.Binding.ToolCallID, ToolName: grant.Binding.ToolName, Status: "completed", ResultJson: request.ResultJSON})
		if err != nil {
			http.Error(w, "independently captured observation does not prove this effect", http.StatusConflict)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(RunEffect{record.ActionID, record.State, record.EvidenceRefs})
	})
}
