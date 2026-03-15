package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL        string
	botToken       string
	httpClient     *http.Client
	sendMessage    func(context.Context, int64, string) (Message, error)
	editMessage    func(context.Context, int64, int64, string) (Message, error)
	answerCallback func(context.Context, string, string) error
}

type apiError struct {
	StatusCode  int
	Description string
	Retryable   bool
}

func (e *apiError) Error() string {
	if e == nil {
		return ""
	}
	if e.Description == "" {
		return fmt.Sprintf("telegram api error (%d)", e.StatusCode)
	}
	return e.Description
}

type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Date      int64  `json:"date"`
	Text      string `json:"text"`
	Chat      Chat   `json:"chat"`
	From      *User  `json:"from,omitempty"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    *User    `json:"from,omitempty"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
}

type getUpdatesRequest struct {
	Offset         int64    `json:"offset,omitempty"`
	Timeout        int      `json:"timeout,omitempty"`
	AllowedUpdates []string `json:"allowed_updates,omitempty"`
}

type sendMessageRequest struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

type sendMessageWithMarkupRequest struct {
	ChatID      int64                 `json:"chat_id"`
	Text        string                `json:"text"`
	ReplyMarkup *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
}

type editMessageTextRequest struct {
	ChatID    int64  `json:"chat_id"`
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
}

type answerCallbackQueryRequest struct {
	CallbackQueryID string `json:"callback_query_id"`
	Text            string `json:"text,omitempty"`
}

type apiEnvelope[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
	ErrorCode   int    `json:"error_code"`
}

func NewClient(baseURL, botToken string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(botToken) == "" {
		return nil, fmt.Errorf("telegram bot token is required")
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.telegram.org"
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 40 * time.Second}
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		botToken:   botToken,
		httpClient: httpClient,
	}, nil
}

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	response, err := doTelegramRequest[[]Update](ctx, c, "getUpdates", getUpdatesRequest{
		Offset:         offset,
		Timeout:        timeoutSeconds,
		AllowedUpdates: []string{"message", "callback_query"},
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) (Message, error) {
	if c.sendMessage != nil {
		return c.sendMessage(ctx, chatID, text)
	}
	return doTelegramRequest[Message](ctx, c, "sendMessage", sendMessageRequest{ChatID: chatID, Text: text})
}

func (c *Client) SendMessageWithReplyMarkup(ctx context.Context, chatID int64, text string, markup *InlineKeyboardMarkup) (Message, error) {
	return doTelegramRequest[Message](ctx, c, "sendMessage", sendMessageWithMarkupRequest{ChatID: chatID, Text: text, ReplyMarkup: markup})
}

func (c *Client) AnswerCallbackQuery(ctx context.Context, callbackQueryID, text string) error {
	if c.answerCallback != nil {
		return c.answerCallback(ctx, callbackQueryID, text)
	}
	_, err := doTelegramRequest[bool](ctx, c, "answerCallbackQuery", answerCallbackQueryRequest{CallbackQueryID: callbackQueryID, Text: text})
	return err
}

func (c *Client) EditMessageText(ctx context.Context, chatID, messageID int64, text string) (Message, error) {
	if c.editMessage != nil {
		return c.editMessage(ctx, chatID, messageID, text)
	}
	return doTelegramRequest[Message](ctx, c, "editMessageText", editMessageTextRequest{ChatID: chatID, MessageID: messageID, Text: text})
}

func (c *Client) endpoint(method string) string {
	return c.baseURL + "/bot" + c.botToken + "/" + method
}

func doTelegramRequest[T any](ctx context.Context, client *Client, method string, payload any) (T, error) {
	var zero T
	body, err := json.Marshal(payload)
	if err != nil {
		return zero, fmt.Errorf("marshal telegram %s request: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint(method), bytes.NewReader(body))
	if err != nil {
		return zero, fmt.Errorf("create telegram %s request: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.httpClient.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, fmt.Errorf("read telegram %s response: %w", method, err)
	}

	var envelope apiEnvelope[T]
	if err := json.Unmarshal(bodyBytes, &envelope); err != nil {
		return zero, fmt.Errorf("decode telegram %s response: %w", method, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !envelope.OK {
		return zero, &apiError{
			StatusCode:  resp.StatusCode,
			Description: strings.TrimSpace(envelope.Description),
			Retryable:   resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests,
		}
	}
	return envelope.Result, nil
}
