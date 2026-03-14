package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/logger"
)

var ErrIgnoredUpdate = errors.New("telegram update ignored")

type Executor interface {
	Execute(context.Context, ingress.InputEvent) (*flow.ExecutionResult, error)
}

type Config struct {
	AllowedChatIDs []string
	PollTimeout    time.Duration
}

type Adapter struct {
	client            *Client
	allowedChatIDs    map[string]struct{}
	pollTimeoutSecond int
	executor          Executor
	log               *slog.Logger
	nextOffset        int64
}

func NewAdapter(cfg Config, client *Client, log *slog.Logger) (*Adapter, error) {
	if client == nil {
		return nil, fmt.Errorf("telegram client is required")
	}
	if len(cfg.AllowedChatIDs) == 0 {
		return nil, fmt.Errorf("at least one allowed chat id is required")
	}
	if cfg.PollTimeout <= 0 {
		cfg.PollTimeout = 25 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	allowed := make(map[string]struct{}, len(cfg.AllowedChatIDs))
	for _, chatID := range cfg.AllowedChatIDs {
		allowed[strings.TrimSpace(chatID)] = struct{}{}
	}
	return &Adapter{
		client:            client,
		allowedChatIDs:    allowed,
		pollTimeoutSecond: int(cfg.PollTimeout / time.Second),
		log:               logger.WithComponent(log, "telegram-adapter"),
	}, nil
}

func (a *Adapter) SetExecutor(executor Executor) {
	a.executor = executor
}

func (a *Adapter) Run(ctx context.Context) error {
	if a.executor == nil {
		return fmt.Errorf("telegram executor is not configured")
	}
	nextOffset, err := a.primeOffset(ctx)
	if err != nil {
		return err
	}
	a.nextOffset = nextOffset
	a.log.Info("telegram adapter started", slog.Int("allowed_chat_count", len(a.allowedChatIDs)), slog.Int64("next_offset", a.nextOffset))

	for {
		select {
		case <-ctx.Done():
			a.log.Info("telegram adapter stopped")
			return nil
		default:
		}

		updates, err := a.client.GetUpdates(ctx, a.nextOffset, a.pollTimeoutSecond)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			var apiErr *apiError
			if errors.As(err, &apiErr) && !apiErr.Retryable {
				return fmt.Errorf("telegram polling failed permanently: %w", err)
			}
			a.log.Warn("telegram polling failed", slog.String("error", err.Error()))
			time.Sleep(2 * time.Second)
			continue
		}

		for _, update := range updates {
			if update.UpdateID >= a.nextOffset {
				a.nextOffset = update.UpdateID + 1
			}
			event, err := NormalizeUpdate(update)
			if err != nil {
				if errors.Is(err, ErrIgnoredUpdate) {
					continue
				}
				a.log.Warn("telegram update normalization failed", slog.Int64("update_id", update.UpdateID), slog.String("error", err.Error()))
				continue
			}
			if !a.isAllowedSession(event.SessionKey) {
				a.log.Warn("telegram update ignored for disallowed chat", slog.Int64("update_id", update.UpdateID), slog.String("session_key", event.SessionKey))
				continue
			}
			result, execErr := a.executor.Execute(ctx, event)
			if execErr != nil {
				a.log.Error("telegram update execution failed", slog.Int64("update_id", update.UpdateID), slog.String("session_key", event.SessionKey), slog.String("error", execErr.Error()))
				continue
			}
			a.log.Info("telegram update executed", slog.Int64("update_id", update.UpdateID), slog.String("run_id", result.RunID), slog.String("session_key", result.SessionKey))
		}
	}
}

func (a *Adapter) DeliverAssistantDelta(context.Context, flow.DeliveryEvent) error {
	return nil
}

func (a *Adapter) DeliverAssistantFinal(ctx context.Context, event flow.DeliveryEvent) error {
	chatID, ok := ChatIDFromSessionKey(event.SessionKey)
	if !ok || !a.isAllowedChatID(chatID) || strings.TrimSpace(event.Content) == "" {
		return nil
	}
	if err := a.client.SendMessage(ctx, chatID, event.Content); err != nil {
		return fmt.Errorf("send telegram message: %w", err)
	}
	a.log.Info("telegram final response delivered", slog.String("run_id", event.RunID), slog.String("session_key", event.SessionKey))
	return nil
}

func NormalizeUpdate(update Update) (ingress.InputEvent, error) {
	if update.Message == nil {
		return ingress.InputEvent{}, ErrIgnoredUpdate
	}
	text := strings.TrimSpace(update.Message.Text)
	if text == "" {
		return ingress.InputEvent{}, ErrIgnoredUpdate
	}
	payload := map[string]any{
		"text":       text,
		"chat_id":    strconv.FormatInt(update.Message.Chat.ID, 10),
		"message_id": strconv.FormatInt(update.Message.MessageID, 10),
	}
	if update.Message.From != nil {
		payload["user_id"] = strconv.FormatInt(update.Message.From.ID, 10)
		payload["username"] = update.Message.From.Username
		payload["first_name"] = update.Message.From.FirstName
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return ingress.InputEvent{}, fmt.Errorf("marshal telegram payload: %w", err)
	}
	createdAt := time.Now().UTC()
	if update.Message.Date > 0 {
		createdAt = time.Unix(update.Message.Date, 0).UTC()
	}
	eventID := fmt.Sprintf("telegram-update-%d", update.UpdateID)
	return ingress.InputEvent{
		EventID:        eventID,
		EventType:      runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE,
		SessionKey:     SessionKeyFromChatID(update.Message.Chat.ID),
		Source:         "telegram",
		PayloadJSON:    string(payloadJSON),
		CreatedAt:      createdAt,
		IdempotencyKey: eventID,
	}, nil
}

func SessionKeyFromChatID(chatID int64) string {
	return fmt.Sprintf("telegram:chat:%d", chatID)
}

func ChatIDFromSessionKey(sessionKey string) (int64, bool) {
	const prefix = "telegram:chat:"
	if !strings.HasPrefix(sessionKey, prefix) {
		return 0, false
	}
	chatID, err := strconv.ParseInt(strings.TrimPrefix(sessionKey, prefix), 10, 64)
	if err != nil {
		return 0, false
	}
	return chatID, true
}

func (a *Adapter) primeOffset(ctx context.Context) (int64, error) {
	updates, err := a.client.GetUpdates(ctx, 0, 1)
	if err != nil {
		return 0, fmt.Errorf("prime telegram update offset: %w", err)
	}
	var nextOffset int64
	for _, update := range updates {
		if update.UpdateID >= nextOffset {
			nextOffset = update.UpdateID + 1
		}
	}
	return nextOffset, nil
}

func (a *Adapter) isAllowedSession(sessionKey string) bool {
	chatID, ok := ChatIDFromSessionKey(sessionKey)
	if !ok {
		return false
	}
	return a.isAllowedChatID(chatID)
}

func (a *Adapter) isAllowedChatID(chatID int64) bool {
	_, ok := a.allowedChatIDs[strconv.FormatInt(chatID, 10)]
	return ok
}
