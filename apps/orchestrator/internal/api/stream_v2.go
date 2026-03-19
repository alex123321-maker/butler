package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/observability"
	"github.com/butler/butler/apps/orchestrator/internal/run"
)

type StreamServer struct {
	hub       *observability.Hub
	tasks     *TaskViewServer
	overview  *OverviewServer
	approvals *ApprovalsServer
	system    *SystemServer
	activity  *ActivityServer
}

func NewStreamServer(hub *observability.Hub, tasks *TaskViewServer, overview *OverviewServer, approvals *ApprovalsServer, system *SystemServer, activity *ActivityServer) *StreamServer {
	return &StreamServer{hub: hub, tasks: tasks, overview: overview, approvals: approvals, system: system, activity: activity}
}

func (s *StreamServer) HandleStream() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		topics := parseTopics(r.URL.Query().Get("topics"))
		typeFilter := strings.TrimSpace(r.URL.Query().Get("type"))

		// Initial snapshot events so frontend can render without N requests.
		s.emitInitialSnapshots(r.Context(), w, topics)
		flusher.Flush()

		if s.hub == nil {
			writeGenericSSE(w, "system.updated", map[string]any{"partial": true, "reason": "event_hub_unavailable"})
			flusher.Flush()
			return
		}

		hb := time.NewTicker(15 * time.Second)
		defer hb.Stop()

		// run-scoped hub requires active run IDs. We poll active task IDs and subscribe lazily.
		subs := make(map[string]<-chan observability.Event)
		cancels := make(map[string]func())
		defer func() {
			for _, cancel := range cancels {
				cancel()
			}
		}()

		refresh := time.NewTicker(5 * time.Second)
		defer refresh.Stop()

		refreshSubs := func() {
			runIDs := s.activeRunIDs(r.Context())
			runSet := make(map[string]struct{}, len(runIDs))
			for _, id := range runIDs {
				runSet[id] = struct{}{}
				if _, exists := subs[id]; !exists {
					ch, cancel := s.hub.Subscribe(id)
					subs[id] = ch
					cancels[id] = cancel
				}
			}
			for id, cancel := range cancels {
				if _, keep := runSet[id]; !keep {
					cancel()
					delete(cancels, id)
					delete(subs, id)
				}
			}
		}

		refreshSubs()

		type queued struct {
			eventType string
			payload   map[string]any
		}

		for {
			select {
			case <-r.Context().Done():
				return
			case <-hb.C:
				fmt.Fprintf(w, ": heartbeat\n\n")
				flusher.Flush()
			case <-refresh.C:
				refreshSubs()
				s.emitSnapshotUpdates(r.Context(), w, topics)
				flusher.Flush()
			default:
				queue := make([]queued, 0)
				for runID, ch := range subs {
					select {
					case evt, ok := <-ch:
						if !ok {
							if cancel, exists := cancels[runID]; exists {
								cancel()
								delete(cancels, runID)
								delete(subs, runID)
							}
							continue
						}
						for _, item := range s.mapObservabilityToStreamEvents(evt, topics) {
							if typeFilter != "" && item.eventType != typeFilter {
								continue
							}
							queue = append(queue, item)
						}
					default:
					}
				}
				if len(queue) > 0 {
					for _, item := range queue {
						writeGenericSSE(w, item.eventType, item.payload)
					}
					flusher.Flush()
				} else {
					time.Sleep(40 * time.Millisecond)
				}
			}
		}
	})
}

func (s *StreamServer) emitInitialSnapshots(ctx context.Context, w http.ResponseWriter, topics map[string]struct{}) {
	s.emitSnapshotUpdates(ctx, w, topics)
	writeGenericSSE(w, "system.updated", map[string]any{"fallback": "manual_refresh_or_polling"})
}

func (s *StreamServer) emitSnapshotUpdates(ctx context.Context, w http.ResponseWriter, topics map[string]struct{}) {
	if hasTopic(topics, "overview") && s.overview != nil {
		payload := captureJSONFromHandler(ctx, s.overview.HandleGetOverview())
		writeGenericSSE(w, "overview.updated", payload)
	}
	if hasTopic(topics, "tasks") && s.tasks != nil {
		payload := captureJSONFromHandler(ctx, s.tasks.HandleListTasks())
		writeGenericSSE(w, "tasks.updated", payload)
	}
	if hasTopic(topics, "approvals") && s.approvals != nil {
		payload := captureJSONFromHandler(ctx, s.approvals.HandleListApprovals())
		writeGenericSSE(w, "approvals.updated", payload)
	}
	if hasTopic(topics, "system") && s.system != nil {
		payload := captureJSONFromHandler(ctx, s.system.HandleGetSystem())
		writeGenericSSE(w, "system.updated", payload)
	}
	if hasTopic(topics, "activity") && s.activity != nil {
		payload := captureJSONFromHandler(ctx, s.activity.HandleListActivity())
		writeGenericSSE(w, "activity.updated", payload)
	}
}

func (s *StreamServer) mapObservabilityToStreamEvents(evt observability.Event, topics map[string]struct{}) []struct {
	eventType string
	payload   map[string]any
} {
	type mapped struct {
		eventType string
		payload   map[string]any
	}
	items := make([]mapped, 0, 4)
	switch evt.EventType {
	case observability.EventStateTransition, observability.EventRunCompleted, observability.EventRunError:
		if hasTopic(topics, "tasks") {
			items = append(items, mapped{eventType: "task.updated", payload: map[string]any{"run_id": evt.RunID, "event_type": evt.EventType}})
		}
		if hasTopic(topics, "overview") {
			items = append(items, mapped{eventType: "overview.updated", payload: map[string]any{"run_id": evt.RunID, "event_type": evt.EventType}})
		}
		if hasTopic(topics, "activity") {
			items = append(items, mapped{eventType: "activity.created", payload: map[string]any{"run_id": evt.RunID, "event_type": evt.EventType}})
		}
	case observability.EventApprovalRequested:
		if hasTopic(topics, "approvals") {
			items = append(items, mapped{eventType: "approval.created", payload: map[string]any{"run_id": evt.RunID, "event_type": evt.EventType}})
		}
		if hasTopic(topics, "activity") {
			items = append(items, mapped{eventType: "activity.created", payload: map[string]any{"run_id": evt.RunID, "event_type": evt.EventType}})
		}
	case observability.EventApprovalResolved:
		if hasTopic(topics, "approvals") {
			items = append(items, mapped{eventType: "approval.resolved", payload: map[string]any{"run_id": evt.RunID, "event_type": evt.EventType}})
		}
		if hasTopic(topics, "tasks") {
			items = append(items, mapped{eventType: "task.updated", payload: map[string]any{"run_id": evt.RunID, "event_type": evt.EventType}})
		}
	case observability.EventAssistantFinal:
		if hasTopic(topics, "tasks") {
			items = append(items, mapped{eventType: "artifact.created", payload: map[string]any{"run_id": evt.RunID, "artifact_type": "assistant_final"}})
		}
		if hasTopic(topics, "overview") {
			items = append(items, mapped{eventType: "overview.updated", payload: map[string]any{"run_id": evt.RunID, "event_type": evt.EventType}})
		}
	case observability.EventMemoryLoaded:
		if hasTopic(topics, "activity") {
			items = append(items, mapped{eventType: "memory.updated", payload: map[string]any{"run_id": evt.RunID}})
		}
	}

	// Convert named struct slice to anonymous return type.
	result := make([]struct {
		eventType string
		payload   map[string]any
	}, 0, len(items))
	for _, item := range items {
		result = append(result, struct {
			eventType string
			payload   map[string]any
		}{eventType: item.eventType, payload: item.payload})
	}
	return result
}

func (s *StreamServer) activeRunIDs(ctx context.Context) []string {
	if s.tasks == nil || s.tasks.tasks == nil {
		return []string{}
	}
	rows, err := s.tasks.tasks.ListTasks(ctx, run.TaskListParams{Status: "in_progress", Limit: 100, Sort: run.TaskSortUpdatedAtDesc})
	if err != nil {
		return []string{}
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.RunID) != "" {
			ids = append(ids, row.RunID)
		}
	}
	ids = uniqueStrings(ids)
	sort.Strings(ids)
	return ids
}

func writeGenericSSE(w http.ResponseWriter, eventType string, payload map[string]any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, string(raw))
}

func captureJSONFromHandler(ctx context.Context, handler http.Handler) map[string]any {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://local/internal", nil)
	rr := &captureResponseWriter{header: http.Header{}, body: strings.Builder{}}
	handler.ServeHTTP(rr, req)
	if rr.status >= 400 {
		return map[string]any{"partial": true, "status": rr.status}
	}
	if strings.TrimSpace(rr.body.String()) == "" {
		return map[string]any{}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(rr.body.String()), &payload); err != nil {
		return map[string]any{"partial": true, "decode_error": true}
	}
	return payload
}

type captureResponseWriter struct {
	header http.Header
	body   strings.Builder
	status int
	mu     sync.Mutex
}

func (w *captureResponseWriter) Header() http.Header { return w.header }
func (w *captureResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}
func (w *captureResponseWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(data)
}

func parseTopics(raw string) map[string]struct{} {
	topics := map[string]struct{}{}
	for _, part := range strings.Split(strings.TrimSpace(raw), ",") {
		value := strings.TrimSpace(strings.ToLower(part))
		if value != "" {
			topics[value] = struct{}{}
		}
	}
	if len(topics) == 0 {
		topics["overview"] = struct{}{}
		topics["tasks"] = struct{}{}
		topics["approvals"] = struct{}{}
		topics["system"] = struct{}{}
		topics["activity"] = struct{}{}
	}
	return topics
}

func hasTopic(topics map[string]struct{}, name string) bool {
	_, ok := topics[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}
