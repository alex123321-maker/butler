package api

import (
	"context"
	"net/http"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/owneridentity"
)

type CommunicationDoctorReporter interface {
	CommunicationReport(context.Context) (communication.DiagnosticReport, error)
}

// Owner metadata only; no model/tool endpoint or mutation capability is exposed.
func CommunicationDiagnostics(auth *OwnerControlAuth, reporter CommunicationDoctorReporter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		a, ok := auth.Authenticate(r)
		if !ok || len(r.Header.Values("Authorization")) != 1 {
			writeJSON(w, 401, map[string]string{"error": "owner_auth_required"})
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" || r.URL.Path != "/api/v3/communications/diagnostics" {
			w.WriteHeader(400)
			return
		}
		if reporter == nil {
			writeJSON(w, 503, map[string]string{"error": "communication_observation_unavailable"})
			return
		}
		out, err := reporter.CommunicationReport(owneridentity.WithActor(r.Context(), a))
		if err != nil {
			writeJSON(w, 503, map[string]string{"error": "communication_observation_unavailable"})
			return
		}
		writeJSON(w, 200, out)
	})
}
