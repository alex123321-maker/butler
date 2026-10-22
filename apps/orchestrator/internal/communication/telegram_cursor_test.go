package communication_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
)

type cursorClient struct {
	bot     int64
	updates []telegramrich.Update
	offsets []int64
}

func (c *cursorClient) GetMe(context.Context) (telegramrich.User, error) {
	return telegramrich.User{ID: c.bot, IsBot: true}, nil
}
func (c *cursorClient) Updates(_ context.Context, offset int64) ([]telegramrich.Update, error) {
	c.offsets = append(c.offsets, offset)
	out := []telegramrich.Update{}
	for _, u := range c.updates {
		if u.ID >= offset {
			out = append(out, u)
		}
	}
	return out, nil
}

type replyFunc func(context.Context, telegramrich.Update) error

func (f replyFunc) Reply(ctx context.Context, u telegramrich.Update) error { return f(ctx, u) }

func cursorFixture(t *testing.T) (*fixture, *telegramrich.Poller, *cursorClient, telegramrich.Update) {
	t.Helper()
	f, _ := mediaFixture(t)
	if _, err := f.pool.Exec(f.ctx, `UPDATE communication_bindings SET enabled=FALSE WHERE binding_id=$1`, f.bindings["web"]); err != nil {
		t.Fatal(err)
	}
	if _, err := f.publish(t, "cursor-target", "communication.message", message); err != nil {
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
	b := telegramrich.OwnerBinding{OwnerID: f.p.OwnerID, BindingID: f.bindings["telegram"], Version: 2, ChatID: 42, UserID: 42, BotID: 77}
	u := telegramrich.Update{ID: 5, Message: &telegramrich.Message{ID: 10, Chat: telegramrich.Chat{ID: 42, Type: "private"}, From: telegramrich.User{ID: 42}, Text: "Продолжай", Reply: &telegramrich.Message{ID: 9, Chat: telegramrich.Chat{ID: 42, Type: "private"}, From: telegramrich.User{ID: 77, IsBot: true}}}}
	c := &cursorClient{bot: 77, updates: []telegramrich.Update{u}}
	return f, &telegramrich.Poller{Pool: f.pool, Client: c, Acceptor: &telegramrich.Ingress{Store: f.s, Binding: b}, Binding: b}, c, u
}

func TestCommunicationTelegramCursorCommitGapAndRestartIntegration(t *testing.T) {
	f, p, c, _ := cursorFixture(t)
	actual := p.Acceptor
	// The kernel accepts the reply, but the polling process loses its response
	// before cursor commit. This is the real cross-transaction crash boundary.
	p.Acceptor = replyFunc(func(ctx context.Context, u telegramrich.Update) error {
		if err := actual.Reply(ctx, u); err != nil {
			return err
		}
		return errors.New("fixture death before cursor commit")
	})
	if _, err := p.ProcessOnce(f.ctx); err == nil {
		t.Fatal("failure boundary missed")
	}
	var n int
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM accepted_inputs WHERE event_id='telegram-rich:77:5'`).Scan(&n); err != nil || n != 1 {
		t.Fatal("reply was not durable", n, err)
	}
	if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM communication_telegram_cursors`).Scan(&n); err != nil || n != 0 {
		t.Fatal("failed cursor transaction survived", n, err)
	}
	// A new poller replays the same update: Input identity prevents a second job.
	p = &telegramrich.Poller{Pool: f.pool, Client: c, Acceptor: actual, Binding: p.Binding}
	r, err := p.ProcessOnce(f.ctx)
	if err != nil || r.Accepted != 1 || r.NextOffset != 6 {
		t.Fatal(r, err)
	}
	r, err = p.ProcessOnce(f.ctx)
	if err != nil || r.Accepted != 0 || r.NextOffset != 6 || c.offsets[len(c.offsets)-1] != 6 {
		t.Fatal(r, err, c.offsets)
	}
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM accepted_inputs WHERE event_id='telegram-rich:77:5'`).Scan(&n); err != nil || n != 1 {
		t.Fatal("replay duplicated canonical Input", n, err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(f.ctx, migration(t, "092_telegram_communication_cursor.down.sql")); err == nil {
		t.Fatal("rollback discarded consumed cursor")
	}
}

func TestCommunicationTelegramCursorRejectsForgedAndUnavailableInputsIntegration(t *testing.T) {
	for _, attack := range []string{"wrong-bot", "stale-binding", "ambiguous", "unavailable", "indeterminate-denial", "out-of-order"} {
		t.Run(attack, func(t *testing.T) {
			f, p, c, u := cursorFixture(t)
			switch attack {
			case "wrong-bot":
				c.bot = 78
			case "stale-binding":
				p.Binding.Version = 1
			case "ambiguous":
				u.Message.Reply = nil
				c.updates = []telegramrich.Update{u}
			case "unavailable":
				p.Acceptor = replyFunc(func(context.Context, telegramrich.Update) error { return telegramrich.ErrUnavailable })
			case "indeterminate-denial":
				p.Acceptor = replyFunc(func(context.Context, telegramrich.Update) error { return communication.ErrDenied })
			case "out-of-order":
				other := u
				other.ID = 4
				c.updates = []telegramrich.Update{u, other}
			}
			r, err := p.ProcessOnce(f.ctx)
			if attack == "ambiguous" {
				if err != nil || r.Rejected != 1 || r.NextOffset != 6 {
					t.Fatal(r, err)
				}
			} else if err == nil {
				t.Fatal("unsafe cursor advanced", attack, r)
			}
			var n int
			if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM accepted_inputs WHERE source='telegram'`).Scan(&n); err != nil || n != 0 {
				t.Fatal("forged/ambiguous input routed", n, err)
			}
		})
	}
}
