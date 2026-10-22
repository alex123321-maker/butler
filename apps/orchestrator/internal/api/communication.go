package api

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
)

// CommunicationOwnerControls constructs a dormant backend handler. Neither
// Server nor Bootstrap registers it. Existing owner credentials prove the Web
// actor; Telegram identity remains the responsibility of its trusted adapter.
func CommunicationOwnerControls(auth *OwnerControlAuth, store *communication.Store, controls ...*task.Controller) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		a, ok := auth.Authenticate(r)
		if !ok || len(r.Header.Values("Authorization")) != 1 {
			writeJSON(w, 401, map[string]string{"error": "owner_auth_required"})
			return
		}
		if store == nil {
			writeJSON(w, 503, map[string]string{"error": "communication_unavailable"})
			return
		}
		if r.URL.RawQuery != "" || r.URL.RawPath != "" || r.URL.ForceQuery {
			writeJSON(w, 400, map[string]string{"error": "fixed_communication_path_required"})
			return
		}
		// Authenticated owner scope, never X-Owner-ID, chat ID or model JSON.
		a.SessionKey = "web:" + a.OwnerID
		ctx := owneridentity.WithActor(r.Context(), a)
		if r.URL.Path == "/api/v3/channel-bindings" && r.Method == http.MethodGet {
			v, err := store.Bindings(ctx)
			if err != nil {
				writeJSON(w, 403, map[string]string{"error": "communication_denied"})
				return
			}
			writeJSON(w, 200, v)
			return
		}
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		if len(r.Header.Values("X-Butler-Delivery-ID")) != 1 || strings.TrimSpace(r.Header.Get("X-Butler-Delivery-ID")) == "" || len(a.DeliveryID) > 512 {
			writeJSON(w, 400, map[string]string{"error": "stable_delivery_id_required"})
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32*1024))
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": "invalid_communication_body"})
			return
		}
		bad := func() { writeJSON(w, 400, map[string]string{"error": "invalid_communication_body"}) }
		trimmed := strings.TrimSpace(string(raw))
		if len(trimmed) == 0 || trimmed[0] != '{' {
			bad()
			return
		}
		failed := func(err error) {
			code := 403
			reason := "communication_denied"
			if errors.Is(err, communication.ErrConflict) || errors.Is(err, communication.ErrResolved) {
				code = 409
				reason = "communication_conflict"
			}
			if errors.Is(err, communication.ErrResolved) {
				code = 409
				reason = "already_resolved"
			}
			if errors.Is(err, communication.ErrWidgetStale) {
				code = 409
				reason = "stale_interaction"
			}
			if errors.Is(err, communication.ErrWidgetAnswer) {
				code = 422
				reason = "invalid_answer"
			}
			if errors.Is(err, communication.ErrAmbiguous) {
				code = 409
				reason = "explicit_target_required"
			}
			if errors.Is(err, communication.ErrInteractionRequired) {
				code = 409
				reason = "typed_response_required"
			}
			if errors.Is(err, task.ErrConflict) {
				code = 409
				reason = "communication_conflict"
			}
			if errors.Is(err, communication.ErrDraining) {
				code = 409
				reason = "communication_draining"
			}
			writeJSON(w, code, map[string]string{"error": reason})
		}
		switch {
		case r.URL.Path == "/api/v3/channel-bindings":
			var q struct {
				BindingID       string   `json:"binding_id"`
				ExpectedVersion int64    `json:"expected_version"`
				Enabled         bool     `json:"enabled"`
				Capabilities    []string `json:"capabilities"`
			}
			if communicationcontrol.StrictJSON(raw, &q) != nil {
				bad()
				return
			}
			err = store.Bind(ctx, communication.Binding{ID: q.BindingID, Channel: a.Channel, Recipient: a.SessionKey, Enabled: q.Enabled, Capabilities: q.Capabilities}, q.ExpectedVersion)
			if err != nil {
				failed(err)
				return
			}
			writeJSON(w, 200, map[string]bool{"saved": true})
		case r.URL.Path == "/api/v3/communications/replies":
			var q communication.ReplyRequest
			if communicationcontrol.StrictJSON(raw, &q) != nil {
				bad()
				return
			}
			if kind, _ := task.ClassifyOwnerText(q.Text); kind != "" {
				if len(controls) != 1 || controls[0] == nil {
					writeJSON(w, 503, map[string]string{"error": "owner_control_unavailable"})
					return
				}
				v, err := store.ControlReply(ctx, controls[0], q)
				if err != nil {
					failed(err)
					return
				}
				writeJSON(w, 200, v)
				return
			}
			v, err := store.Reply(ctx, q)
			if err != nil {
				failed(err)
				return
			}
			writeJSON(w, 202, map[string]string{"input_ref": v.InputID, "session_ref": v.SessionKey, "status": v.Status})
		case strings.HasPrefix(r.URL.Path, "/api/v3/interactions/") && strings.HasSuffix(r.URL.Path, "/actions"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v3/interactions/"), "/actions")
			var q communication.WidgetAction
			if !strings.HasPrefix(id, "interaction:") || len(id) > 128 || strings.Contains(id, "/") || communicationcontrol.StrictJSON(raw, &q) != nil {
				bad()
				return
			}
			v, e := store.WidgetAction(ctx, id, q)
			if e != nil {
				failed(e)
				return
			}
			writeJSON(w, 200, v)
		case strings.HasPrefix(r.URL.Path, "/api/v3/interactions/") && strings.HasSuffix(r.URL.Path, "/responses"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v3/interactions/"), "/responses")
			if id == "" || len(id) > 128 || strings.Contains(id, "/") {
				bad()
				return
			}
			var q struct {
				BindingID       string                      `json:"binding_id"`
				BindingVersion  int64                       `json:"binding_version"`
				ExpectedVersion int64                       `json:"expected_version"`
				Answer          communicationcontrol.Answer `json:"answer"`
			}
			if communicationcontrol.StrictJSON(raw, &q) != nil {
				bad()
				return
			}
			v, err := store.Respond(ctx, id, q.BindingID, q.BindingVersion, q.ExpectedVersion, q.Answer)
			if err != nil {
				failed(err)
				return
			}
			writeJSON(w, 200, v)
		default:
			w.WriteHeader(404)
		}
	})
}
