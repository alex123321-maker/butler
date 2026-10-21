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
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/owneridentity"
)

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
