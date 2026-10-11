package singletab_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/resources"
	"github.com/butler/butler/internal/singletabcontrol"
	"github.com/jackc/pgx/v5"
)

type waitingMetadata struct{ check func() }

func (*waitingMetadata) List(context.Context) ([]singletabcontrol.Tab, error) {
	return []singletabcontrol.Tab{{BrowserInstance: "waiting-host", Ref: "431", Origin: "https://example.org", Active: true}}, nil
}
func (m *waitingMetadata) Check(context.Context, singletabcontrol.Tab) error {
	if m.check != nil {
		m.check()
	}
	return nil
}

func TestOwnerWaitingSelectionAfterActualLeaseReleaseIntegration(t *testing.T) {
	for _, scenario := range []string{"select", "revoke", "anonymous", "foreign_owner", "changed_revision", "stopped_task", "resumed_executor", "source_changes_after_metadata"} {
		t.Run(scenario, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx := context.Background()
			if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('waiting-session','waiting-owner','web')`); err != nil {
				t.Fatal(err)
			}
			tasks := task.NewStore(pool)
			item, err := tasks.Create(ctx, task.CreateRequest{SessionKey: "waiting-session", OwnerID: "waiting-owner", Goal: "select tab", CompletionCriteria: []string{"owner choice"}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES('waiting-run','waiting-session','waiting-event','running','tool_pending','mode_2','fixture',clock_timestamp(),clock_timestamp(),$1,1)`, item.TaskID); err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `UPDATE tasks SET last_run_id='waiting-run' WHERE task_id=$1`, item.TaskID); err != nil {
				t.Fatal(err)
			}
			leases := session.NewPostgresLeaseManager(pool)
			lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: "waiting-session", RunID: "waiting-run", OwnerID: "waiting-owner", TTL: time.Minute})
			if err != nil {
				t.Fatal(err)
			}
			actor := owneridentity.Actor{OwnerID: "waiting-owner", SessionKey: "waiting-session", Channel: "web", DeliveryID: "owner-waiting-fixture"}
			owner := owneridentity.WithActor(ctx, actor)
			worker := ownership.WithToken(owner, leases.Authority(), ownership.Token{SessionKey: lease.SessionKey, RunID: lease.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
			currentSource := true
			sourceGuard := func(context.Context, pgx.Tx, string) error {
				if !currentSource {
					return errors.New("source unavailable")
				}
				return nil
			}
			metadata := &waitingMetadata{}
			live, err := singletabcontrol.New(pool, metadata, resources.WorkerGuard{}, sourceGuard, "waiting-host")
			if err != nil {
				t.Fatal(err)
			}
			proof := resources.Binding{OwnerID: actor.OwnerID, SessionKey: lease.SessionKey, RunID: lease.RunID, TaskID: item.TaskID, TaskRevision: 1, ConstraintsRevision: 1, LeaseEpoch: lease.FenceEpoch, Deadline: lease.ExpiresAt.Add(-time.Second)}
			choices, err := live.Request(worker, proof)
			if err != nil || len(choices.Candidates) != 1 {
				t.Fatal("live request failed", err)
			}
			if _, err = pool.Exec(ctx, `UPDATE runs SET current_state='awaiting_approval' WHERE run_id='waiting-run'`); err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, `UPDATE tasks SET status='waiting_for_user' WHERE task_id=$1`, item.TaskID); err != nil {
				t.Fatal(err)
			}
			if released, err := leases.ReleaseLease(ctx, lease.LeaseID); err != nil || !released {
				t.Fatal("real Session lease release failed", err)
			}
			waiting, err := singletabcontrol.NewWaiting(pool, metadata, sourceGuard, "waiting-host")
			if err != nil {
				t.Fatal(err)
			}
			if _, expiredErr := live.Select(worker, proof, choices.ApprovalID, choices.Candidates[0].Token); expiredErr == nil {
				t.Fatal("expired worker proof revived")
			}
			switch scenario {
			case "anonymous":
				owner = ctx
			case "foreign_owner":
				actor.OwnerID = "other-owner"
				owner = owneridentity.WithActor(ctx, actor)
			case "changed_revision":
				_, err = pool.Exec(ctx, `UPDATE tasks SET revision=revision+1 WHERE task_id=$1`, item.TaskID)
			case "stopped_task":
				_, err = pool.Exec(ctx, `UPDATE tasks SET status='cancelled' WHERE task_id=$1`, item.TaskID)
			case "resumed_executor":
				_, err = leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: lease.SessionKey, RunID: lease.RunID, OwnerID: actor.OwnerID, TTL: time.Minute})
			case "source_changes_after_metadata":
				metadata.check = func() { currentSource = false }
			}
			if err != nil {
				t.Fatal(err)
			}
			token := strings.Repeat("fixture-owner-", 4)
			handler := api.GuardedTabOwnerControls(api.NewOwnerControlAuth(token, actor.OwnerID), waiting)
			get := httptest.NewRequest(http.MethodGet, "/api/v3/tab-choices/"+choices.ApprovalID, nil)
			if scenario != "anonymous" {
				get.Header.Set("Authorization", "Bearer "+token)
			}
			read := httptest.NewRecorder()
			handler.ServeHTTP(read, get)
			readWant := http.StatusConflict
			if scenario == "select" || scenario == "revoke" || scenario == "source_changes_after_metadata" {
				readWant = http.StatusOK
			}
			if scenario == "anonymous" {
				readWant = http.StatusUnauthorized
			}
			if read.Code != readWant {
				t.Fatal("current owner choice read", scenario, read.Code, read.Body.String())
			}
			if read.Code == http.StatusOK {
				var current singletabcontrol.OwnerChoices
				if json.Unmarshal(read.Body.Bytes(), &current) != nil || len(current.Candidates) != 1 || current.Candidates[0].Token != choices.Candidates[0].Token || current.Candidates[0].Ordinal != 1 || !current.Candidates[0].ActiveAtRequest || strings.Contains(read.Body.String(), "waiting-host") || strings.Contains(read.Body.String(), `"431"`) {
					t.Fatal("sanitized immutable choices lost")
				}
				if _, mutateErr := pool.Exec(ctx, `UPDATE guarded_single_tab_choices SET ordinal=2 WHERE request_id=$1`, choices.ApprovalID); mutateErr == nil {
					t.Fatal("recorded choice label could change")
				}
			}
			action, body := "select", `{"candidate_token":"`+choices.Candidates[0].Token+`"}`
			if scenario == "revoke" {
				action, body = "revoke", `{}`
			}
			invoke := func() *httptest.ResponseRecorder {
				r := httptest.NewRequest(http.MethodPost, "/api/v3/tab-choices/"+choices.ApprovalID+"/"+action, strings.NewReader(body))
				if scenario != "anonymous" {
					r.Header.Set("Authorization", "Bearer "+token)
				}
				r.Header.Set("X-Butler-Actor", "waiting-owner")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				return w
			}
			w := invoke()
			want := http.StatusConflict
			if scenario == "select" || scenario == "revoke" {
				want = http.StatusOK
			}
			if scenario == "anonymous" {
				want = http.StatusUnauthorized
			}
			if w.Code != want {
				t.Fatal("actual owner HTTP/PG decision", scenario, w.Code, w.Body.String())
			}
			if scenario == "select" {
				var selected map[string]string
				if json.Unmarshal(w.Body.Bytes(), &selected) != nil || selected["single_tab_session_id"] == "" {
					t.Fatal("opaque selection receipt missing")
				}
				if repeated := invoke(); repeated.Code != http.StatusOK || repeated.Body.String() != w.Body.String() {
					t.Fatal("owner HTTP retry changed choice")
				}
			}
			var activeLeases int
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM session_ownership WHERE lease_id<>'' AND expires_at>clock_timestamp()`).Scan(&activeLeases); err != nil {
				t.Fatal(err)
			}
			expectedLeases := 0
			if scenario == "resumed_executor" {
				expectedLeases = 1
			}
			if activeLeases != expectedLeases {
				t.Fatal("owner selection created or renewed a worker lease")
			}
			var selectedCount int
			if err = pool.QueryRow(ctx, `SELECT count(*) FROM guarded_single_tab_selections`).Scan(&selectedCount); err != nil {
				t.Fatal(err)
			}
			if scenario != "select" && selectedCount != 0 {
				t.Fatal("failed decision leaked a binding")
			}
		})
	}
}
