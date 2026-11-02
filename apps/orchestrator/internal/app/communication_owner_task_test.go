package app

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
)

func TestCommunicationWebExactTaskIngressIntegration(t *testing.T) {
	f := newDelegationGuardFixtureWithManifest(t, filepath.Join(t.TempDir(), "deletions"))
	inputs := input.NewStore(f.pool)
	store := &communication.Store{Pool: f.pool, Inputs: inputs, SourceGuard: f.sources.CheckRunSourcesTx}
	c := &telegramCommunication{store: store, binding: telegramrich.OwnerBinding{OwnerID: f.parent.OwnerID, BindingID: "task-bot", Version: 1, BotID: 77, ChatID: 42, UserID: 42}}
	c.installInputHooks()
	if err := c.installWeb(f.ctx, TelegramCommunicationOptions{OwnerID: f.parent.OwnerID, WebBindingID: "task-web"}, f.sources.CheckRunSourcesTx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := session.NewPostgresRepository(f.pool).CreateSession(f.ctx, session.CreateSessionParams{SessionKey: c.webConversationKey(), UserID: f.parent.OwnerID, Channel: "web", MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	tasks := task.NewStore(f.pool)
	makeTask := func() task.Record {
		r, err := tasks.Create(f.ctx, task.CreateRequest{SessionKey: c.webConversationKey(), OwnerID: f.parent.OwnerID, Goal: "Synthetic owner browser task", CompletionCriteria: []string{"Actual delivery checked"}})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	active, stopped := makeTask(), makeTask()
	if _, err := f.pool.Exec(f.ctx, `UPDATE tasks SET status='cancelled' WHERE task_id=$1`, stopped.TaskID); err != nil {
		t.Fatal(err)
	}
	proof := strings.Repeat("t", 40)
	handler := api.CommunicationWeb(api.NewOwnerControlAuth(proof, f.parent.OwnerID), c.web.reader, nil, c.webConversation)
	post := func(delivery, target, token string, want int) {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"text": "Run the bounded synthetic task", "task_id": target})
		q := httptest.NewRequest("POST", "/api/v3/communications/messages", strings.NewReader(string(body)))
		q.Header.Set("Authorization", "Bearer "+token)
		q.Header.Set("X-Butler-Delivery-ID", delivery)
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, q)
		if r.Code != want {
			t.Fatalf("owner task ingress got HTTP %d, want %d", r.Code, want)
		}
	}
	post("task-ingress", active.TaskID, proof, 202)
	post("task-ingress", active.TaskID, proof, 202)
	post("task-ingress", stopped.TaskID, proof, 403)
	post("stopped-task", stopped.TaskID, proof, 403)
	post("foreign-session", f.parent.TaskID, proof, 403)
	post("unknown-task", "task-missing", proof, 403)
	post("oversize", strings.Repeat("x", 129), proof, 400)
	post("unauthenticated", active.TaskID, "wrong-proof", 401)
	c.draining = true
	post("task-ingress", active.TaskID, proof, 202)
	post("draining-new", active.TaskID, proof, 409)
	var count, epoch int
	var target, payloadTarget string
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM accepted_inputs`).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate or denied ingress committed", count, err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT communication_ingress_epoch,task_id,event_json->>'PayloadJSON' FROM accepted_inputs`).Scan(&epoch, &target, &payloadTarget); err != nil || epoch != 1 || target != active.TaskID {
		t.Fatal("lost canonical task or channel proof", err)
	}
	var payload map[string]any
	if json.Unmarshal([]byte(payloadTarget), &payload) != nil || payload["task_id"] != active.TaskID {
		t.Fatal("task target missing from immutable payload")
	}
}
