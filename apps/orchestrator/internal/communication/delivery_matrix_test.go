package communication_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
)

func TestCommunicationDeniedExportTargetDoesNotBlockPermittedTargetIntegration(t *testing.T) {
	for _, method := range []string{"communication.message", "communication.widget"} {
		t.Run(method, func(t *testing.T) {
			f := setup(t)
			f.s.ChannelGuard = func(_ context.Context, _ pgx.Tx, _ string, b communication.Binding, _ communicationcontrol.Args) error {
				if b.Channel == "telegram" {
					return errors.New("private source forbids export")
				}
				return nil
			}
			raw := message
			if method == "communication.widget" {
				raw = widget
			}
			p, e := f.publish(t, "scoped", method, raw)
			if e != nil {
				t.Fatal(e)
			}
			var state, code, body, representation string
			if e = f.pool.QueryRow(f.ctx, `SELECT status,error_code,content,representation_json::text FROM delivery_outbox WHERE publication_id=$1 AND channel='telegram'`, p.PublicationRef).Scan(&state, &code, &body, &representation); e != nil || state != "failed" || code != "policy_denied" || body != "" || representation != "{}" {
				t.Fatal("suppressed target copied forbidden data", state, code, e)
			}
			sender := &matrixSender{calls: map[string]int{}}
			w := deliveryoutbox.Worker{Store: &communication.DeliveryStore{Core: f.s}, Sender: sender}
			for i := 0; i < 3; i++ {
				if _, e = w.ProcessOne(f.ctx); e != nil {
					t.Fatal(e)
				}
			}
			if len(sender.calls) != 1 {
				t.Fatal("denied target sent or blocked Web", sender.calls)
			}
			if method == "communication.widget" {
				if _, e = f.s.Respond(f.actor("telegram", "forbidden-response"), p.InteractionRef, f.bindings["telegram"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e == nil {
					t.Fatal("forbidden export became reply permission")
				}
				if _, e = f.s.Respond(f.actor("web", "permitted-response"), p.InteractionRef, f.bindings["web"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e != nil {
					t.Fatal(e)
				}
			}
		})
	}
}

type matrixSender struct {
	mu     sync.Mutex
	calls  map[string]int
	absent bool
}

func TestCommunicationSendPolicyWaitDoesNotExtendClaimOrQuestionIntegration(t *testing.T) {
	for _, scenario := range []string{"claim_expired", "question_expired", "final_source_wait"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t)
			p, e := f.publish(t, "question", "communication.widget", widget)
			if e != nil {
				t.Fatal(e)
			}
			store := &communication.DeliveryStore{Core: f.s}
			ttl := time.Minute
			if scenario != "question_expired" {
				ttl = 100 * time.Millisecond
			} else {
				if _, e = f.pool.Exec(f.ctx, `UPDATE communication_interactions SET expires_at=clock_timestamp()+INTERVAL '100 milliseconds' WHERE interaction_id=$1`, p.InteractionRef); e != nil {
					t.Fatal(e)
				}
			}
			m, e := store.Claim(f.ctx, ttl)
			if e != nil || m == nil {
				t.Fatal(m, e)
			}
			entered := false
			f.s.ChannelGuard = func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
				if scenario == "final_source_wait" {
					return nil
				}
				entered = true
				time.Sleep(150 * time.Millisecond)
				return nil
			}
			if scenario == "final_source_wait" {
				checks := 0
				f.s.SourceGuard = func(context.Context, pgx.Tx, string) error {
					checks++
					if checks == 2 {
						entered = true
						time.Sleep(150 * time.Millisecond)
					}
					return nil
				}
			}
			if e = store.AuthorizeSend(f.ctx, *m); e == nil || !entered {
				t.Fatal("send deadline crossed policy wait", entered, e)
			}
		})
	}
}

func (s *matrixSender) SendStored(_ context.Context, m deliveryoutbox.Message) deliveryoutbox.Outcome {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls[m.NotificationID]++
	if s.absent && s.calls[m.NotificationID] == 1 {
		return deliveryoutbox.Outcome{Status: "absent", Retryable: true, ErrorCode: "telegram_rejected"}
	}
	return deliveryoutbox.Outcome{Status: "sent", ACK: m.Channel + ":42"}
}
func TestCommunicationConcurrentDeliveryAndKnownAbsenceMatrixIntegration(t *testing.T) {
	for _, absent := range []bool{false, true} {
		t.Run(map[bool]string{false: "concurrent_claim", true: "confirmed_absence_retry"}[absent], func(t *testing.T) {
			f := setup(t)
			if _, e := f.publish(t, "result", "communication.message", message); e != nil {
				t.Fatal(e)
			}
			sender := &matrixSender{calls: map[string]int{}, absent: absent}
			w := deliveryoutbox.Worker{Store: &communication.DeliveryStore{Core: f.s}, Sender: sender}
			var wg sync.WaitGroup
			for i := 0; i < 12; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, e := w.ProcessOne(f.ctx); e != nil {
						t.Error(e)
					}
				}()
			}
			wg.Wait()
			if absent {
				if _, e := f.pool.Exec(f.ctx, `UPDATE delivery_outbox SET next_attempt_at=clock_timestamp()-INTERVAL '1 second' WHERE session_key=$1`, f.p.SessionKey); e != nil {
					t.Fatal(e)
				}
				for i := 0; i < 3; i++ {
					if _, e := w.ProcessOne(f.ctx); e != nil {
						t.Fatal(e)
					}
				}
			}
			want := 1
			if absent {
				want = 2
			}
			sender.mu.Lock()
			defer sender.mu.Unlock()
			if len(sender.calls) != 2 {
				t.Fatal("targets blocked each other", sender.calls)
			}
			for _, n := range sender.calls {
				if n != want {
					t.Fatal("duplicate send", n, want)
				}
			}
			var sent int
			if e := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM delivery_outbox WHERE session_key=$1 AND status='sent' AND channel_ack<>''`, f.p.SessionKey).Scan(&sent); e != nil || sent != 2 || f.count(t, "communication_publications") != 1 {
				t.Fatal("send became publication/tool replay", sent, e)
			}
		})
	}
}

func TestCommunicationActualACKPersistenceFailureNeverResendsIntegration(t *testing.T) {
	f := setup(t)
	if _, e := f.publish(t, "result", "communication.message", message); e != nil {
		t.Fatal(e)
	}
	_, e := f.pool.Exec(f.ctx, `CREATE FUNCTION fixture_fail_ack() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.channel='telegram' AND NEW.status='sent' THEN RAISE EXCEPTION 'fixture ACK write unavailable'; END IF; RETURN NEW; END $$; CREATE TRIGGER fixture_fail_ack BEFORE UPDATE ON delivery_outbox FOR EACH ROW EXECUTE FUNCTION fixture_fail_ack()`)
	if e != nil {
		t.Fatal(e)
	}
	sender := &matrixSender{calls: map[string]int{}}
	store := &communication.DeliveryStore{Core: f.s}
	w := deliveryoutbox.Worker{Store: store, Sender: sender}
	failed := false
	for i := 0; i < 2; i++ {
		if _, e = w.ProcessOne(f.ctx); e != nil {
			failed = true
		}
	}
	if !failed {
		t.Fatal("ACK storage fault not reached")
	}
	var sending int
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM delivery_outbox WHERE session_key=$1 AND channel='telegram' AND status='sending' AND channel_ack=''`, f.p.SessionKey).Scan(&sending); e != nil || sending != 1 {
		t.Fatal("failed ACK falsely persisted", sending, e)
	}
	if _, e = f.pool.Exec(f.ctx, `UPDATE delivery_outbox SET claim_until=clock_timestamp()-INTERVAL '1 second' WHERE session_key=$1 AND status='sending'`, f.p.SessionKey); e != nil {
		t.Fatal(e)
	}
	// Construct a new worker/store; it has no knowledge of the sender's old ACK.
	w.Store = &communication.DeliveryStore{Core: f.s}
	for i := 0; i < 3; i++ {
		if _, e = w.ProcessOne(f.ctx); e != nil {
			t.Fatal(e)
		}
	}
	var unknown int
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM delivery_outbox WHERE session_key=$1 AND status='delivery_unknown'`, f.p.SessionKey).Scan(&unknown); e != nil || unknown != 1 {
		t.Fatal("uncertain ACK became retry", unknown, e)
	}
	for _, n := range sender.calls {
		if n != 1 {
			t.Fatal("ambiguous delivery resent", n)
		}
	}
	if m, e := store.Claim(f.ctx, time.Second); e != nil || m != nil {
		t.Fatal("unknown claim available", m, e)
	}
}
