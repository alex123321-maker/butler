package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
)

// CommunicationWeb is installed only by the isolated owning composition. Bearer
// proof stays in the Authorization header, including streaming and media reads.
func CommunicationWeb(auth *OwnerControlAuth, reader *communication.WebReader, controls *task.Controller, conversation func(context.Context, string, string) (input.Record, error), draining ...bool) http.Handler {
	var store *communication.Store
	if reader != nil {
		store = reader.Core
	}
	owner := CommunicationOwnerControls(auth, store, controls)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		a, ok := auth.Authenticate(r)
		if !ok || len(r.Header.Values("Authorization")) != 1 {
			writeJSON(w, 401, map[string]string{"error": "owner_auth_required"})
			return
		}
		if reader == nil || reader.Core == nil {
			writeJSON(w, 503, map[string]string{"error": "communication_unavailable"})
			return
		}
		a.SessionKey = reader.Binding.Recipient
		ctx := owneridentity.WithActor(r.Context(), a)
		if r.URL.Path == "/api/v3/communications/replies" || r.URL.Path == "/api/v3/channel-bindings" || strings.HasPrefix(r.URL.Path, "/api/v3/interactions/") || strings.HasPrefix(r.URL.Path, "/api/v3/communications/inputs/") {
			owner.ServeHTTP(w, r)
			return
		}
		deny := func() { writeJSON(w, 403, map[string]string{"error": "communication_unavailable"}) }
		if r.URL.Path == "/api/v3/communications/messages" {
			if r.Method != http.MethodPost {
				w.WriteHeader(405)
				return
			}
			if r.URL.RawQuery != "" || len(r.Header.Values("X-Butler-Delivery-ID")) != 1 || strings.TrimSpace(r.Header.Get("X-Butler-Delivery-ID")) == "" || len(a.DeliveryID) > 512 || conversation == nil {
				w.WriteHeader(400)
				return
			}
			var q struct {
				Text   string `json:"text"`
				TaskID string `json:"task_id,omitempty"`
			}
			raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32*1024))
			if err != nil || communicationcontrol.StrictJSON(raw, &q) != nil || !utf8.ValidString(q.Text) || strings.TrimSpace(q.Text) == "" || utf8.RuneCountInString(q.Text) > 4000 {
				w.WriteHeader(400)
				return
			}
			if kind, _ := task.ClassifyOwnerText(q.Text); kind != "" {
				writeJSON(w, 409, map[string]string{"error": "explicit_target_required"})
				return
			}
			if len(q.TaskID) > 128 {
				w.WriteHeader(400)
				return
			}
			v, err := conversation(ctx, q.Text, q.TaskID)
			if err != nil {
				if errors.Is(err, communication.ErrDraining) {
					writeJSON(w, 409, map[string]string{"error": "communication_draining"})
					return
				}
				deny()
				return
			}
			writeJSON(w, 202, map[string]string{"input_id": v.InputID, "session_key": v.SessionKey, "status": v.Status})
			return
		}
		if r.Method != http.MethodGet {
			w.WriteHeader(405)
			return
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil {
			w.WriteHeader(400)
			return
		}
		after := int64(0)
		for k, v := range query {
			if (k != "cursor" && k != "artifact_ref") || len(v) != 1 {
				w.WriteHeader(400)
				return
			}
		}
		if raw := r.URL.Query().Get("cursor"); raw != "" {
			after, err = strconv.ParseInt(raw, 10, 64)
			if err != nil || after < 0 {
				w.WriteHeader(400)
				return
			}
		}
		switch r.URL.Path {
		case "/api/v3/communications/states":
			if r.URL.RawQuery != "" {
				w.WriteHeader(400)
				return
			}
			if reader.Core.ExecutionNotices == nil {
				writeJSON(w, 200, map[string]any{"ready": false, "items": []any{}})
				return
			}
			items, err := reader.Core.ExecutionNotices.List(ctx, reader.Binding.ID)
			if err != nil {
				deny()
				return
			}
			writeJSON(w, 200, map[string]any{"ready": true, "items": items})
		case "/api/v3/communications/profile":
			if _, err = reader.Feed(ctx, 0, 1); err != nil {
				deny()
				return
			}
			writeJSON(w, 200, map[string]any{"ready": true, "binding_id": reader.Binding.ID, "binding_version": reader.Binding.Version, "methods": reader.Binding.Capabilities, "drain_only": len(draining) > 0 && draining[0]})
		case "/api/v3/communications":
			// Keep each complete export proof bounded to one publication. A page
			// of retained history must not hold Session/transcript locks through
			// dozens of later publications while an owning writer has a 5s RPC.
			page, err := reader.Feed(ctx, after, 1)
			if err != nil {
				deny()
				return
			}
			writeJSON(w, 200, page)
		case "/api/v3/communications/events":
			// Reconnection always re-reads REST. SSE carries only a scoped cursor,
			// never content, source refs or authorization. Pulses also refresh
			// per-part outcomes and remove revoked content without a new publish.
			if raw := r.Header.Get("Last-Event-ID"); raw != "" {
				after, err = strconv.ParseInt(raw, 10, 64)
				if err != nil || after < 0 {
					w.WriteHeader(400)
					return
				}
			}
			if _, err = reader.RefreshCursor(ctx, after); err != nil {
				deny()
				return
			}
			flush, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(503)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("X-Accel-Buffering", "no")
			tick := time.NewTicker(2 * time.Second)
			defer tick.Stop()
			deadline := time.NewTimer(24 * time.Second)
			defer deadline.Stop()
			for {
				cursor, err := reader.RefreshCursor(ctx, after)
				if err != nil {
					_, _ = io.WriteString(w, "event: reset\ndata: {}\n\n")
					flush.Flush()
					return
				}
				after = cursor
				data, _ := json.Marshal(map[string]int64{"cursor": after})
				if _, err = fmt.Fprintf(w, "id: %d\nevent: refresh\ndata: %s\n\n", after, data); err != nil {
					return
				}
				flush.Flush()
				select {
				case <-ctx.Done():
					return
				case <-deadline.C:
					return
				case <-tick.C:
				}
			}
		default:
			id := strings.TrimPrefix(r.URL.Path, "/api/v3/communications/")
			media := strings.HasSuffix(id, "/media")
			if media {
				id = strings.TrimSuffix(id, "/media")
			}
			if !strings.HasPrefix(id, "publication:") || len(id) > 128 || strings.Contains(id, "/") {
				w.WriteHeader(404)
				return
			}
			if media {
				m, err := reader.Media(ctx, id, r.URL.Query().Get("artifact_ref"))
				if err != nil {
					deny()
					return
				}
				w.Header().Set("Content-Type", m.MIME)
				w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox; frame-ancestors 'none'")
				disposition := "attachment"
				if m.MIME == "image/png" || m.MIME == "image/jpeg" {
					disposition = "inline"
				}
				w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": m.Name}))
				w.Header().Set("Content-Length", strconv.Itoa(len(m.Data)))
				_, _ = w.Write(m.Data)
				return
			}
			p, err := reader.Detail(ctx, id)
			if err != nil {
				deny()
				return
			}
			writeJSON(w, 200, p)
		}
	})
}
