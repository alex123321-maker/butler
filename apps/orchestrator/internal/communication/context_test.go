package communication_test

import (
	"context"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
)

func TestCommunicationConversationContextUsesOnlyPublishedCompletedExchangesIntegration(t *testing.T) {
	f := setup(t)
	publication, err := f.publish(t, "result", "communication.message", `{"schema_version":1,"purpose":"result","text":"Published answer"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE runs SET status='completed' WHERE run_id=$1`, f.p.RunID); err != nil {
		t.Fatal(err)
	}
	current := "fresh-context"
	if _, err = run.NewPostgresRepository(f.pool).CreateRun(f.ctx, run.Record{RunID: current, SessionKey: f.p.SessionKey, InputEventID: current, Status: "preparing", CurrentState: "preparing", AutonomyMode: "mode_2", ModelProvider: "fixture", StartedAt: time.Now(), UpdatedAt: time.Now(), MetadataJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	i, _, err := input.NewStore(f.pool).Accept(f.ctx, ingress.InputEvent{SessionKey: f.p.SessionKey, Source: "web", EventID: current, IdempotencyKey: current, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"followup"}`, CreatedAt: time.Now()}, f.p.OwnerID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET run_id=$2,communication_ingress_epoch=1 WHERE input_id=$1`, i.InputID, current); err != nil {
		t.Fatal(err)
	}
	ids, bounded, err := f.s.ConversationHistory(f.ctx, current)
	if err != nil || !bounded || len(ids) != 1 || ids[0] != f.p.RunID {
		t.Fatal(ids, bounded, err)
	}
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sources := catalog.New(f.pool, manifest)
	actor := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", DeliveryID: "delete"})
	source, err := sources.Get(actor, "message:"+publication.PublicationRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = sources.Delete(actor, source.ID, "delete", source.Revision); err != nil {
		t.Fatal(err)
	}
	ids, bounded, err = f.s.ConversationHistory(f.ctx, current)
	if err != nil || !bounded || len(ids) != 0 {
		t.Fatal("redacted exchange recovered", ids, bounded, err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET context_epoch=0 WHERE input_id=$1`, i.InputID); err != nil {
		t.Fatal(err)
	}
	_, bounded, err = f.s.ConversationHistory(f.ctx, current)
	if err != nil || bounded {
		t.Fatal("legacy work changed replay policy", bounded, err)
	}
}
