package budget

import (
	"context"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestModelAdmissionDeadlineOriginIntegration(t *testing.T) {
	for _, name := range []string{"nominal_model_turn", "earlier_task_scope", "earlier_assigned_authority"} {
		t.Run(name, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx := context.Background()
			_, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('deadline-session','owner','web');INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('deadline-root','deadline-session','owner','one bounded retry');INSERT INTO runs(run_id,session_key,task_id,task_revision,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('deadline-run','deadline-session','deadline-root',1,'input','model_running','mode_1','model_running','fixture',clock_timestamp(),clock_timestamp());INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('deadline-session','deadline-run','lease',1,clock_timestamp()+interval '1 hour')`)
			if err != nil {
				t.Fatal(err)
			}
			p := DefaultPolicy()
			p.Models["fixture:fixture"] = ModelPrice{Basis: "local_no_billing"}
			p.OutputTokens = 8
			p.ModelMillis = 60000
			if name == "earlier_task_scope" {
				p.TaskLifetimeMillis = 30000
			}
			c := NewController(pool, p)
			if name == "earlier_assigned_authority" {
				c.SetAuthorityResolver(func(context.Context, pgx.Tx, string, string) (MandateScope, error) {
					// Declare the fixture's model privacy explicitly; this test owns
					// deadline origin, not admission under missing mandate authority.
					return MandateScope{ID: "mandate", OwnerID: "owner", Privacy: "cloud_allowed", Limits: p.Mandate, Deadline: time.Now().Add(time.Hour), AuthorityDeadline: time.Now().Add(20 * time.Second)}, nil
				})
			}
			owned := ownership.WithToken(ctx, nil, ownership.Token{SessionKey: "deadline-session", RunID: "deadline-run", LeaseID: "lease", FenceEpoch: 1})
			a, err := c.AdmitModel(owned, "deadline-run", "", "fixture", "fixture", 64)
			if err != nil || a.ModelTurnLimited != (name == "nominal_model_turn") {
				t.Fatal("model-turn and authority deadlines conflated", err, a.ModelTurnLimited)
			}
			var n int
			if pool.QueryRow(ctx, `SELECT count(*) FROM execution_model_attempts`).Scan(&n) != nil || n != 1 {
				t.Fatal("deadline observation refunded admission")
			}
		})
	}
}
