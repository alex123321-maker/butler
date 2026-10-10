package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"io"
	"net/http"
	"strconv"
	"time"
)

// SendStored performs exactly one channel attempt. Plain text avoids a second
// parse-mode fallback send, and every chunk is acknowledged independently.
func (a *Adapter) SendStored(ctx context.Context, m deliveryoutbox.Message) deliveryoutbox.Outcome {
	chatID, ok := ChatIDFromSessionKey(m.SessionKey)
	if !ok || !a.isAllowedChatID(chatID) || m.Channel != "telegram" {
		return deliveryoutbox.Outcome{Status: "absent", ErrorCode: "telegram_chat_not_allowed"}
	}
	outcome := a.client.sendStoredPlain(ctx, chatID, m.Content)
	if outcome.Status == "sent" {
		a.mu.Lock()
		delete(a.streamingMessages, m.RunID)
		a.mu.Unlock()
	}
	return outcome
}

func (c *Client) sendStoredPlain(ctx context.Context, chatID int64, text string) deliveryoutbox.Outcome {
	unknown := deliveryoutbox.Outcome{Status: "delivery_unknown", ErrorCode: "transport_outcome_unknown"}
	body, err := json.Marshal(sendMessageRequest{ChatID: chatID, Text: text})
	if err != nil {
		return unknown
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint("sendMessage"), bytes.NewReader(body))
	if err != nil {
		return unknown
	}
	req.Header.Set("Content-Type", "application/json")
	// A redirect could leak the bot token URL or duplicate a POST.
	client := *c.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return unknown
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20+1))
	if err != nil || len(data) > 1<<20 {
		return unknown
	}
	var envelope struct {
		OK         *bool   `json:"ok"`
		Result     Message `json:"result"`
		Parameters struct {
			RetryAfter int `json:"retry_after"`
		} `json:"parameters"`
		ErrorCode int `json:"error_code"`
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.OK == nil {
		unknown.ErrorCode = "response_outcome_unknown"
		return unknown
	}
	if *envelope.OK {
		if response.StatusCode < 200 || response.StatusCode >= 300 || envelope.Result.MessageID <= 0 || envelope.Result.Chat.ID != chatID {
			return unknown
		}
		return deliveryoutbox.Outcome{Status: "sent", ACK: strconv.FormatInt(envelope.Result.MessageID, 10)}
	}
	status := envelope.ErrorCode
	if status == 0 {
		status = response.StatusCode
	}
	return deliveryoutbox.Outcome{Status: "absent", ErrorCode: "telegram_rejected", Retryable: status == 429 || status >= 500, RetryAfter: time.Duration(envelope.Parameters.RetryAfter) * time.Second}
}
