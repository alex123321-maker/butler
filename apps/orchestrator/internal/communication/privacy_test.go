package communication_test

import (
	"context"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"path/filepath"
	"testing"
	"time"
)

func TestCommunicationSourceDeletionErasesCopiesIntegration(t *testing.T) {
	f := setup(t)
	r, e := f.publish(t, "publication", "communication.message", message)
	if e != nil {
		t.Fatal(e)
	}
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
	if e = manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	sources := catalog.New(f.pool, manifest)
	actor := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", DeliveryID: "delete"})
	source, e := sources.Get(actor, "message:"+r.PublicationRef)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = sources.Delete(actor, source.ID, "delete", source.Revision); e != nil {
		t.Fatal("canonical deletion", e)
	}
	var redacted bool
	var payload string
	if e = f.pool.QueryRow(f.ctx, `SELECT privacy_redacted,payload::text FROM communication_publications WHERE publication_id=$1`, r.PublicationRef).Scan(&redacted, &payload); e != nil || !redacted || payload != "{}" {
		t.Fatal("canonical payload retained", redacted, payload, e)
	}
	var copies int
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM delivery_outbox WHERE publication_id=$1 AND (content<>'' OR representation_json<>'{}' OR NOT privacy_redacted)`, r.PublicationRef).Scan(&copies); e != nil || copies != 0 {
		t.Fatal("delivery copies retained", copies, e)
	}
}

func TestCommunicationAncestorDeletionErasesQuestionAndResponseIntegration(t *testing.T) {
	f := setup(t)
	if _, e := f.pool.Exec(f.ctx, `INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('original',$1,$2,'user','private preference context')`, f.p.SessionKey, f.p.RunID); e != nil {
		t.Fatal(e)
	}
	r, e := f.publish(t, "question", "communication.widget", widget)
	if e != nil {
		t.Fatal(e)
	}
	if e = f.s.Inputs.Finish(f.ctx, f.claim, "waiting_for_user", "", "", 0); e != nil {
		t.Fatal(e)
	}
	answer := communicationcontrol.Answer{OptionIDs: []string{"a"}}
	if _, e = f.s.Respond(f.actor("telegram", "answer"), r.InteractionRef, f.bindings["telegram"], 1, 1, answer); e != nil {
		t.Fatal(e)
	}
	d := &communication.DeliveryStore{Core: f.s}
	cached, e := d.Claim(f.ctx, 30*time.Second)
	if e != nil || cached == nil {
		t.Fatal(cached, e)
	}
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "deletions"))
	if e = manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	sources := catalog.New(f.pool, manifest)
	actor := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", DeliveryID: "delete"})
	src, e := sources.Get(actor, "message:original")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = sources.Delete(actor, src.ID, "delete", src.Revision); e != nil {
		t.Fatal(e)
	}
	var copies int
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM messages WHERE session_key=$1 AND content<>''`, f.p.SessionKey).Scan(&copies); e != nil || copies != 0 {
		t.Fatal("canonical copies retained", copies, e)
	}
	if e = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM communication_interactions WHERE interaction_id=$1 AND (question_json<>'{}' OR response_json IS NOT NULL OR NOT privacy_redacted)`, r.InteractionRef).Scan(&copies); e != nil || copies != 0 {
		t.Fatal("interaction copies retained", copies, e)
	}
	if _, e = f.s.Respond(f.actor("telegram", "answer"), r.InteractionRef, f.bindings["telegram"], 1, 1, answer); e == nil {
		t.Fatal("redacted response replay allowed")
	}
	if e = d.AuthorizeSend(f.ctx, *cached); e == nil {
		t.Fatal("cached copy sent after deletion")
	}
	var state string
	if e = f.pool.QueryRow(f.ctx, `SELECT status FROM delivery_outbox WHERE notification_id=$1`, cached.NotificationID).Scan(&state); e != nil || state != "delivery_unknown" {
		t.Fatal("ambiguous historical send rewritten", state, e)
	}
}
