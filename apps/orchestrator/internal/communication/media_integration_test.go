package communication_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

func mediaFixture(t *testing.T) (*fixture, string) {
	t.Helper()
	f := setup(t)
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 20, 20))); err != nil {
		t.Fatal(err)
	}
	ref, err := artifacts.NewService(artifacts.NewPostgresRepository(f.pool)).SaveMedia(f.ctx, f.p.RunID, f.p.SessionKey, "test.png", "image/png", data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	policy := func(ctx context.Context, tx pgx.Tx, run, ref string) error { return f.s.SourceGuard(ctx, tx, run) }
	planner := telegramrich.Planner{Resolve: func(ctx context.Context, tx pgx.Tx, run string, b communication.Binding, ref string) (artifacts.Media, error) {
		return artifacts.ResolveMediaTx(ctx, tx, run, ref, policy)
	}}
	f.s.PlanDelivery = planner.Plan
	a := owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "telegram", SessionKey: "telegram:42", DeliveryID: "new-bot-bind"}
	if err = f.s.Bind(owneridentity.WithActor(f.ctx, a), communication.Binding{ID: f.bindings["telegram"], Channel: "telegram", Recipient: a.SessionKey, Enabled: true, Capabilities: []string{"message", "photo", "report"}}, 1); err != nil {
		t.Fatal(err)
	}
	// The existing Web binding is not opted into photo/report. This isolated
	// profile makes no Web renderer or complete-profile readiness claim.
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(f.ctx, `DELETE FROM communication_admissions WHERE input_id=$1`, f.p.InputID); err != nil {
		t.Fatal(err)
	}
	if err = f.s.AdmitTx(f.ctx, tx, f.p, false, []string{"communication.message", "communication.photo", "communication.report"}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f, ref
}

func TestCommunicationMediaPartsAreAtomicOrderedAndRestartSafeIntegration(t *testing.T) {
	f, ref := mediaFixture(t)
	args := communicationcontrol.Args{SchemaVersion: 1, Purpose: "result", Text: "Фотография 🧪", Images: []communicationcontrol.Image{{ArtifactRef: ref, Alt: "Проверочное изображение"}}}
	raw, _ := json.Marshal(args)
	receipt, err := f.publish(t, "photo", "communication.photo", string(raw))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.publish(t, "photo", "communication.photo", string(raw)); err != nil {
		t.Fatal("lost commit ACK replay", err)
	}
	if f.count(t, "messages") != 1 || f.count(t, "communication_publications") != 1 || f.count(t, "delivery_outbox") != 2 {
		t.Fatal("non-atomic publication or duplicated transcript")
	}
	d := &communication.DeliveryStore{Core: f.s}
	first, err := d.Claim(f.ctx, 30*time.Second)
	if err != nil || first == nil || first.ChunkIndex != 0 {
		t.Fatal(first, err)
	}
	if err = d.AuthorizeSend(f.ctx, *first); err != nil {
		t.Fatal(err)
	}
	if second, err := d.Claim(f.ctx, 30*time.Second); err != nil || second != nil {
		t.Fatal("later photo part overtook pending caption", second, err)
	}
	if err = d.Finish(f.ctx, *first, deliveryoutbox.Outcome{Status: "sent", ACK: "100"}); err != nil {
		t.Fatal(err)
	}
	// A fresh dispatcher resumes only the remaining persisted media part.
	d = &communication.DeliveryStore{Core: f.s}
	second, err := d.Claim(f.ctx, 30*time.Second)
	if err != nil || second == nil || second.ChunkIndex != 1 {
		t.Fatal(second, err)
	}
	var part telegramrich.Part
	if json.Unmarshal([]byte(second.Content), &part) != nil || part.Method != "sendPhoto" || len(part.Files) != 1 || part.Files[0].Ref != ref {
		t.Fatal("lost media manifest")
	}
	if err = d.Finish(f.ctx, *second, deliveryoutbox.Outcome{Status: "delivery_unknown"}); err != nil {
		t.Fatal(err)
	}
	if next, err := d.Claim(f.ctx, 30*time.Second); err != nil || next != nil {
		t.Fatal("unknown media replayed", next, err)
	}
	var count int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM delivery_outbox WHERE publication_id=$1 AND status='sent' AND channel_ack='100'`, receipt.PublicationRef).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestCommunicationMediaPolicyAndIntegrityIntegration(t *testing.T) {
	for _, kind := range []string{"revoked", "redacted", "digest", "wrong-owner", "missing-guard"} {
		t.Run(kind, func(t *testing.T) {
			f, ref := mediaFixture(t)
			policy := func(ctx context.Context, tx pgx.Tx, run, ref string) error { return f.s.SourceGuard(ctx, tx, run) }
			switch kind {
			case "revoked":
				f.revoked.Store(true)
			case "redacted":
				_, _ = f.pool.Exec(f.ctx, `UPDATE artifacts SET privacy_redacted=TRUE WHERE run_id=$1`, f.p.RunID)
			case "digest":
				_, _ = f.pool.Exec(f.ctx, `UPDATE artifacts SET content_json=jsonb_set(content_json,'{sha256}','"changed"') WHERE run_id=$1`, f.p.RunID)
			case "wrong-owner":
				other := f.p.SessionKey + ":other"
				_, _, err := session.NewPostgresRepository(f.pool).CreateSession(f.ctx, session.CreateSessionParams{SessionKey: other, UserID: "different-owner", Channel: "web", MetadataJSON: `{}`})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _, _ = f.pool.Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, other) })
				_, _ = f.pool.Exec(f.ctx, `UPDATE artifacts SET session_key=$2 WHERE run_id=$1`, f.p.RunID, other)
			case "missing-guard":
				policy = nil
			}
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if _, err = artifacts.ResolveMediaTx(f.ctx, tx, f.p.RunID, ref, policy); err == nil {
				t.Fatal("media guard bypass", kind)
			}
		})
	}
}

func TestCommunicationReportPlanningFailureRollsBackIntegration(t *testing.T) {
	f, ref := mediaFixture(t)
	a := communicationcontrol.Args{SchemaVersion: 1, Purpose: "result", Title: "Report", Summary: "Summary", Blocks: []communicationcontrol.Block{{Type: "image", ArtifactRef: ref, Alt: "Photo"}, {Type: "document", ArtifactRef: "artifact:missing"}}}
	raw, _ := json.Marshal(a)
	if _, err := f.publish(t, "report", "communication.report", string(raw)); err == nil {
		t.Fatal("missing required attachment accepted")
	}
	if f.count(t, "messages") != 0 || f.count(t, "delivery_outbox") != 0 || f.count(t, "communication_publications") != 0 {
		t.Fatal("partial report transaction")
	}
	a.Blocks = a.Blocks[:1]
	raw, _ = json.Marshal(a)
	if _, err := f.publish(t, "report", "communication.report", string(raw)); err != nil {
		t.Fatal(err)
	}
	m, err := (&communication.DeliveryStore{Core: f.s}).Claim(f.ctx, 30*time.Second)
	if err != nil || m == nil {
		t.Fatal(err)
	}
	f.revoked.Store(true)
	if err = (&communication.DeliveryStore{Core: f.s}).AuthorizeSend(f.ctx, *m); err == nil {
		t.Fatal("delayed report ignored source revocation")
	}
}

func TestCommunicationRichAdapterReplyIdentityAndCanonicalTargetIntegration(t *testing.T) {
	f, _ := mediaFixture(t)
	if _, err := f.pool.Exec(f.ctx, `UPDATE communication_bindings SET enabled=FALSE WHERE binding_id=$1`, f.bindings["web"]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.publish(t, "reply-target", "communication.message", message); err != nil {
		t.Fatal(err)
	}
	d := &communication.DeliveryStore{Core: f.s}
	m, err := d.Claim(f.ctx, 30*time.Second)
	if err != nil || m == nil {
		t.Fatal(err)
	}
	if err = d.Finish(f.ctx, *m, deliveryoutbox.Outcome{Status: "sent", ACK: "9"}); err != nil {
		t.Fatal(err)
	}
	i := telegramrich.Ingress{Store: f.s, Binding: telegramrich.OwnerBinding{OwnerID: f.p.OwnerID, BindingID: f.bindings["telegram"], Version: 2, ChatID: 42, UserID: 42, BotID: 77}}
	update := func() telegramrich.Update {
		return telegramrich.Update{ID: 5, Message: &telegramrich.Message{ID: 10, Chat: telegramrich.Chat{ID: 42, Type: "private"}, From: telegramrich.User{ID: 42}, Text: "Продолжай", Reply: &telegramrich.Message{ID: 9, Chat: telegramrich.Chat{ID: 42, Type: "private"}, From: telegramrich.User{ID: 77, IsBot: true}}}}
	}
	for _, attack := range []string{"wrong-user", "group", "wrong-bot", "no-reply", "unknown-ack"} {
		u := update()
		switch attack {
		case "wrong-user":
			u.Message.From.ID = 43
		case "group":
			u.Message.Chat.Type = "group"
		case "wrong-bot":
			u.Message.Reply.From.ID = 78
		case "no-reply":
			u.Message.Reply = nil
		case "unknown-ack":
			u.Message.Reply.ID = 999
		}
		if err = i.Reply(f.ctx, u); err == nil {
			t.Fatal("forged ingress accepted", attack)
		}
	}
	if err = i.Reply(f.ctx, update()); err != nil {
		t.Fatal(err)
	}
	if err = i.Reply(f.ctx, update()); err != nil {
		t.Fatal("duplicate update not idempotent", err)
	}
	var key, owner string
	var count int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*),min(session_key),min(owner_id) FROM accepted_inputs WHERE source='telegram' AND event_id='telegram-rich:77:5'`).Scan(&count, &key, &owner); err != nil || count != 1 || key != f.p.SessionKey || owner != f.p.OwnerID {
		t.Fatal("reply lost canonical target", count, key, owner, err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE communication_bindings SET version=version+1 WHERE binding_id=$1`, f.bindings["telegram"]); err != nil {
		t.Fatal(err)
	}
	u := update()
	u.ID = 6
	if err = i.Reply(f.ctx, u); err == nil {
		t.Fatal("stale binding accepted")
	}
}

func TestCommunicationMediaMigrationRetainsHistoryIntegration(t *testing.T) {
	f, _ := mediaFixture(t)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(f.ctx, migration(t, "091_communication_media.down.sql")); err == nil {
		t.Fatal("rollback discarded retained media")
	}
}
