package communication_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	input "github.com/butler/butler/apps/orchestrator/internal/input"
	run "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/communicationcontrol"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	ctx      context.Context
	pool     *pgxpool.Pool
	s        *communication.Store
	p        communication.Principal
	claim    input.Record
	leases   *session.PostgresLeaseManager
	lease    session.LeaseRecord
	revoked  atomic.Bool
	bindings map[string]string
}

type fixtureOptions struct{ Task, Quiet, VerifiedTask bool }

func setup(t *testing.T, options ...fixtureOptions) *fixture {
	t.Helper()
	var opt fixtureOptions
	if len(options) > 0 {
		opt = options[0]
	}
	dsn := os.Getenv("BUTLER_TEST_COMMUNICATION_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_COMMUNICATION_POSTGRES_URL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	t.Setenv("BUTLER_TEST_MEMORY_POSTGRES_URL", dsn)
	pool := memorytest.Database(t)
	var e error
	id := fmt.Sprintf("comm:%d", time.Now().UnixNano())
	key := "web:" + id
	_, _, e = session.NewPostgresRepository(pool).CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: id + ":owner", Channel: "web", MetadataJSON: `{}`})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if _, e := pool.Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, key); e != nil {
			t.Error("cleanup", e)
		}
	})
	_, e = run.NewPostgresRepository(pool).CreateRun(ctx, run.Record{RunID: id, SessionKey: key, InputEventID: id, Status: "tool_running", CurrentState: "tool_running", AutonomyMode: "mode_1", ModelProvider: "fixture", StartedAt: time.Now(), UpdatedAt: time.Now(), MetadataJSON: `{}`})
	if e != nil {
		t.Fatal(e)
	}
	var job task.Record
	if opt.Task || opt.VerifiedTask {
		tasks := task.NewStore(pool)
		mode := "attempt"
		if opt.VerifiedTask {
			mode = "verified_task"
		}
		job, e = tasks.Create(ctx, task.CreateRequest{SessionKey: key, OwnerID: id + ":owner", Goal: "Choose a preference", CompletionCriteria: []string{"Owner response recorded"}, ResultDelivery: mode})
		if e != nil {
			t.Fatal(e)
		}
		if e = tasks.AttachRun(ctx, job.TaskID, job.Revision, id, key); e != nil {
			t.Fatal(e)
		}
	}
	inputs := input.NewStore(pool)
	_, _, e = inputs.Accept(ctx, ingress.InputEvent{SessionKey: key, EventID: id, Source: "fixture", IdempotencyKey: id, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"ask owner"}`, CreatedAt: time.Now()}, id+":owner", job.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	claim, e := inputs.Claim(ctx, time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	leases := session.NewPostgresLeaseManager(pool)
	lease, e := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: key, RunID: id, OwnerID: "worker", TTL: time.Minute})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `UPDATE runs SET lease_id=$2 WHERE run_id=$1`, id, lease.LeaseID); e != nil {
		t.Fatal(e)
	}
	ctx = ownership.WithToken(input.WithClaim(ctx, claim), leases.Authority(), ownership.Token{SessionKey: key, RunID: id, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	if e = inputs.SaveCheckpoint(ctx, id, "tool_dispatching", `{"tool_call_id":"call"}`); e != nil {
		t.Fatal(e)
	}
	f := &fixture{ctx: ctx, pool: pool, p: communication.Principal{OwnerID: id + ":owner", SessionKey: key, RunID: id, InputID: claim.InputID, TaskID: job.TaskID, TaskRevision: job.Revision, ConstraintsRevision: job.ConstraintsRevision}, claim: claim, leases: leases, lease: lease, bindings: map[string]string{}}
	f.s = &communication.Store{Pool: f.pool, Inputs: inputs, SourceGuard: func(context.Context, pgx.Tx, string) error {
		if f.revoked.Load() {
			return errors.New("revoked")
		}
		return nil
	}, ChannelGuard: func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
		return nil
	}}
	tx, e := f.pool.Begin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.s.AdmitTx(ctx, tx, f.p, opt.Quiet, []string{"communication.message", "communication.widget"}); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	inputs.CommunicationEpoch = 1
	for _, channel := range []string{"web", "telegram"} {
		bid := id + ":" + channel
		f.bindings[channel] = bid
		a := owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: channel, SessionKey: channel + ":verified", DeliveryID: "bind:" + id}
		if e = f.s.Bind(owneridentity.WithActor(ctx, a), communication.Binding{ID: bid, Channel: channel, Recipient: a.SessionKey, Enabled: true, Capabilities: []string{"message", "widget"}}, 0); e != nil {
			t.Fatal(e)
		}
	}
	return f
}
func (f *fixture) publish(t *testing.T, call, tool, raw string) (communicationcontrol.Receipt, error) {
	t.Helper()
	tx, e := f.pool.Begin(f.ctx)
	if e != nil {
		return communicationcontrol.Receipt{}, e
	}
	defer tx.Rollback(context.Background())
	r, e := f.s.ApplyTx(f.ctx, tx, f.p, communicationcontrol.Envelope{RunID: f.p.RunID, ToolCallID: call, EffectID: call + ":" + f.p.RunID, Tool: tool, ArgsJSON: raw})
	if e != nil {
		return r, e
	}
	return r, tx.Commit(f.ctx)
}
func (f *fixture) actor(channel, delivery string) context.Context {
	return owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: channel, SessionKey: channel + ":verified", DeliveryID: delivery})
}
func (f *fixture) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if e := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM `+table+` WHERE session_key=$1`, f.p.SessionKey).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}

const message = `{"schema_version":1,"purpose":"result","text":"Published answer"}`
const widget = `{"schema_version":1,"question":"Choose","input":{"type":"single_choice","options":[{"id":"a","label":"A"},{"id":"b","label":"B"}]}}`

func TestCommunicationPublicationAtomicIdentityAndSeparateFinalIntegration(t *testing.T) {
	f := setup(t)
	r, e := f.publish(t, "one", "communication.message", message)
	if e != nil {
		t.Fatal(e)
	}
	again, e := f.publish(t, "one", "communication.message", message)
	if e != nil || again.PublicationRef != r.PublicationRef {
		t.Fatal("replay", e)
	}
	if _, e = f.publish(t, "one", "communication.message", `{"schema_version":1,"purpose":"result","text":"changed"}`); !errors.Is(e, communication.ErrConflict) {
		t.Fatal("different payload replay", e)
	}
	if _, e = f.publish(t, "two", "communication.message", message); e != nil {
		t.Fatal(e)
	}
	if f.count(t, "communication_publications") != 2 || f.count(t, "messages") != 2 || f.count(t, "delivery_outbox") != 4 {
		t.Fatal("fanout/transcript identity")
	}
	if m, e := deliveryoutbox.NewStore(f.pool).Claim(f.ctx, time.Second); e != nil || m != nil {
		t.Fatal("legacy dispatcher picked new publication", m, e)
	}
	if _, e = f.pool.Exec(f.ctx, `UPDATE runs SET current_state='finalizing',status='finalizing' WHERE run_id=$1`, f.p.RunID); e != nil {
		t.Fatal(e)
	}
	c := completion.New(f.pool)
	c.Communication = f.s
	c.DataGuard = f.s.SourceGuard
	q := completion.Request{RunID: f.p.RunID, SessionKey: f.p.SessionKey, Content: "Different internal final", AnswerState: "final", TerminalObserved: true}
	if _, e = c.Save(f.ctx, q); e != nil {
		t.Fatal("final conflicts with explicit publications", e)
	}
	if f.count(t, "messages") != 3 || f.count(t, "delivery_outbox") != 4 {
		t.Fatal("implicit duplicate final notification")
	}
	if _, e = c.Save(f.ctx, q); e != nil {
		t.Fatal("frozen final recovery", e)
	}
	var kind, role string
	if e = f.pool.QueryRow(f.ctx, `SELECT metadata->>'kind',role FROM messages WHERE message_id=$1`, "final:"+f.p.RunID).Scan(&kind, &role); e != nil || kind != "attempt_final" || role != "system" {
		t.Fatal("internal final type", kind, role, e)
	}
}
func TestCommunicationRollbackAndSourceGuardIntegration(t *testing.T) {
	f := setup(t)
	tx, e := f.pool.Begin(f.ctx)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ApplyTx(f.ctx, tx, f.p, communicationcontrol.Envelope{RunID: f.p.RunID, ToolCallID: "rollback", EffectID: "rollback:" + f.p.RunID, Tool: "communication.message", ArgsJSON: message}); e != nil {
		t.Fatal(e)
	}
	if e = tx.Rollback(f.ctx); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"messages", "communication_publications", "delivery_outbox", "communication_events"} {
		if f.count(t, table) != 0 {
			t.Fatal("partial rollback", table)
		}
	}
	f.revoked.Store(true)
	if _, e = f.publish(t, "revoked", "communication.message", message); e == nil {
		t.Fatal("revoked sources published")
	}
	if f.count(t, "communication_publications") != 0 {
		t.Fatal("denied publication committed")
	}
}
func TestCommunicationFastResponseFinishAndResumeIntegration(t *testing.T) {
	for _, early := range []bool{true, false} {
		t.Run(fmt.Sprint("early_", early), func(t *testing.T) {
			f := setup(t)
			r, e := f.publish(t, "question", "communication.widget", widget)
			if e != nil {
				t.Fatal(e)
			}
			if !early {
				if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
					t.Fatal(e)
				}
			}
			var winners atomic.Int32
			var wg sync.WaitGroup
			for _, channel := range []string{"web", "telegram"} {
				wg.Add(1)
				go func(ch string) {
					defer wg.Done()
					_, e := f.s.Respond(f.actor(ch, "answer:"+ch), r.InteractionRef, f.bindings[ch], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}})
					if e == nil {
						winners.Add(1)
					} else if !errors.Is(e, communication.ErrResolved) {
						t.Error(e)
					}
				}(channel)
			}
			wg.Wait()
			if winners.Load() != 1 {
				t.Fatal("first submit", winners.Load())
			}
			if early {
				if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
					t.Fatal(e)
				}
			}
			saved, e := f.s.Inputs.Get(f.ctx, f.claim.InputID)
			if e != nil || saved.Status != "queued" || saved.Checkpoint != "interaction_ready" {
				t.Fatal("late Finish lost response", saved.Status, saved.Checkpoint, e)
			}
			if _, e = f.leases.ReleaseLease(f.ctx, f.lease.LeaseID); e != nil {
				t.Fatal(e)
			}
			claim, e := f.s.Inputs.Claim(f.ctx, time.Minute)
			if e != nil {
				t.Fatal(e)
			}
			lease, e := f.leases.AcquireLease(f.ctx, session.AcquireLeaseParams{SessionKey: f.p.SessionKey, RunID: f.p.RunID, OwnerID: "resumed-worker", TTL: time.Minute})
			if e != nil {
				t.Fatal(e)
			}
			if _, e = f.pool.Exec(f.ctx, `UPDATE runs SET lease_id=$2 WHERE run_id=$1`, f.p.RunID, lease.LeaseID); e != nil {
				t.Fatal(e)
			}
			fresh := ownership.WithToken(input.WithClaim(context.Background(), claim), f.leases.Authority(), ownership.Token{RunID: f.p.RunID, SessionKey: f.p.SessionKey, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
			if _, e = f.s.ResumeInteraction(fresh, f.p.RunID, r.InteractionRef); e != nil {
				t.Fatal("fresh resume", e)
			}
			if f.count(t, "messages") != 2 {
				t.Fatal("response/transcript duplicated")
			}
		})
	}
}
func TestCommunicationNoExpiryAndInvalidSubmitIntegration(t *testing.T) {
	f := setup(t)
	r, e := f.publish(t, "question", "communication.widget", widget)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
		t.Fatal(e)
	}
	if _, e = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET expires_at=clock_timestamp()-INTERVAL '25 hours' WHERE input_id=$1`, f.claim.InputID); e != nil {
		t.Fatal(e)
	}
	if e = f.s.Inputs.ExpireQueued(f.ctx); e != nil {
		t.Fatal(e)
	}
	bad := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "other", Channel: "web", SessionKey: "web:verified", DeliveryID: "wrong"})
	if _, e = f.s.Respond(bad, r.InteractionRef, f.bindings["web"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"a"}}); e == nil {
		t.Fatal("wrong owner")
	}
	if _, e = f.s.Respond(f.actor("web", "invalid"), r.InteractionRef, f.bindings["web"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"unknown"}}); e == nil {
		t.Fatal("invalid option")
	}
	if _, e = f.s.Respond(f.actor("telegram", "valid"), r.InteractionRef, f.bindings["telegram"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"b"}}); e != nil {
		t.Fatal("no-expiry question lost", e)
	}
	if _, e = f.s.Respond(f.actor("telegram", "valid"), r.InteractionRef, f.bindings["telegram"], 1, 1, communicationcontrol.Answer{OptionIDs: []string{"b"}}); e != nil {
		t.Fatal("response receipt replay", e)
	}
}

type sender struct {
	calls int
	out   deliveryoutbox.Outcome
}

func (s *sender) SendStored(context.Context, deliveryoutbox.Message) deliveryoutbox.Outcome {
	s.calls++
	return s.out
}
func TestCommunicationUnknownDeliveryNeverReplaysIntegration(t *testing.T) {
	f := setup(t)
	if _, e := f.publish(t, "one", "communication.message", message); e != nil {
		t.Fatal(e)
	}
	send := &sender{out: deliveryoutbox.Outcome{Status: "delivery_unknown"}}
	store := &communication.DeliveryStore{Core: f.s}
	w := deliveryoutbox.Worker{Store: store, Sender: send}
	for n := 0; n < 3; n++ {
		if _, e := w.ProcessOne(f.ctx); e != nil {
			t.Fatal(e)
		}
	}
	if send.calls != 2 {
		t.Fatal("unknown delivery resent", send.calls)
	}
	var n int
	if e := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM delivery_outbox WHERE session_key=$1 AND status='delivery_unknown'`, f.p.SessionKey).Scan(&n); e != nil || n != 2 {
		t.Fatal(n, e)
	}
}
func TestCommunicationCorrectionIsBoundedIntegration(t *testing.T) {
	f := setup(t)
	state, e := f.s.BeforeFinal(f.ctx, f.p.RunID, "not published")
	if e != nil || state != "correct" {
		t.Fatal(state, e)
	}
	// Simulates recovery after correction admission; a restart cannot replenish it.
	state, e = f.s.BeforeFinal(f.ctx, f.p.RunID, "still not published")
	if e != nil || state != "failed" {
		t.Fatal(state, e)
	}
	if f.count(t, "communication_publications") != 1 {
		t.Fatal("safe failure notice missing")
	}
	var p communicationcontrol.Args
	var raw []byte
	_ = f.pool.QueryRow(f.ctx, `SELECT payload FROM communication_publications WHERE run_id=$1`, f.p.RunID).Scan(&raw)
	_ = json.Unmarshal(raw, &p)
	if p.Text == "not published" || p.Text == "still not published" || p.Purpose != "blocked" {
		t.Fatal("raw final fallback")
	}
}
