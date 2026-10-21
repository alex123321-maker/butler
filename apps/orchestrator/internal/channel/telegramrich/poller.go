package telegramrich

import (
	"context"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UpdateClient interface {
	GetMe(context.Context) (User, error)
	Updates(context.Context, int64) ([]Update, error)
}
type ReplyAcceptor interface {
	Reply(context.Context, Update) error
}
type Poller struct {
	Pool     *pgxpool.Pool
	Client   UpdateClient
	Acceptor ReplyAcceptor
	Binding  OwnerBinding
}
type PollResult struct {
	NextOffset                  int64
	Accepted, Rejected, Ignored int
}

// ProcessOnce serializes this bot's bounded getUpdates request with a durable
// cursor. Input/control commit precedes cursor commit. A death between them
// replays the same bot/update delivery identity through the ordinary kernel;
// transport code never runs a provider, tool or task itself.
func (p *Poller) ProcessOnce(ctx context.Context) (PollResult, error) {
	var result PollResult
	if p == nil || p.Pool == nil || p.Client == nil || p.Acceptor == nil {
		return result, ErrUnavailable
	}
	b := p.Binding
	if b.BotID <= 0 || b.ChatID <= 0 || b.UserID != b.ChatID || b.OwnerID == "" || b.BindingID == "" || b.Version < 1 {
		return result, ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	me, err := p.Client.GetMe(ctx)
	if err != nil || !me.IsBot || me.ID != b.BotID {
		return result, ErrUnavailable
	}
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(context.Background())
	var current bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM communication_bindings WHERE binding_id=$1 AND version=$2 AND owner_id=$3 AND channel='telegram' AND recipient=$4 AND enabled AND verified FOR SHARE`, b.BindingID, b.Version, b.OwnerID, "telegram:"+strconv.FormatInt(b.ChatID, 10)).Scan(&current) != nil {
		return result, ErrUnavailable
	}
	if _, err = tx.Exec(ctx, `INSERT INTO communication_telegram_cursors(bot_id,binding_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, b.BotID, b.BindingID); err != nil {
		return result, err
	}
	if err = tx.QueryRow(ctx, `SELECT next_update_id FROM communication_telegram_cursors WHERE bot_id=$1 AND binding_id=$2 FOR UPDATE`, b.BotID, b.BindingID).Scan(&result.NextOffset); err != nil {
		return result, ErrUnavailable
	}
	updates, err := p.Client.Updates(ctx, result.NextOffset)
	if err != nil {
		return result, err
	}
	if len(updates) > 100 {
		return result, ErrUnavailable
	}
	// Validate the whole transport envelope before accepting the first update.
	last := result.NextOffset - 1
	for _, u := range updates {
		if u.ID < result.NextOffset || u.ID <= last || u.ID == math.MaxInt64 {
			return result, ErrUnavailable
		}
		last = u.ID
	}
	outcome := ""
	for _, u := range updates {
		m := u.Message
		if m == nil || m.Chat.Type != "private" || m.Chat.ID != b.ChatID || m.From.ID != b.UserID || m.From.IsBot {
			result.Ignored++
			outcome = "ignored"
		} else if err = p.Acceptor.Reply(ctx, u); err == nil {
			result.Accepted++
			outcome = "accepted"
		} else if errors.Is(err, communication.ErrAmbiguous) || errors.Is(err, communication.ErrInteractionRequired) || errors.Is(err, ErrRejectedUpdate) {
			// A rejected owner reply is not routed to a guessed Task or the legacy
			// executor. A caller can surface the typed decision to the owner.
			result.Rejected++
			outcome = "rejected"
		} else {
			// ErrDenied also masks failed guarded SQL reads. It is not proof of
			// permanent rejection: preserve the update/cursor on that boundary.
			return result, err
		}
		result.NextOffset = u.ID + 1
	}
	if len(updates) > 0 {
		if _, err = tx.Exec(ctx, `UPDATE communication_telegram_cursors SET next_update_id=$2,last_outcome=$3,updated_at=clock_timestamp() WHERE bot_id=$1`, b.BotID, result.NextOffset, outcome); err != nil {
			return result, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}
