package telegramrich

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Notices are fixed transport receipts, distinct from agent publications and
// Task completion. There is no model-selected text, template, artifact or URL.
type Notices struct {
	Pool    *pgxpool.Pool
	Client  *Client
	Binding OwnerBinding
}

func noticeText(code string) string {
	switch code {
	case "target_required":
		return "Уточните, о какой задаче речь: ответьте на её сообщение или укажите команду с TASK_ID, например /stop TASK_ID. Никаких изменений не сделано."
	case "unsupported_input":
		return "Этот формат или команда пока недоступны. Можно написать обычное сообщение или ответить на опубликованный результат."
	case "interaction_required":
		return "Не удалось однозначно связать ответ с вопросом. Ответьте на сообщение с нужным вопросом или используйте его кнопки. Ответ не принят."
	case "reply_target_unavailable":
		return "Не удалось подтвердить задачу по этому сообщению. Ответьте на актуальный результат или напишите новое сообщение. Изменений не сделано."
	}
	return ""
}

func (n *Notices) binding(ctx context.Context, tx pgx.Tx) error {
	var yes bool
	b := n.Binding
	return tx.QueryRow(ctx, `SELECT TRUE FROM communication_bindings WHERE binding_id=$1 AND version=$2 AND owner_id=$3 AND channel='telegram' AND recipient=$4 AND enabled AND verified FOR SHARE`, b.BindingID, b.Version, b.OwnerID, "telegram:"+strconv.FormatInt(b.ChatID, 10)).Scan(&yes)
}

func (n *Notices) Notify(ctx context.Context, u Update, code string) error {
	if n == nil || n.Pool == nil || n.Client == nil || noticeText(code) == "" || u.ID < 0 || u.Message == nil || u.Message.Chat.Type != "private" || u.Message.Chat.ID != n.Binding.ChatID || u.Message.From.ID != n.Binding.UserID || u.Message.From.IsBot {
		return ErrUnavailable
	}
	me, err := n.Client.GetMe(ctx)
	if err != nil || me.ID != n.Binding.BotID {
		return ErrUnavailable
	}
	tx, err := n.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err = n.binding(ctx, tx); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO communication_telegram_notices(bot_id,update_id,binding_id,binding_version,code,status) VALUES($1,$2,$3,$4,$5,'sending') ON CONFLICT DO NOTHING`, n.Binding.BotID, u.ID, n.Binding.BindingID, n.Binding.Version, code)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, err = tx.Exec(ctx, `UPDATE communication_telegram_notices SET status='delivery_unknown' WHERE bot_id=$1 AND update_id=$2 AND status='sending'`, n.Binding.BotID, u.ID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	// A separate durable transaction has fenced any replay before the network.
	tx, err = n.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	err = n.binding(ctx, tx)
	_ = tx.Rollback(context.Background())
	out := deliveryoutbox.Outcome{Status: "failed"}
	if err == nil {
		out = n.Client.sendNotice(ctx, n.Binding.ChatID, code)
	}
	finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, finishErr := n.Pool.Exec(finish, `UPDATE communication_telegram_notices SET status=$3,channel_ack=$4 WHERE bot_id=$1 AND update_id=$2 AND status='sending'`, n.Binding.BotID, u.ID, out.Status, out.ACK)
	return finishErr
}

func (c *Client) sendNotice(ctx context.Context, chat int64, code string) deliveryoutbox.Outcome {
	text := noticeText(code)
	if text == "" {
		return deliveryoutbox.Outcome{Status: "failed"}
	}
	body, _ := json.Marshal(map[string]any{"chat_id": chat, "text": text, "protect_content": true})
	e, status, err := c.call(ctx, "sendMessage", body, "application/json")
	if err != nil {
		return deliveryoutbox.Outcome{Status: "delivery_unknown"}
	}
	if !*e.OK {
		if e.Code >= 400 && e.Code <= 599 && status >= 400 && status <= 599 {
			return deliveryoutbox.Outcome{Status: "failed"}
		}
		return deliveryoutbox.Outcome{Status: "delivery_unknown"}
	}
	var m Message
	if status != 200 || json.Unmarshal(e.Result, &m) != nil || m.ID <= 0 || m.Chat.ID != chat {
		return deliveryoutbox.Outcome{Status: "delivery_unknown"}
	}
	return deliveryoutbox.Outcome{Status: "sent", ACK: strconv.FormatInt(m.ID, 10)}
}
