package telegramrich

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	runstore "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/jackc/pgx/v5"
)

// These are channel-edit fixtures, not publication/authority acceptance. Owning
// publication, source revocation and resume are separately tested through Store.
func widgetProjectionFixture(t *testing.T, c *Client) (*Poller, *communication.Store) {
	t.Helper()
	pool := memorytest.Database(t)
	ctx := context.Background()
	key, run, owner := "web:projection", "run-projection", "projection-owner"
	_, _, e := session.NewPostgresRepository(pool).CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: owner, Channel: "web", MetadataJSON: `{}`})
	if e != nil {
		t.Fatal(e)
	}
	_, e = runstore.NewPostgresRepository(pool).CreateRun(ctx, runstore.Record{RunID: run, SessionKey: key, InputEventID: "projection", Status: "awaiting_approval", CurrentState: "awaiting_approval", AutonomyMode: "mode_1", ModelProvider: "fixture", StartedAt: time.Now(), UpdatedAt: time.Now(), MetadataJSON: `{}`})
	if e != nil {
		t.Fatal(e)
	}
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, e := pool.Exec(ctx, sql, args...); e != nil {
			t.Fatal(e)
		}
	}
	exec(`INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,run_id,status,checkpoint) VALUES('projection-input',$1,'projection','projection','web',$2,'{}',$3,'waiting_for_user','awaiting_interaction')`, key, owner, run)
	exec(`INSERT INTO communication_bindings(binding_id,owner_id,channel,recipient,version,enabled,verified,capabilities) VALUES('projection-binding',$1,'telegram','telegram:42',1,true,true,'["widget"]')`, owner)
	exec(`INSERT INTO communication_admissions(input_id,owner_id,session_key,schema_epoch,methods,obligation_state) VALUES('projection-input',$1,$2,1,'["communication.widget"]','waiting_for_user')`, owner, key)
	question := `{"schema_version":1,"question":"Какую пиццу?","input":{"type":"multi_choice","options":[{"id":"a","label":"Маргарита"},{"id":"b","label":"Пепперони"}]}}`
	exec(`INSERT INTO communication_publications(publication_id,effect_id,tool_call_id,input_id,owner_id,session_key,run_id,task_revision,constraints_revision,kind,purpose,payload,payload_hash,source_refs,receipt_json) VALUES('publication:projection','projection','projection','projection-input',$1,$2,$3,0,0,'widget','question',$4,'synthetic','[]','{}')`, owner, key, run, question)
	exec(`INSERT INTO communication_interactions(interaction_id,publication_id,input_id,owner_id,session_key,run_id,task_revision,constraints_revision,question_json,state,version) VALUES('interaction:projection','publication:projection','projection-input',$1,$2,$3,0,0,$4,'resolved',2)`, owner, key, run, question)
	exec(`INSERT INTO communication_widget_views(interaction_id,callback_token) VALUES('interaction:projection','aaaaaaaaaaaaaaaaaaaaaaaa')`)
	part, _ := json.Marshal(Part{Version: 1, Method: "sendMessage", Text: "Какую пиццу?", Widget: true})
	exec(`INSERT INTO delivery_outbox(notification_id,run_id,session_key,channel,chunk_index,content,status,deadline_at,publication_id,binding_id,binding_version,recipient_snapshot,representation_json,channel_ack) VALUES('projection-notification',$1,$2,'telegram',0,$3,'sent',clock_timestamp()+INTERVAL '1 day','publication:projection','projection-binding',1,'telegram:42',$4,'7')`, run, key, string(part), question)
	core := &communication.Store{Pool: pool, Inputs: durableinput.NewStore(pool), WidgetProjections: true, SourceGuard: func(context.Context, pgx.Tx, string) error { return nil }, ChannelGuard: func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
		return nil
	}}
	b := OwnerBinding{OwnerID: owner, BindingID: "projection-binding", Version: 1, BotID: 77, ChatID: 42, UserID: 42}
	return &Poller{Pool: pool, Client: c, Binding: b, Acceptor: &Ingress{Store: core, Binding: b, Widgets: true}}, core
}

func TestCommunicationWidgetProjectionFailureJournalIntegration(t *testing.T) {
	for _, scenario := range []string{"sent", "lost_ack", "wrong_ack", "rejected", "process_loss", "revoked"} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			c := client(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if !strings.HasSuffix(r.URL.Path, "/editMessageText") {
					t.Error("edit became a new publication")
				}
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if body["message_id"] != "7" || !strings.Contains(body["text"].(string), "Вопрос закрыт") {
					t.Error("wrong target/state", body)
				}
				switch scenario {
				case "lost_ack":
					_, _ = w.Write([]byte(`{`))
				case "wrong_ack":
					_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":8,"chat":{"id":42}}}`))
				case "rejected":
					w.WriteHeader(429)
					_, _ = w.Write([]byte(`{"ok":false,"error_code":429}`))
				default:
					_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"chat":{"id":42}}}`))
				}
			})
			p, core := widgetProjectionFixture(t, c)
			if scenario == "process_loss" {
				if _, e := p.Pool.Exec(context.Background(), `INSERT INTO communication_widget_projection_ops(notification_id,revision,status,created_at) VALUES('projection-notification','2:1:resolved','sending',clock_timestamp()-INTERVAL '31 seconds')`); e != nil {
					t.Fatal(e)
				}
			}
			if scenario == "revoked" {
				core.SourceGuard = func(context.Context, pgx.Tx, string) error { return errors.New("revoked") }
			}
			if e := p.RefreshWidgets(context.Background()); e != nil {
				t.Fatal(e)
			}
			if e := p.RefreshWidgets(context.Background()); e != nil {
				t.Fatal(e)
			}
			if scenario == "revoked" {
				if calls.Load() != 0 {
					t.Fatal("revoked edit")
				}
				return
			}
			var status string
			if e := p.Pool.QueryRow(context.Background(), `SELECT status FROM communication_widget_projection_ops`).Scan(&status); e != nil {
				t.Fatal(e)
			}
			want := "sent"
			if scenario == "lost_ack" || scenario == "wrong_ack" || scenario == "process_loss" {
				want = "delivery_unknown"
			}
			if scenario == "rejected" {
				want = "failed"
			}
			expectedCalls := int32(1)
			if scenario == "process_loss" {
				expectedCalls = 0
			}
			if status != want || calls.Load() != expectedCalls {
				t.Fatal("edit replay/outcome", status, want, calls.Load())
			}
			var deliveries int
			if e := p.Pool.QueryRow(context.Background(), `SELECT count(*) FROM delivery_outbox WHERE status='sent' AND channel_ack='7'`).Scan(&deliveries); e != nil || deliveries != 1 {
				t.Fatal("initial ACK changed", deliveries, e)
			}
		})
	}
}

func TestCommunicationWidgetProjectionEditsCannotOvertakeIntegration(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	c := client(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7,"chat":{"id":42}}}`))
	})
	p, _ := widgetProjectionFixture(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, e := p.Pool.Exec(ctx, `UPDATE communication_interactions SET state='open',version=1; UPDATE communication_widget_views SET draft_version=2,draft_json='{"option_ids":["a"]}'`); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { done <- p.RefreshWidgets(ctx) }()
	select {
	case <-entered:
	case <-ctx.Done():
		close(release)
		t.Fatal("first edit did not reach server")
	}
	if _, e := p.Pool.Exec(ctx, `UPDATE communication_widget_views SET draft_version=3,draft_json='{"option_ids":["b"]}'`); e != nil {
		close(release)
		t.Fatal(e)
	}
	if e := p.RefreshWidgets(ctx); e != nil {
		close(release)
		t.Fatal(e)
	}
	if calls.Load() != 1 {
		close(release)
		t.Fatal("new projection overtook old in-flight edit")
	}
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if e := p.RefreshWidgets(ctx); e != nil {
		t.Fatal(e)
	}
	if calls.Load() != 2 {
		t.Fatal("newest projection did not follow", calls.Load())
	}
	var versions int
	if e := p.Pool.QueryRow(ctx, `SELECT count(*) FROM communication_widget_projection_ops WHERE status='sent' AND revision IN ('1:2:open','1:3:open')`).Scan(&versions); e != nil || versions != 2 {
		t.Fatal(versions, e)
	}
}
