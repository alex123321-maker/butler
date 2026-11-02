package api

import (
	"io"
	"net/http"

	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/credentials"
)

// CredentialVault accepts secret input outside event/transcript/communication ingress.
// Fixed errors deliberately discard request bodies and database diagnostics.
func CredentialVault(auth *OwnerControlAuth, vault *credentials.Vault) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		actor, ok := auth.Authenticate(r)
		if !ok || len(r.Header.Values("Authorization")) != 1 {
			writeJSON(w, 401, map[string]string{"error": "owner_auth_required"})
			return
		}
		if vault == nil {
			writeJSON(w, 503, map[string]string{"error": "vault_unavailable"})
			return
		}
		if r.URL.RawQuery != "" || r.URL.RawPath != "" || r.URL.Path != "/api/v4/credentials" {
			w.WriteHeader(400)
			return
		}
		if r.Method == http.MethodGet {
			configured, err := vault.PinConfigured(r.Context(), actor.OwnerID)
			if err != nil {
				writeJSON(w, 503, map[string]string{"error": "vault_unavailable"})
				return
			}
			pending, err := vault.Requests(r.Context(), actor.OwnerID)
			if err != nil {
				writeJSON(w, 503, map[string]string{"error": "vault_unavailable"})
				return
			}
			writeJSON(w, 200, map[string]any{"configured": configured, "requests": pending})
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		var q struct {
			Action             string                  `json:"action"`
			PIN                string                  `json:"pin"`
			Confirm            string                  `json:"confirm"`
			Query              string                  `json:"query"`
			Entry              *credentials.EntryInput `json:"entry"`
			ID                 string                  `json:"id"`
			Revision           int64                   `json:"revision"`
			Allow              bool                    `json:"allow"`
			RecordID           string                  `json:"record_id"`
			OTP                string                  `json:"otp"`
			LoginAuthorization string                  `json:"login_authorization"`
			SMSRoute           *credentials.SMSRoute   `json:"sms_route"`
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32*1024))
		if err != nil || communicationcontrol.StrictJSON(raw, &q) != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_credential_request"})
			return
		}
		if q.Action == "unlock" {
			token, err := vault.Unlock(r.Context(), actor.OwnerID, q.PIN, q.Confirm)
			if err != nil {
				writeJSON(w, 403, map[string]string{"error": "pin_rejected"})
				return
			}
			writeJSON(w, 200, map[string]string{"unlock_token": token})
			return
		}
		token := r.Header.Get("X-Butler-Vault")
		if len(r.Header.Values("X-Butler-Vault")) != 1 || vault.CheckUnlock(r.Context(), actor.OwnerID, token) != nil {
			writeJSON(w, 423, map[string]string{"error": "vault_locked"})
			return
		}
		deny := func() { writeJSON(w, 409, map[string]string{"error": "credential_operation_unavailable"}) }
		switch q.Action {
		case "sms_routes":
			routes, err := vault.SMSRoutes(r.Context(), actor.OwnerID)
			if err != nil {
				deny()
				return
			}
			devices, err := vault.SMSDevices(r.Context(), actor.OwnerID)
			if err != nil {
				deny()
				return
			}
			writeJSON(w, 200, map[string]any{"routes": routes, "devices": devices})
			return
		case "sms_route":
			if q.SMSRoute == nil || vault.SaveSMSRoute(r.Context(), actor.OwnerID, *q.SMSRoute) != nil {
				deny()
				return
			}
		case "change_pin":
			if vault.ChangePIN(r.Context(), actor.OwnerID, q.PIN, q.Confirm) != nil {
				deny()
				return
			}
		case "lock":
			if vault.Lock(r.Context(), actor.OwnerID, token) != nil {
				deny()
				return
			}
		case "list":
			entries, e := vault.List(r.Context(), actor.OwnerID, q.Query)
			if e != nil {
				deny()
				return
			}
			writeJSON(w, 200, map[string]any{"entries": entries})
			return
		case "save":
			if q.Entry == nil {
				deny()
				return
			}
			entry, e := vault.Save(r.Context(), actor.OwnerID, *q.Entry)
			if e != nil {
				deny()
				return
			}
			writeJSON(w, 200, map[string]any{"entry": entry})
			return
		case "revoke", "delete":
			if vault.Remove(r.Context(), actor.OwnerID, q.ID, q.Revision, q.Action == "delete") != nil {
				deny()
				return
			}
		case "requests":
			requests, e := vault.Requests(r.Context(), actor.OwnerID)
			if e != nil {
				deny()
				return
			}
			writeJSON(w, 200, map[string]any{"requests": requests})
			return
		case "respond":
			if vault.Respond(r.Context(), actor.OwnerID, q.ID, q.RecordID, q.Allow, q.OTP, q.LoginAuthorization) != nil {
				deny()
				return
			}
		default:
			writeJSON(w, 400, map[string]string{"error": "invalid_credential_request"})
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	})
}
