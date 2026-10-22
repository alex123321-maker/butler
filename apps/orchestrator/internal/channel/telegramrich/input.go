package telegramrich

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/owneridentity"
)

type Update struct {
	ID       int64          `json:"update_id"`
	Message  *Message       `json:"message,omitempty"`
	Callback *CallbackQuery `json:"callback_query,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data"`
}

// OwnerBinding is trusted operator configuration, never inferred from chat text.
// Each bot has its own binding ID. Bots/channels/groups/forwarded replies do not
// acquire an owner's identity merely by mentioning a publication/message ID.
type OwnerBinding struct {
	OwnerID   string
	BindingID string
	Version   int64
	ChatID    int64
	UserID    int64
	BotID     int64
}

type Ingress struct {
	Store    *communication.Store
	Controls *task.Controller
	Binding  OwnerBinding
	// Conversation is an owning Input admission hook, never a model executor.
	// Replies always retain canonical ACK resolution, including invalid replies.
	Conversation func(context.Context, Update) error
	Widgets      bool
}

var ErrRejectedUpdate = errors.New("Telegram update is outside the owning binding")

func (i *Ingress) Reply(ctx context.Context, u Update) error {
	if i == nil || i.Store == nil {
		return ErrUnavailable
	}
	b := i.Binding
	m := u.Message
	if u.Callback != nil {
		c := u.Callback
		if !i.Widgets || u.Message != nil || c.ID == "" || len(c.ID) > 128 || c.Message == nil || c.Message.ID <= 0 || c.From.ID != b.UserID || c.From.IsBot || c.Message.Chat.Type != "private" || c.Message.Chat.ID != b.ChatID || !c.Message.From.IsBot || c.Message.From.ID != b.BotID {
			return ErrRejectedUpdate
		}
		actor := owneridentity.Actor{OwnerID: b.OwnerID, Channel: "telegram", SessionKey: "telegram:" + strconv.FormatInt(b.ChatID, 10), DeliveryID: "telegram-widget:" + strconv.FormatInt(b.BotID, 10) + ":" + strconv.FormatInt(u.ID, 10)}
		_, err := i.Store.TelegramWidgetAction(owneridentity.WithActor(ctx, actor), communication.Binding{ID: b.BindingID, Version: b.Version, Channel: "telegram"}, strconv.FormatInt(c.Message.ID, 10), c.Data, "")
		return err
	}
	if b.OwnerID == "" || b.BindingID == "" || b.Version <= 0 || b.ChatID <= 0 || b.UserID <= 0 || b.BotID <= 0 {
		return ErrUnavailable
	}
	if u.ID < 0 || m == nil || m.ID <= 0 || m.Chat.Type != "private" || m.Chat.ID != b.ChatID || m.From.IsBot || m.From.ID != b.UserID || m.Text == "" {
		return ErrRejectedUpdate
	}
	actor := owneridentity.Actor{OwnerID: b.OwnerID, Channel: "telegram", SessionKey: "telegram:" + strconv.FormatInt(b.ChatID, 10), DeliveryID: "telegram-rich:" + strconv.FormatInt(b.BotID, 10) + ":" + strconv.FormatInt(u.ID, 10)}
	ctx = owneridentity.WithActor(ctx, actor)
	if m.Reply == nil {
		if i.Conversation == nil {
			return communication.ErrAmbiguous
		}
		if !utf8.ValidString(m.Text) || utf8.RuneCountInString(m.Text) > 4000 || strings.TrimSpace(m.Text) == "" {
			return ErrRejectedUpdate
		}
		return i.Conversation(ctx, u)
	}
	if m.Reply.ID <= 0 || m.Reply.Chat.ID != b.ChatID || !m.Reply.From.IsBot || m.Reply.From.ID != b.BotID {
		return ErrRejectedUpdate
	}
	q := communication.ReplyRequest{BindingID: b.BindingID, BindingVersion: b.Version, ChannelACK: strconv.FormatInt(m.Reply.ID, 10), Text: m.Text}
	if kind, _ := task.ClassifyOwnerText(m.Text); kind != "" {
		_, err := i.Store.ControlReply(ctx, i.Controls, q)
		return err
	}
	if strings.HasPrefix(m.Text, "/") {
		return ErrRejectedUpdate
	}
	_, err := i.Store.Reply(ctx, q)
	if errors.Is(err, communication.ErrInteractionRequired) && i.Widgets {
		_, err = i.Store.TelegramWidgetAction(ctx, communication.Binding{ID: b.BindingID, Version: b.Version, Channel: "telegram"}, q.ChannelACK, "", m.Text)
	}
	return err
}

// Updates does not own a cursor. The polling composition must persist/accept
// each update before advancing its offset. Replays use durable Input identity.
// No poller, webhook or communication admission is registered by default.
func (c *Client) Updates(ctx context.Context, offset int64) ([]Update, error) {
	if offset < 0 {
		return nil, ErrUnavailable
	}
	body, _ := json.Marshal(map[string]any{"offset": offset, "limit": 100, "timeout": 0, "allowed_updates": []string{"message", "callback_query"}})
	e, status, err := c.call(ctx, "getUpdates", body, "application/json")
	var out []Update
	if err != nil || status != 200 || !*e.OK || json.Unmarshal(e.Result, &out) != nil || len(out) > 100 {
		return nil, ErrUnavailable
	}
	return out, nil
}
