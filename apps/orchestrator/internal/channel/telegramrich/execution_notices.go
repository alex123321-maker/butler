package telegramrich

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
)

// ExecutionSender sends only owning, source-free lifecycle records. It cannot
// publish model text, execute an action, or resolve an approval.
type ExecutionSender struct {
	Client    *Client
	Binding   OwnerBinding
	Authorize func(context.Context, deliveryoutbox.Message) error
}

func (s *ExecutionSender) SendStored(ctx context.Context, m deliveryoutbox.Message) deliveryoutbox.Outcome {
	absent := deliveryoutbox.Outcome{Status: "absent", ErrorCode: "execution_notice_denied"}
	if s == nil || s.Client == nil || s.Authorize == nil || m.BindingID != s.Binding.BindingID || m.BindingVersion != s.Binding.Version || m.Recipient != "telegram:"+strconv.FormatInt(s.Binding.ChatID, 10) || m.Channel != "telegram" || !strings.HasPrefix(m.NotificationID, "execution:") || m.PublicationID != "" || m.ResultID != m.NotificationID || m.Content == "" {
		return absent
	}
	me, err := s.Client.GetMe(ctx)
	if err != nil || me.ID != s.Binding.BotID {
		return absent
	}
	body, _ := json.Marshal(map[string]any{"chat_id": s.Binding.ChatID, "text": m.Content})
	if s.Authorize(ctx, m) != nil {
		return absent
	}
	e, status, err := s.Client.call(ctx, "sendMessage", body, "application/json")
	unknown := deliveryoutbox.Outcome{Status: "delivery_unknown", ErrorCode: "transport_outcome_unknown"}
	if err != nil {
		return unknown
	}
	if !*e.OK {
		if e.Code < 400 || e.Code > 599 || status < 400 || status > 599 {
			return unknown
		}
		absent.ErrorCode = "telegram_rejected"
		absent.Retryable = e.Code == 429 || e.Code >= 500
		if e.Parameters.RetryAfter > 0 && e.Parameters.RetryAfter <= 3600 {
			absent.RetryAfter = time.Duration(e.Parameters.RetryAfter) * time.Second
		}
		return absent
	}
	var message Message
	if status != 200 || json.Unmarshal(e.Result, &message) != nil || message.ID <= 0 || message.Chat.ID != s.Binding.ChatID {
		return unknown
	}
	return deliveryoutbox.Outcome{Status: "sent", ACK: strconv.FormatInt(message.ID, 10)}
}
