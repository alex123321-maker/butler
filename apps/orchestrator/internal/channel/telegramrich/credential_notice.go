package telegramrich

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/butler/butler/internal/credentials"
)

// CredentialNotice sends a fixed control card. Secret input uses the protected
// owner Web form; no callback or chat reply can provide or authorize a secret.
func (c *Client) CredentialNotice(ctx context.Context, b OwnerBinding, r credentials.AccessRequest, webURL, ack string) (string, error) {
	u, err := url.Parse(webURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || b.ChatID <= 0 || b.UserID != b.ChatID || !strings.HasPrefix(r.ID, "creq_") {
		return "", ErrUnavailable
	}
	me, err := c.GetMe(ctx)
	if err != nil || !me.IsBot || me.ID != b.BotID {
		return "", ErrUnavailable
	}
	title := "Разрешить использование учётных данных?"
	if r.Kind == "create" {
		title = "Нужно создать учётные данные"
	}
	if r.Kind == "otp" {
		title = "Нужен одноразовый код для входа"
	}
	if r.Kind == "login" {
		title = "Разрешить один вход с проверкой доступа?"
	}
	text := title + "\nСайт: " + r.TargetOrigin + "\nЦель: " + r.Purpose + "\nПоля: " + strings.Join(r.Fields, ", ") + "\nВводите данные только в защищённой форме Butler."
	if r.RecordLabel != "" {
		text += "\nЗапись: " + r.RecordLabel
	}
	fields := map[string]any{"chat_id": b.ChatID, "text": text, "protect_content": true}
	method := "sendMessage"
	if ack != "" {
		message, e := strconv.ParseInt(ack, 10, 64)
		if e != nil || message <= 0 {
			return "", ErrUnavailable
		}
		fields["message_id"] = message
		method = "editMessageText"
	}
	if r.State == "pending" {
		fields["reply_markup"] = map[string]any{"inline_keyboard": [][]map[string]string{{{"text": "Открыть защищённую форму", "url": strings.TrimRight(webURL, "/") + "/#credential=" + r.ID}}}}
	} else {
		fields["text"] = "Запрос учётных данных закрыт: " + r.State
		fields["reply_markup"] = map[string]any{"inline_keyboard": []any{}}
	}
	raw, _ := json.Marshal(fields)
	response, status, err := c.call(ctx, method, raw, "application/json")
	if err != nil || status != 200 || response.OK == nil || !*response.OK {
		return "", ErrUnavailable
	}
	var message Message
	if json.Unmarshal(response.Result, &message) != nil || message.ID <= 0 || message.Chat.ID != b.ChatID {
		return "", ErrUnavailable
	}
	return strconv.FormatInt(message.ID, 10), nil
}
