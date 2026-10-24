package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/scheduled"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/schedulecontrol"
	"github.com/jackc/pgx/v5"
)

func TestCommunicationScheduledOccurrenceGetsQuietCurrentAdmissionIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	inputs := input.NewStore(pool)
	inputs.CommunicationEpoch = 1
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.New(pool, manifest)
	core := &communication.Store{Pool: pool, Inputs: inputs, SourceGuard: sources.CheckRunSourcesTx, PlanDelivery: func(_ context.Context, _ pgx.Tx, _ string, _ communication.Binding, a communicationcontrol.Args) ([]string, error) {
		return []string{a.Text}, nil
	}}
	b := communication.Binding{ID: "monitor-bot", Version: 1, Channel: "telegram", Recipient: "telegram:42", Enabled: true, Capabilities: []string{"message", "photo", "report"}}
	actor := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "telegram", SessionKey: b.Recipient, DeliveryID: "telegram-rich:77:5"})
	if err := core.Bind(actor, b, 0); err != nil {
		t.Fatal(err)
	}
	c := &telegramCommunication{store: core, binding: telegramrich.OwnerBinding{OwnerID: "owner", BindingID: b.ID, Version: 1, BotID: 77, ChatID: 42, UserID: 42}, policy: telegramExportPolicy{owner: "owner", binding: b}}
	c.installInputHooks()
	if err := c.conversation(actor, telegramrich.Update{ID: 5, Message: &telegramrich.Message{Text: "Yes, check the issuer for a short time"}}); err != nil {
		t.Fatal(err)
	}
	runs := run.NewPostgresRepository(pool)
	if _, err := runs.CreateRun(ctx, run.Record{RunID: "source", SessionKey: c.conversationKey(), InputEventID: "telegram-rich:77:5", Status: "completed", CurrentState: "completed", AutonomyMode: "mode_2", ModelProvider: "fixture", StartedAt: time.Now(), UpdatedAt: time.Now(), MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE accepted_inputs SET run_id='source',status='completed' WHERE event_id='telegram-rich:77:5'`); err != nil {
		t.Fatal(err)
	}
	policy := budget.DefaultPolicy()
	policy.Models["fixture:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	jobs := &scheduled.Store{Pool: pool, Inputs: inputs, Tasks: task.NewStore(pool), Budgets: budget.NewController(pool, policy), Allowance: budget.ModelAllowance{Provider: "fixture", Model: "fixture", InputTokens: 1024, OutputTokens: 8192}, SourceGuard: sources.CheckRunSourcesTx}
	sp := scheduleFixtureSpec("agent_task")
	sp.Monitor = true
	sp.Timing = schedulecontrol.Timing{Kind: "interval", TimeZone: "UTC", IntervalSeconds: 60, MissedPolicy: "coalesce", GraceSeconds: 60}
	sp.MaxOccurrences = 2
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	created, err := jobs.ApplyTx(ctx, tx, scheduled.Principal{OwnerID: "owner", SessionKey: c.conversationKey(), RunID: "source", ExecutionPolicy: "normal", Constraints: []string{}, Approved: true}, "schedule.create", "owner-consented-plan", "plan-hash", schedulecontrol.Args{Job: &sp})
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE scheduled_jobs SET next_at=clock_timestamp()-interval '1 second' WHERE schedule_id=$1`, created.Schedule.ID); err != nil {
		t.Fatal(err)
	}
	if err = jobs.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	claim, err := inputs.Claim(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = runs.CreateRun(ctx, run.Record{RunID: "occurrence", SessionKey: c.conversationKey(), InputEventID: claim.Event.EventID, Status: "preparing", CurrentState: "preparing", AutonomyMode: "mode_2", ModelProvider: "fixture", StartedAt: time.Now(), UpdatedAt: time.Now(), MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	job, err := jobs.Tasks.Get(ctx, claim.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if err = jobs.Tasks.AttachRun(ctx, job.TaskID, job.Revision, "occurrence", c.conversationKey()); err != nil {
		t.Fatal(err)
	}
	leases := session.NewPostgresLeaseManager(pool)
	lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: c.conversationKey(), RunID: "occurrence", OwnerID: "worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE runs SET lease_id=$2 WHERE run_id=$1`, "occurrence", lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(input.WithClaim(ctx, claim), leases.Authority(), ownership.Token{SessionKey: c.conversationKey(), RunID: "occurrence", LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	if err = inputs.SaveCheckpoint(owned, "occurrence", "prepared", `{}`); err != nil {
		t.Fatal(err)
	}
	if err = c.admit(owned, "occurrence"); err != nil {
		t.Fatal("scheduled ingress lost explicit profile", err)
	}
	var quiet bool
	var state string
	if err = pool.QueryRow(ctx, `SELECT quiet,obligation_state FROM communication_admissions WHERE input_id=$1`, claim.InputID).Scan(&quiet, &state); err != nil || !quiet || state != "response_committed" {
		t.Fatal(quiet, state, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE scheduled_jobs SET state='cancelled' WHERE schedule_id=$1`, created.Schedule.ID); err != nil {
		t.Fatal(err)
	}
	if err = c.admit(owned, "occurrence"); !errors.Is(err, communication.ErrDenied) {
		t.Fatal("cancelled monitor retained admission", err)
	}
}
