package app

import (
	"context"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestCommunicationDeferredTelegramIdentityRequiresExistingExactOwnerMetadataIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	o := TelegramCommunicationOptions{OwnerID: "owner", BindingID: "bot", BindingVersion: 1, WebBindingID: "web", WebBindingVersion: 1, ChatID: 42, ownerDatabase: pool.Config().ConnConfig.Database, expectedBotID: 77}
	if deferredTelegramIdentityAllowed(ctx, pool, o) {
		t.Fatal("first boot deferred identity")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO communication_bindings(binding_id,owner_id,channel,recipient,version,enabled,verified,capabilities) VALUES('bot','owner','telegram','telegram:42',1,TRUE,TRUE,'["message"]'),('web','owner','web','web:owner',1,TRUE,TRUE,'["message"]')`); err != nil {
		t.Fatal(err)
	}
	if deferredTelegramIdentityAllowed(ctx, pool, o) {
		t.Fatal("missing verified bot cursor accepted")
	}
	if err := pinTelegramBotAt(ctx, pool, communication.Binding{ID: "bot", Version: 1}, 77, 123); err != nil {
		t.Fatal(err)
	}
	if !deferredTelegramIdentityAllowed(ctx, pool, o) {
		t.Fatal("verified restart metadata denied")
	}
	for _, mode := range []string{"foreign_owner", "wrong_bot", "version", "recipient", "no_production_profile", "no_web"} {
		bad := o
		switch mode {
		case "foreign_owner":
			bad.OwnerID = "other"
		case "wrong_bot":
			bad.expectedBotID = 88
		case "version":
			bad.BindingVersion = 2
		case "recipient":
			bad.ChatID = 43
		case "no_production_profile":
			bad.ownerDatabase = ""
		case "no_web":
			bad.WebBindingID = ""
		}
		if deferredTelegramIdentityAllowed(ctx, pool, bad) {
			t.Fatal("invalid deferred identity", mode)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE communication_bindings SET enabled=FALSE WHERE binding_id='bot'`); err != nil {
		t.Fatal(err)
	}
	if deferredTelegramIdentityAllowed(ctx, pool, o) {
		t.Fatal("revoked metadata restored")
	}
}

func TestCommunicationTelegramBotIdentityPinnedBeforeQueueIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO communication_bindings(binding_id,owner_id,channel,recipient,version,enabled,verified,capabilities) VALUES('bot','owner','telegram','telegram:42',1,TRUE,TRUE,'["message"]'),('other','owner','telegram','telegram:42',1,TRUE,TRUE,'["message"]')`); err != nil {
		t.Fatal(err)
	}
	b := communication.Binding{ID: "bot", Version: 1}
	if err := pinTelegramBotAt(ctx, pool, b, 77, 123); err != nil {
		t.Fatal(err)
	}
	if err := pinTelegramBotAt(ctx, pool, b, 77, 999); err != nil {
		t.Fatal("same bot restart denied", err)
	}
	var offset int64
	if err := pool.QueryRow(ctx, `SELECT next_update_id FROM communication_telegram_cursors WHERE bot_id=77`).Scan(&offset); err != nil || offset != 123 {
		t.Fatal("restart reset durable cutover offset", offset, err)
	}
	if err := pinTelegramBot(ctx, pool, b, 88); err == nil {
		t.Fatal("different bot adopted binding")
	}
	if err := pinTelegramBot(ctx, pool, communication.Binding{ID: "other", Version: 1}, 77); err == nil {
		t.Fatal("same bot adopted different binding")
	}
	if err := pinTelegramBot(ctx, pool, communication.Binding{ID: "bot", Version: 2}, 77); err == nil {
		t.Fatal("wrong binding version admitted")
	}
	if _, err := pool.Exec(ctx, `UPDATE communication_bindings SET enabled=FALSE WHERE binding_id='bot'`); err != nil {
		t.Fatal(err)
	}
	if err := pinTelegramBot(ctx, pool, b, 77); err == nil {
		t.Fatal("revoked binding admitted")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM communication_telegram_cursors WHERE bot_id=77 AND binding_id='bot'`).Scan(&count); err != nil || count != 1 {
		t.Fatal("identity changed", count, err)
	}
}
