package restart

import (
	"encoding/json"
	"errors"
	"net/http"
)

type restartRequest struct {
	Services []string `json:"services"`
}

func NewHandler(service *Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if service == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "restart service is unavailable"})
			return
		}

		var request restartRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
			return
		}

		accepted, err := service.ScheduleRestart(request.Services)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, ErrServiceNotAllowed) || errors.Is(err, ErrSelfRestartDenied) {
				status = http.StatusForbidden
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusAccepted, accepted)
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
