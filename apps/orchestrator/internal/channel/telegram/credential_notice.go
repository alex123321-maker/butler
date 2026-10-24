package telegram

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/butler/butler/internal/credentials"
)

// CredentialNotice is called only by Core's durable protected-request worker.
// It has no chat-input or model route and accepts no credential value.
func (a *Adapter) CredentialNotice(ctx context.Context, session string, r credentials.AccessRequest, webURL, ack string) (string, error) {
	if a == nil || a.client == nil || a.explicit != nil {
		return "", ErrExplicitUnsupported
	}
	chat, ok := ChatIDFromSessionKey(session)
	if !ok || !a.isAllowedChatID(chat) {
		return "", ErrIgnoredUpdate
	}
	u, err := url.Parse(webURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(r.ID, "creq_") {
		return "", ErrExplicitUnsupported
	}
	title := "Разрешить использование учётных данных?"
	if r.Kind == "create" {
		title = "Нужно создать учётные данные"
	}
	if r.Kind == "otp" {
		title = "Нужен одноразовый код для входа"
	}
	text := title + "\nСайт: " + r.TargetOrigin + "\nЦель: " + r.Purpose + "\nПоля: " + strings.Join(r.Fields, ", ") + "\nВведите данные в защищённой форме Butler."
	if r.RecordLabel != "" {
		text += "\nЗапись: " + r.RecordLabel
	}
	body := map[string]any{"chat_id": chat, "text": text, "protect_content": true}
	method := "sendMessage"
	if ack != "" {
		id, e := strconv.ParseInt(ack, 10, 64)
		if e != nil || id <= 0 {
			return "", ErrExplicitUnsupported
		}
		body["message_id"] = id
		method = "editMessageText"
	}
	if r.State == "pending" {
		body["reply_markup"] = map[string]any{"inline_keyboard": [][]map[string]string{{{"text": "Открыть защищённую форму", "url": strings.TrimRight(webURL, "/") + "/#credential=" + r.ID}}}}
	} else {
		body["text"] = "Запрос учётных данных закрыт: " + r.State
		body["reply_markup"] = map[string]any{"inline_keyboard": []any{}}
	}
	message, err := doTelegramRequest[Message](ctx, a.client, method, body)
	if err != nil || message.MessageID <= 0 {
		return "", ErrExplicitUnsupported
	}
	return strconv.FormatInt(message.MessageID, 10), nil
}
