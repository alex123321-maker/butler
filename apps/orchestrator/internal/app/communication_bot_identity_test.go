package app

import (
	"context"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/memory/memorytest"
)

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
