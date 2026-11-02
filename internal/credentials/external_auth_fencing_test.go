package credentials

import (
	"net/url"
	"strings"
	"testing"

	"github.com/butler/butler/internal/externalauth"
)

func TestVaultExternalAuthCurrentAuthorityAndCancellation(t *testing.T) {
	changes := map[string]string{
		"task_revision":     `UPDATE tasks SET revision=revision+1`,
		"constraints":       `UPDATE tasks SET constraints_revision=constraints_revision+1`,
		"mandate_revoked":   `UPDATE action_mandates SET status='revoked'`,
		"goal_revision":     `UPDATE owner_goals SET revision=revision+1`,
		"record_revision":   `UPDATE credential_vault SET revision=revision+1`,
		"owner_takeover":    `UPDATE server_browser_sessions SET mode='owner',epoch=epoch+1`,
		"source_navigation": `UPDATE server_browser_pages SET document_id=repeat('e',32),observed_url='https://foreign.example/',observation_version=observation_version+1`,
		"source_version":    `UPDATE server_browser_pages SET observation_version=observation_version+1`,
		"foreign_input":     `UPDATE accepted_inputs SET checkpoint_json='{}'`,
		"profile_disabled":  `UPDATE browser_external_auth_profiles SET active=false`,
		"cancelled":         "",
		"budget_exhausted":  "",
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			v, p, entry, _, _, args := externalFixture(t)
			q := createExternalFixtureRequest(t, v, p, args)
			provider := &externalProviderFixture{original: v.external.google}
			v.external.google = provider
			launch, e := v.RespondExternal(t.Context(), p.OwnerID, q.ID, entry.ID, true, externalauth.Consent, q.ScopeDigest)
			if e != nil {
				t.Fatal("protected decision", e)
			}
			u, _ := url.Parse(launch.URL)
			recovered, e := v.ExternalLaunch(t.Context(), p.OwnerID, q.ID, q.ScopeDigest)
			if e != nil || recovered.URL != launch.URL {
				t.Fatal("reload changed the granted launch")
			}
			start, e := v.StartExternal(t.Context(), u.Query().Get("ticket"))
			if e != nil {
				t.Fatal("start", e)
			}
			auth, _ := url.Parse(start.URL)
			if name == "cancelled" {
				if v.CancelExternal(t.Context(), "foreign", q.ID, q.ScopeDigest) == nil || v.CancelExternal(t.Context(), p.OwnerID, q.ID, strings.Repeat("e", 64)) == nil {
					t.Fatal("foreign cancellation admitted")
				}
				if v.CancelExternal(t.Context(), p.OwnerID, q.ID, q.ScopeDigest) != nil {
					t.Fatal("cancel")
				}
			} else if name == "budget_exhausted" {
				v.external.budgets.Policy.Task.ExternalCalls = 0
			} else {
				if _, e := v.Pool.Exec(t.Context(), change); e != nil {
					t.Fatal("authority change fixture", e)
				}
			}
			if v.CompleteExternalAuth(t.Context(), auth.Query().Get("state"), "synthetic-code-canary", start.CookieValue, false) == nil || provider.calls.Load() != 0 {
				t.Fatal("changed authority reached provider")
			}
			var n int
			if v.Pool.QueryRow(t.Context(), `SELECT count(*) FROM browser_external_auth_exchanges`).Scan(&n) != nil || n != 0 {
				t.Fatal("undispatched operation retained a spent exchange")
			}
			if name == "cancelled" || name == "budget_exhausted" {
				var state string
				if v.Pool.QueryRow(t.Context(), `SELECT live_state FROM browser_external_auth_state WHERE id=$1`, q.ID).Scan(&state) != nil || state != map[string]string{"cancelled": "cancelled", "budget_exhausted": "rejected"}[name] {
					t.Fatal("attempt did not close", state)
				}
			}
		})
	}
}
