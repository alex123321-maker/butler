package actionauth

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

type mandateRequest struct {
	RunID   string  `json:"run_id"`
	OwnerID string  `json:"owner_id"`
	Mandate Mandate `json:"mandate"`
}

// NewMandateHandler is an administrative service endpoint, not a model tool.
// The service token establishes a trusted operator/service caller; the supplied
// owner and task versions must additionally match authoritative run context.
func NewMandateHandler(token string, store *PostgresStore, reader ContextReader) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(token) < 32 || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			http.Error(w, "private authority authentication required", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodPost && r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var request mandateRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&request) != nil || request.RunID == "" || request.OwnerID == "" {
			http.Error(w, "invalid mandate request", http.StatusBadRequest)
			return
		}
		if store == nil || reader == nil {
			http.Error(w, "mandate authority unavailable", http.StatusServiceUnavailable)
			return
		}
		c, err := reader.GetActionContext(r.Context(), request.RunID)
		if err != nil || c.OwnerID != request.OwnerID || c.TaskID == "" || c.TaskStatus != "active" || c.PinnedTaskRevision != c.TaskRevision {
			http.Error(w, "current owner task authority required", http.StatusForbidden)
			return
		}
		m := request.Mandate
		if m.OwnerID != c.OwnerID || m.TaskID != c.TaskID || m.TaskRevision != c.TaskRevision || m.ConstraintsRevision != c.ConstraintsRevision {
			http.Error(w, "mandate task versions do not match", http.StatusConflict)
			return
		}
		if r.Method == http.MethodDelete {
			err = store.RevokeMandate(r.Context(), m.ID, c.OwnerID, m.Revision)
		} else {
			err = store.PutMandate(r.Context(), m)
		}
		if err != nil {
			http.Error(w, "mandate scope or revision denied", http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
