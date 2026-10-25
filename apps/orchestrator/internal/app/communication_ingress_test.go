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
	"github.com/butler/butler/apps/orchestrator/internal/session"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

func TestStoppedAttemptActualTelegramIngressSurvivesStoreRestartIntegration(t *testing.T) {
	f := newDelegationGuardFixtureWithManifest(t, filepath.Join(t.TempDir(), "deletions.log"))
	ctx := context.Background()
	inputs := input.NewStore(f.pool)
	inputs.CommunicationEpoch = 1
	core := &communication.Store{Pool: f.pool, Inputs: inputs, SourceGuard: f.sources.CheckRunSourcesTx}
	b := communication.Binding{ID: "stop-test-bot", Version: 1, Channel: "telegram", Recipient: "telegram:42", Enabled: true, Capabilities: []string{"message"}}
	a := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "telegram", SessionKey: b.Recipient, DeliveryID: "telegram-rich:77:5"})
	if err := core.Bind(a, b, 0); err != nil {
		t.Fatal(err)
	}
	c := &telegramCommunication{store: core, binding: telegramrich.OwnerBinding{OwnerID: f.parent.OwnerID, BindingID: b.ID, Version: 1, BotID: 77, ChatID: 42, UserID: 42}, policy: telegramExportPolicy{binding: b}}
	c.installInputHooks()
	if err := c.conversation(a, telegramrich.Update{ID: 5, Message: &telegramrich.Message{Text: "old request"}}); err != nil {
		t.Fatal(err)
	}
	old, err := inputs.Claim(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	repo := run.NewPostgresRepository(f.pool)
	if _, err = repo.CreateRun(ctx, run.Record{RunID: "telegram-stopped", SessionKey: c.conversationKey(), InputEventID: old.Event.EventID, Status: "model_running", CurrentState: "model_running", AutonomyMode: "mode_2", ModelProvider: "fixture", StartedAt: time.Now(), UpdatedAt: time.Now(), MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	leases := session.NewPostgresLeaseManager(f.pool)
	lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: c.conversationKey(), RunID: "telegram-stopped", OwnerID: "worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	live := ownership.WithToken(input.WithClaim(ctx, old), leases.Authority(), ownership.Token{SessionKey: lease.SessionKey, RunID: lease.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	if err = inputs.SaveCheckpoint(live, lease.RunID, "tool_dispatching", `{"tool_call_id":"unknown-call","effect_id":"telegram-isolated"}`); err != nil {
		t.Fatal(err)
	}
	if err = inputs.Finish(ctx, old, "blocked", "effect_unknown", "retained original failure", 0); err != nil {
		t.Fatal(err)
	}
	if _, err = leases.ReleaseLease(ctx, lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `UPDATE accepted_inputs SET updated_at=clock_timestamp()-interval '10 seconds' WHERE input_id=$1`, old.InputID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('telegram-isolated','f',jsonb_build_object('run_id','telegram-stopped','owner_id',$1::text,'tool_call_id','unknown-call','effect','commit','tool_name','http.request'),'{}','effect_unknown')`, f.parent.OwnerID); err != nil {
		t.Fatal(err)
	}
	a = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: "telegram", SessionKey: b.Recipient, DeliveryID: "telegram-rich:77:6"})
	if err = c.conversation(a, telegramrich.Update{ID: 6, Message: &telegramrich.Message{Text: "independent question"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = inputs.Claim(ctx, time.Minute); !errors.Is(err, input.ErrNotFound) {
		t.Fatal("fixture did not reproduce blocked incoming queue", err)
	}
	restarted := input.NewStore(f.pool)
	restarted.CommunicationEpoch = 1
	restarted.StoppedFinalizer = core
	if err = restarted.SweepStopped(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := restarted.Claim(ctx, time.Minute)
	if err != nil || next.Event.EventID != "telegram-rich:77:6" || next.Source != "telegram" || next.Attempts != 1 {
		t.Fatal("Telegram message required resubmission after restart", next.InputID, next.Event.EventID, err)
	}
	previous, err := restarted.Get(ctx, old.InputID)
	if err != nil || previous.Status != "failed" || previous.ErrorCode != "effect_unknown" {
		t.Fatal("old attempt reopened or lost cause", err)
	}
	var state string
	if err = f.pool.QueryRow(ctx, `SELECT state FROM action_effects WHERE action_id='telegram-isolated'`).Scan(&state); err != nil || state != "effect_unknown" {
		t.Fatal("queue release replayed or reconciled unknown action", state, err)
	}
}

func TestCommunicationTelegramIngressEpochAndPayloadCannotOptInIntegration(t *testing.T) {
	f := newDelegationGuardFixtureWithManifest(t, filepath.Join(t.TempDir(), "deletions.log"))
	inputs := input.NewStore(f.pool)
	core := &communication.Store{Pool: f.pool, Inputs: inputs, SourceGuard: f.sources.CheckRunSourcesTx}
	b := communication.Binding{ID: "test-bot", Version: 1, Channel: "telegram", Recipient: "telegram:42", Enabled: true, Capabilities: []string{"message", "photo", "report"}}
	a := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: f.parent.OwnerID, Channel: b.Channel, SessionKey: b.Recipient, DeliveryID: "telegram-rich:77:5"})
	if err := core.Bind(a, b, 0); err != nil {
		t.Fatal(err)
	}
	c := &telegramCommunication{store: core, binding: telegramrich.OwnerBinding{OwnerID: f.parent.OwnerID, BindingID: b.ID, Version: 1, BotID: 77, ChatID: 42, UserID: 42}, policy: telegramExportPolicy{binding: b}}
	c.installInputHooks()
	if err := c.conversation(a, telegramrich.Update{ID: 5, Message: &telegramrich.Message{Text: "Hello"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := inputs.Claim(f.ctx, time.Minute); !errors.Is(err, input.ErrNotFound) {
		t.Fatal("epoch-0 worker claimed verified ingress", err)
	}
	inputs.CommunicationEpoch = 1
	verified, err := inputs.Claim(f.ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = inputs.Finish(f.ctx, verified, "completed", "", "", 0); err != nil {
		t.Fatal(err)
	}
	forged := ingress.InputEvent{EventID: "forged-payload", IdempotencyKey: "forged-payload", Source: "telegram", SessionKey: c.conversationKey(), EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"Hello","user_id":"guard-owner","communication_ingress_epoch":1,"mode":"explicit","origin_binding_ref":"test-bot"}`, CreatedAt: time.Now().UTC()}
	if _, _, err = inputs.Accept(f.ctx, forged, f.parent.OwnerID, ""); err != nil {
		t.Fatal(err)
	}
	claim, err := inputs.Claim(f.ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	repo := run.NewPostgresRepository(f.pool)
	if _, err = repo.CreateRun(f.ctx, run.Record{RunID: "forged-run", SessionKey: c.conversationKey(), InputEventID: forged.EventID, Status: "preparing", CurrentState: "preparing", AutonomyMode: "mode_1", ModelProvider: "fixture", StartedAt: time.Now(), UpdatedAt: time.Now(), MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	// Bind the forged public fixture to its attempted run; admission must deny
	// before any execution lease or model capability could be acquired.
	if _, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET run_id='forged-run' WHERE input_id=$1`, claim.InputID); err != nil {
		t.Fatal(err)
	}
	if err = c.admit(f.ctx, "forged-run"); !errors.Is(err, communication.ErrDenied) {
		t.Fatal("payload forged explicit profile", err)
	}
	var epoch int
	if err = f.pool.QueryRow(f.ctx, `SELECT communication_ingress_epoch FROM accepted_inputs WHERE input_id=$1`, claim.InputID).Scan(&epoch); err != nil || epoch != 0 {
		t.Fatal("payload became transport proof", epoch, err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(f.ctx, `UPDATE accepted_inputs SET communication_ingress_epoch=0,communication_ingress_json='{}' WHERE input_id=$1`, verified.InputID); err == nil {
		t.Fatal("transport proof mutable")
	}
}
