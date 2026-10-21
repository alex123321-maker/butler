package communication_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
)

func TestCommunicationQuietAndVerifiedTaskDoNotDemandPublicationIntegration(t *testing.T) {
	for _, verified := range []bool{false, true} {
		t.Run(fmt.Sprint(verified), func(t *testing.T) {
			f := setup(t, fixtureOptions{Quiet: !verified, VerifiedTask: verified})
			state, e := f.s.BeforeFinal(f.ctx, f.p.RunID, "")
			if e != nil || state != "ready" {
				t.Fatal(state, e)
			}
			if f.count(t, "communication_publications") != 0 {
				t.Fatal("quiet run published")
			}
			var corrections int
			if e = f.pool.QueryRow(f.ctx, `SELECT correction_count FROM communication_admissions WHERE input_id=$1`, f.p.InputID).Scan(&corrections); e != nil || corrections != 0 {
				t.Fatal(corrections, e)
			}
			if verified {
				if _, e = f.publish(t, "forbidden", "communication.message", message); e == nil {
					t.Fatal("verified_task published unverified result")
				}
			} else {
				if _, e = f.pool.Exec(f.ctx, `UPDATE runs SET current_state='finalizing',status='finalizing' WHERE run_id=$1`, f.p.RunID); e != nil {
					t.Fatal(e)
				}
				c := completion.New(f.pool)
				c.Communication = f.s
				if _, e = c.Save(f.ctx, completion.Request{RunID: f.p.RunID, SessionKey: f.p.SessionKey, TerminalObserved: true}); e != nil {
					t.Fatal("typed empty quiet final", e)
				}
				if f.count(t, "delivery_outbox") != 0 {
					t.Fatal("quiet final created notification")
				}
			}
		})
	}
}

func TestCommunicationNonemptyFinalRequiresTerminalProofIntegration(t *testing.T) {
	f := setup(t)
	if _, e := f.publish(t, "answer", "communication.message", message); e != nil {
		t.Fatal(e)
	}
	if _, e := f.pool.Exec(f.ctx, `UPDATE runs SET current_state='finalizing',status='finalizing' WHERE run_id=$1`, f.p.RunID); e != nil {
		t.Fatal(e)
	}
	c := completion.New(f.pool)
	c.Communication = f.s
	q := completion.Request{RunID: f.p.RunID, SessionKey: f.p.SessionKey, Content: "Internal final with no terminal observation"}
	if _, e := c.Save(f.ctx, q); e == nil {
		t.Fatal("nonempty explicit final bypassed terminal proof")
	}
	if f.count(t, "run_final_results") != 0 {
		t.Fatal("unproved final partially committed")
	}
	q.TerminalObserved = true
	if _, e := c.Save(f.ctx, q); e != nil {
		t.Fatal("typed nonempty terminal final rejected", e)
	}
}

func TestCommunicationQuietQuestionStillWaitsIntegration(t *testing.T) {
	f := setup(t, fixtureOptions{Quiet: true})
	if _, e := f.publish(t, "question", "communication.widget", widget); e != nil {
		t.Fatal(e)
	}
	if decision, e := f.s.BeforeFinal(f.ctx, f.p.RunID, ""); e != nil || decision != "waiting" {
		t.Fatal("quiet policy bypassed open question", decision, e)
	}
}

func TestCommunicationExhaustedOrUnavailableNoticeStillTerminatesIntegration(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(fmt.Sprint(unavailable), func(t *testing.T) {
			f := setup(t)
			if unavailable {
				if _, e := f.pool.Exec(f.ctx, `UPDATE communication_bindings SET enabled=FALSE`); e != nil {
					t.Fatal(e)
				}
			} else {
				for n := 0; n < 20; n++ {
					if _, e := f.publish(t, fmt.Sprint(n), "communication.message", `{"schema_version":1,"purpose":"progress","text":"progress"}`); e != nil {
						t.Fatal(e)
					}
				}
				if _, e := f.publish(t, "over-quota", "communication.message", message); e == nil {
					t.Fatal("agent exceeded quota")
				}
			}
			for n, want := range []string{"correct", "failed", "failed"} {
				state, e := f.s.BeforeFinal(f.ctx, f.p.RunID, "private final")
				if e != nil || state != want {
					t.Fatal(n, state, e)
				}
			}
			want := 21
			if unavailable {
				want = 0
			}
			if n := f.count(t, "communication_publications"); n != want {
				t.Fatal("unbounded/missing notice", n, want)
			}
		})
	}
}

func TestCommunicationDestinationAndExportRevocationIntegration(t *testing.T) {
	for _, scenario := range []string{"destination_changed", "missing_guard", "export_denied", "send_revoked"} {
		t.Run(scenario, func(t *testing.T) {
			f := setup(t)
			if scenario == "destination_changed" {
				var e error
				f.p.DestinationsHash, e = communication.DestinationHash(f.ctx, f.pool, f.p.OwnerID, "message")
				if e != nil {
					t.Fatal(e)
				}
				if e = f.s.Bind(f.actor("telegram", "disable"), communication.Binding{ID: f.bindings["telegram"], Channel: "telegram", Recipient: "telegram:verified", Enabled: false, Capabilities: []string{"message", "widget"}}, 1); e != nil {
					t.Fatal(e)
				}
			}
			if scenario == "missing_guard" {
				f.s.ChannelGuard = nil
			}
			if scenario == "export_denied" {
				f.s.ChannelGuard = func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
					return communication.ErrDenied
				}
			}
			_, e := f.publish(t, "publish", "communication.message", message)
			if scenario != "send_revoked" {
				if e == nil || f.count(t, "communication_publications") != 0 || f.count(t, "delivery_outbox") != 0 {
					t.Fatal("partial or unauthorized export", e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			d := &communication.DeliveryStore{Core: f.s}
			m, e := d.Claim(f.ctx, 30*time.Second)
			if e != nil || m == nil {
				t.Fatal(m, e)
			}
			f.s.ChannelGuard = func(context.Context, pgx.Tx, string, communication.Binding, communicationcontrol.Args) error {
				return communication.ErrDenied
			}
			if e = d.AuthorizeSend(f.ctx, *m); e == nil {
				t.Fatal("export revocation ignored")
			}
			var state string
			if e = f.pool.QueryRow(f.ctx, `SELECT status FROM delivery_outbox WHERE notification_id=$1`, m.NotificationID).Scan(&state); e != nil || state != "failed" {
				t.Fatal("known no-send became unknown", state, e)
			}
		})
	}
}
