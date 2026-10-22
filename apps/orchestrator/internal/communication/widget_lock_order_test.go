package communication_test

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
)

// A resume owns the session before locking its run. A projection reader must
// wait for that session before its source guard locks the run, otherwise the
// two operations form a cycle that the execution-authority watchdog cancels.
func TestCommunicationWidgetDeliveryCannotBlockOwningResumeIntegration(t *testing.T) {
	for _, phase := range []string{"projection", "authorize_send"} {
		t.Run(phase, func(t *testing.T) {
			f := setup(t)
			if _, err := f.publish(t, "lock-order", "communication.widget", widget); err != nil {
				t.Fatal(err)
			}
			d := (&communication.DeliveryStore{Core: f.s}).ForBinding(communication.Binding{ID: f.bindings["telegram"], Version: 1, Channel: "telegram", Recipient: "telegram:verified"})
			m, err := d.Claim(f.ctx, time.Minute)
			if err != nil || m == nil {
				t.Fatal(m, err)
			}
			f.s.SourceGuard = func(ctx context.Context, tx pgx.Tx, run string) error {
				var yes bool
				return tx.QueryRow(ctx, `SELECT TRUE FROM runs WHERE run_id=$1 FOR SHARE`, run).Scan(&yes)
			}
			f.s.ChannelGuard = func(ctx context.Context, tx pgx.Tx, _ string, _ communication.Binding, _ communicationcontrol.Args) error {
				var yes bool
				return tx.QueryRow(ctx, `SELECT TRUE FROM sessions WHERE session_key=$1 FOR SHARE`, f.p.SessionKey).Scan(&yes)
			}
			ctx, cancel := context.WithTimeout(f.ctx, 8*time.Second)
			defer cancel()
			// The fixture intentionally has a two-connection pool: observe locks
			// through an independent read connection while both writes are held.
			observer, err := pgx.ConnectConfig(ctx, f.pool.Config().ConnConfig.Copy())
			if err != nil {
				t.Fatal(err)
			}
			defer observer.Close(context.Background())
			resume, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer resume.Rollback(context.Background())
			var yes bool
			var pid int32
			if err = resume.QueryRow(ctx, `SELECT TRUE,pg_backend_pid() FROM sessions WHERE session_key=$1 FOR UPDATE`, f.p.SessionKey).Scan(&yes, &pid); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				if phase == "projection" {
					_, err := f.s.DeliveryWidget(ctx, *m)
					done <- err
				} else {
					done <- d.AuthorizeSend(ctx, *m)
				}
			}()
			// Observe the actual PostgreSQL lock wait, not a scheduling delay.
			for {
				var waiting bool
				if err = observer.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, pid).Scan(&waiting); err != nil {
					t.Fatal(err)
				}
				if waiting {
					break
				}
				select {
				case err := <-done:
					t.Fatal("delivery never waited for the owning session", err)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				case <-time.After(5 * time.Millisecond):
				}
			}
			lockErr := resume.QueryRow(ctx, `SELECT TRUE FROM runs WHERE run_id=$1 FOR UPDATE NOWAIT`, f.p.RunID).Scan(&yes)
			_ = resume.Rollback(context.Background())
			if err = <-done; err != nil {
				t.Fatal("delivery after resume release", err)
			}
			if lockErr != nil {
				t.Fatal("delivery held run before the owning session; resume would deadlock", lockErr)
			}
		})
	}
}
