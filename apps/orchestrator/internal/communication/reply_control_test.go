package communication_test

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
)

func replyController(f *fixture) *task.Controller {
	c := task.NewController(task.NewStore(f.pool), f.leases.Authority(), f.leases, run.NewPostgresRepository(f.pool), approval.NewPostgresRepository(f.pool), f.s.Inputs, transcript.NewStore(f.pool))
	c.SetInteractionControls(f.s)
	return c
}

func controlReceipts(t *testing.T, f *fixture) int {
	t.Helper()
	var n int
	if e := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM task_control_events WHERE task_id=$1`, f.p.TaskID).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}

func TestCommunicationCrossChannelOwnerControlBypassesWaitingFIFOIntegration(t *testing.T) {
	for _, scenario := range []string{"stop", "pause", "prepare_only", "ambiguous", "stale_binding", "expired_mandate"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t, fixtureOptions{Task: true, Authority: scenario == "expired_mandate"})
			p, e := f.publish(t, "question", "communication.widget", widget)
			if e != nil {
				t.Fatal(e)
			}
			if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
				t.Fatal(e)
			}
			d := &communication.DeliveryStore{Core: f.s}
			for i := 0; i < 2; i++ {
				m, e := d.Claim(f.ctx, time.Minute)
				if e != nil || m == nil {
					t.Fatal(m, e)
				}
				if e = d.AuthorizeSend(f.ctx, *m); e != nil {
					t.Fatal(e)
				}
				if e = d.Finish(f.ctx, *m, deliveryoutbox.Outcome{Status: "sent", ACK: m.Channel + ":question"}); e != nil {
					t.Fatal(e)
				}
			}
			text := "стоп"
			if scenario == "pause" {
				text = "пауза"
			}
			if scenario == "prepare_only" {
				text = "не отправляй, только подготовь"
			}
			q := communication.ReplyRequest{BindingID: f.bindings["telegram"], BindingVersion: 1, ChannelACK: "telegram:question", Text: text}
			ctx := f.actor("telegram", "owner-control")
			c := replyController(f)
			if _, e = f.s.Reply(ctx, q); !errors.Is(e, communication.ErrOwnerControlRequired) {
				t.Fatal("control queued as ordinary input", e)
			}
			if scenario == "ambiguous" {
				q.ChannelACK = ""
			}
			if scenario == "stale_binding" {
				if e = f.s.Bind(f.actor("telegram", "disable"), communication.Binding{ID: q.BindingID, Channel: "telegram", Recipient: "telegram:verified", Enabled: false, Capabilities: []string{"widget"}}, 1); e != nil {
					t.Fatal(e)
				}
			}
			if scenario == "expired_mandate" {
				if _, e = f.pool.Exec(f.ctx, `UPDATE action_mandates SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE mandate_id=(SELECT mandate_id FROM task_authority_bindings WHERE task_id=$1)`, f.p.TaskID); e != nil {
					t.Fatal(e)
				}
			}
			result, e := f.s.ControlReply(ctx, c, q)
			if scenario == "ambiguous" || scenario == "stale_binding" {
				if e == nil {
					t.Fatal("unproved control mutated task")
				}
				if controlReceipts(t, f) != 0 {
					t.Fatal("denied control receipt")
				}
				return
			}
			if e != nil || result.Task.TaskID != f.p.TaskID {
				t.Fatal("proved cross-channel control failed", result, e)
			}
			if again, e := f.s.ControlReply(ctx, c, q); e != nil || again.Task.Revision != result.Task.Revision {
				t.Fatal("control replay duplicated authority", again, e)
			}
			var state string
			if e = f.pool.QueryRow(f.ctx, `SELECT state FROM communication_interactions WHERE interaction_id=$1`, p.InteractionRef).Scan(&state); e != nil {
				t.Fatal(e)
			}
			if scenario == "pause" {
				if state != "open" || result.Task.Status != "paused" {
					t.Fatal("pause discarded wait", state, result)
				}
			} else if state != "cancelled" && state != "superseded" {
				t.Fatal("control left old question open", state)
			}
			if controlReceipts(t, f) != 1 {
				t.Fatal("control replayed")
			}
		})
	}
}

func TestCommunicationOwnerHTTPRecognizesStopWithoutQueuingIntegration(t *testing.T) {
	f := setup(t, fixtureOptions{Task: true})
	token := strings.Repeat("owner", 9)
	a := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", SessionKey: "web:" + f.p.OwnerID, DeliveryID: "api-bind"})
	if e := f.s.Bind(a, communication.Binding{ID: "owner-http-control", Channel: "web", Recipient: "web:" + f.p.OwnerID, Enabled: true, Capabilities: []string{"widget"}}, 0); e != nil {
		t.Fatal(e)
	}
	p, e := f.publish(t, "question", "communication.widget", widget)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
		t.Fatal(e)
	}
	h := api.CommunicationOwnerControls(api.NewOwnerControlAuth(token, f.p.OwnerID), f.s, replyController(f))
	body := `{"binding_id":"owner-http-control","binding_version":1,"publication_ref":"` + p.PublicationRef + `","text":"стоп"}`
	r := httptest.NewRequest("POST", "/api/v3/communications/replies", strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Butler-Delivery-ID", "stop-via-publication")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var active int
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM accepted_inputs WHERE session_key=$1 AND status IN ('queued','running','waiting_for_user')`, f.p.SessionKey).Scan(&active); e != nil || active != 0 {
		t.Fatal("stop added FIFO work")
	}
	record, e := f.s.Inputs.Get(f.ctx, f.p.InputID)
	if e != nil || record.Status != "cancelled" {
		t.Fatal("waiting input not stopped", record, e)
	}
}
