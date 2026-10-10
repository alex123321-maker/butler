package app

import (
	"encoding/json"
	"github.com/butler/butler/internal/health"
	"net/http"
	"time"
)

// This fixed read-only adapter exposes status enums, never credentials, raw
// upstream errors, configurable queries or authority operations to Doctor.
func coreDiagnostics(deps httpServerDeps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		report := deps.health.Check(r.Context())
		checks := []health.CheckResult{}
		for _, check := range report.Checks {
			if check.Name == "postgres" || check.Name == "redis" {
				check.Message = ""
				checks = append(checks, check)
			}
		}
		provider := health.CheckResult{Name: "provider", Status: health.StatusDegraded, Message: "provider credentials not configured", CheckedAt: time.Now().UTC().Format(time.RFC3339), Duration: "0s"}
		configured := deps.providerResult.ProviderName == "openai" && deps.cfg.OpenAIAPIKey != ""
		if deps.authManager != nil && !configured {
			state, err := deps.authManager.State(r.Context(), deps.providerResult.ProviderName)
			if err == nil {
				configured = state.Connected
			}
		}
		if configured {
			provider.Status = health.StatusHealthy
			provider.Message = "credential readiness; inference availability is not probed"
		}
		checks = append(checks, provider)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(health.Report{Status: report.Status, Service: "orchestrator", CheckedAt: report.CheckedAt, Checks: checks})
	})
}
