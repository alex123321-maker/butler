package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	singletab "github.com/butler/butler/apps/orchestrator/internal/singletab"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/logger"
)

var ErrIgnoredUpdate = errors.New("telegram update ignored")

type Executor interface {
	Execute(context.Context, ingress.InputEvent) (*flow.ExecutionResult, error)
}

type BrowserTabSelector interface {
	ActivateFromApproval(ctx context.Context, params singletab.ActivateFromApprovalParams) (singletab.ActivationResult, error)
	ActivateFromCandidateToken(ctx context.Context, candidateToken string, params singletab.ActivateFromApprovalParams) (singletab.ActivationResult, error)
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
	auth              ProviderAuthManager
	approvalGate      *flow.ApprovalGate
	approvalService   *approvals.Service
	singleTabSelector BrowserTabSelector
	log               *slog.Logger
	nextOffset        int64
	now               func() time.Time
	mu                sync.Mutex
	streamingMessages map[string]streamingMessage
	authPrompts       map[int64]pendingAuthPrompt
}

type streamingMessage struct {
	ChatID        int64
	DraftID       int64
	Content       string
	SentLen       int
	LastSentAt    time.Time
	NextAllowedAt time.Time
}

const (
	minDraftFlushInterval = 900 * time.Millisecond
	minDraftFlushChars    = 48
)

func NewAdapter(cfg Config, client *Client, approvalGate *flow.ApprovalGate, log *slog.Logger) (*Adapter, error) {
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
		approvalGate:      approvalGate,
		log:               logger.WithComponent(log, "telegram-adapter"),
		now:               time.Now,
		streamingMessages: make(map[string]streamingMessage),
		authPrompts:       make(map[int64]pendingAuthPrompt),
	}, nil
}

func (a *Adapter) SetExecutor(executor Executor) {
	a.executor = executor
}

func (a *Adapter) SetApprovalService(service *approvals.Service) {
	a.approvalService = service
}

func (a *Adapter) SetSingleTabSelector(selector BrowserTabSelector) {
	a.singleTabSelector = selector
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

			a.dispatchUpdate(ctx, update)
		}
	}
}

func (a *Adapter) dispatchUpdate(ctx context.Context, update Update) {
	go func() {
		if update.CallbackQuery != nil {
			a.handleCallbackQuery(ctx, update.CallbackQuery)
			return
		}

		if err := a.handleMessageUpdate(ctx, update); err != nil {
			if errors.Is(err, ErrIgnoredUpdate) {
				return
			}
			a.log.Warn("telegram update handling failed", slog.Int64("update_id", update.UpdateID), slog.String("error", err.Error()))
		}
	}()
}

func (a *Adapter) handleMessageUpdate(ctx context.Context, update Update) error {
	if update.Message == nil {
		return ErrIgnoredUpdate
	}
	if !a.isAllowedChatID(update.Message.Chat.ID) {
		a.log.Warn("telegram update ignored for disallowed chat", slog.Int64("update_id", update.UpdateID), slog.Int64("chat_id", update.Message.Chat.ID))
		return ErrIgnoredUpdate
	}
	handled, err := a.handleCommand(ctx, update.Message)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	handled, err = a.handlePendingAuthInput(ctx, update.Message)
	if err != nil {
		return err
	}
	if handled {
		return nil
	}
	event, err := NormalizeUpdate(update)
	if err != nil {
		return err
	}

	// Show "typing" indicator immediately so the user sees the bot is working.
	if actionErr := a.client.SendChatAction(ctx, update.Message.Chat.ID, ChatActionTyping); actionErr != nil {
		a.log.Warn("send typing action failed", slog.String("error", actionErr.Error()))
	}

	result, execErr := a.executor.Execute(ctx, event)
	if execErr != nil {
		if a.shouldPromptForAuth(execErr) {
			if promptErr := a.deliverProviderAuthPrompt(ctx, update.Message.Chat.ID, "I could not process that yet because no working provider auth is available."); promptErr != nil {
				return fmt.Errorf("deliver provider auth fallback: %w", promptErr)
			}
			a.log.Warn("telegram update requires provider auth", slog.Int64("update_id", update.UpdateID), slog.String("session_key", event.SessionKey), slog.String("error", execErr.Error()))
			return nil
		}
		if notifyErr := a.sendExecutionFailureMessage(ctx, update.Message.Chat.ID, execErr); notifyErr != nil {
			return fmt.Errorf("execute telegram update: %w; additionally failed to notify chat: %v", execErr, notifyErr)
		}
		a.log.Warn("telegram update execution failed; user notified", slog.Int64("update_id", update.UpdateID), slog.String("session_key", event.SessionKey), slog.String("error", execErr.Error()))
		return nil
	}
	a.log.Info("telegram update executed", slog.Int64("update_id", update.UpdateID), slog.String("run_id", result.RunID), slog.String("session_key", result.SessionKey))
	return nil
}

func (a *Adapter) sendExecutionFailureMessage(ctx context.Context, chatID int64, err error) error {
	text := "I could not finish that request. Please try again."
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "client.timeout") || strings.Contains(strings.ToLower(err.Error()), "context deadline exceeded") {
		text = "I could not finish that request in time. Please try again, or ask me to be more specific so I can do less work in one run."
	}
	return a.sendFinalMessage(ctx, chatID, text)
}

func (a *Adapter) DeliverAssistantDelta(ctx context.Context, event flow.DeliveryEvent) error {
	if event.Content == "" {
		return nil
	}
	chatID, ok := ChatIDFromSessionKey(event.SessionKey)
	if !ok || !a.isAllowedChatID(chatID) {
		return nil
	}

	a.mu.Lock()
	state, exists := a.streamingMessages[event.RunID]
	if !exists {
		state = streamingMessage{
			ChatID:  chatID,
			DraftID: draftIDFromRunID(event.RunID),
		}
	}
	state.Content += event.Content
	now := a.now().UTC()
	shouldFlush := state.SentLen == 0 || len(state.Content)-state.SentLen >= minDraftFlushChars || now.Sub(state.LastSentAt) >= minDraftFlushInterval
	if now.Before(state.NextAllowedAt) || !shouldFlush {
		a.streamingMessages[event.RunID] = state
		a.mu.Unlock()
		return nil
	}
	a.streamingMessages[event.RunID] = state
	accumulated := state.Content
	draftID := state.DraftID
	a.mu.Unlock()

	// Telegram messages are limited to 4096 characters. During streaming we
	// send in-place drafts, so we truncate to the tail of the accumulated
	// content to keep the user seeing the most recent output. The full
	// content will be delivered properly in DeliverAssistantFinal.
	draftText := accumulated
	if len(draftText) > telegramMaxMessageLen {
		draftText = "…" + draftText[len(draftText)-telegramMaxMessageLen+len("…"):]
	}

	if err := a.client.SendMessageDraft(ctx, chatID, draftID, draftText); err != nil {
		var apiErr *apiError
		if errors.As(err, &apiErr) && apiErr.StatusCode == 429 {
			backoff := nextAllowedAt(now, apiErr.RetryAfter)
			a.mu.Lock()
			state := a.streamingMessages[event.RunID]
			state.NextAllowedAt = backoff
			a.streamingMessages[event.RunID] = state
			a.mu.Unlock()
			a.log.Warn("telegram draft rate limited", slog.String("run_id", event.RunID), slog.String("session_key", event.SessionKey), slog.Duration("retry_after", backoff.Sub(now)))
			return nil
		}
		return fmt.Errorf("send telegram message draft: %w", err)
	}
	a.mu.Lock()
	state = a.streamingMessages[event.RunID]
	state.SentLen = len(accumulated)
	state.LastSentAt = now
	state.NextAllowedAt = time.Time{}
	a.streamingMessages[event.RunID] = state
	a.mu.Unlock()
	return nil
}

func (a *Adapter) DeliverAssistantFinal(ctx context.Context, event flow.DeliveryEvent) error {
	chatID, ok := ChatIDFromSessionKey(event.SessionKey)
	if !ok || !a.isAllowedChatID(chatID) || strings.TrimSpace(event.Content) == "" {
		return nil
	}

	if err := a.sendFinalMessage(ctx, chatID, event.Content); err != nil {
		return fmt.Errorf("send telegram final message: %w", err)
	}

	a.mu.Lock()
	delete(a.streamingMessages, event.RunID)
	a.mu.Unlock()

	a.log.Info("telegram final response delivered", slog.String("run_id", event.RunID), slog.String("session_key", event.SessionKey))
	return nil
}

// DeliverToolCallEvent sends a short notification about tool call lifecycle
// and shows a chat action indicator so the user knows the bot is working.
func (a *Adapter) DeliverToolCallEvent(ctx context.Context, event flow.ToolCallEvent) error {
	chatID, ok := ChatIDFromSessionKey(event.SessionKey)
	if !ok || !a.isAllowedChatID(chatID) {
		return nil
	}
	switch event.Status {
	case "started":
		// Show "upload_document" action to indicate tool work in progress.
		if err := a.client.SendChatAction(ctx, chatID, ChatActionUploadDocument); err != nil {
			a.log.Warn("send chat action failed", slog.String("action", ChatActionUploadDocument), slog.String("error", err.Error()))
		}
		text := formatToolCallStarted(event.ToolName)
		if _, err := a.client.SendMessage(ctx, chatID, text); err != nil {
			a.log.Warn("send tool call started message failed", slog.String("tool_name", event.ToolName), slog.String("error", err.Error()))
		}
	case "completed", "failed":
		text := formatToolCallFinished(event.ToolName, event.Status, event.DurationMs)
		if _, err := a.client.SendMessage(ctx, chatID, text); err != nil {
			a.log.Warn("send tool call completed message failed", slog.String("tool_name", event.ToolName), slog.String("error", err.Error()))
		}
		// Resume "typing" indicator since the model will continue generating.
		if err := a.client.SendChatAction(ctx, chatID, ChatActionTyping); err != nil {
			a.log.Warn("send chat action failed", slog.String("action", ChatActionTyping), slog.String("error", err.Error()))
		}
	}
	return nil
}

// DeliverStatusEvent shows a chat action and optionally sends a short status
// message so the user gets immediate feedback when the bot starts working.
func (a *Adapter) DeliverStatusEvent(ctx context.Context, event flow.StatusEvent) error {
	chatID, ok := ChatIDFromSessionKey(event.SessionKey)
	if !ok || !a.isAllowedChatID(chatID) {
		return nil
	}
	// Always show "typing" indicator for status events.
	if err := a.client.SendChatAction(ctx, chatID, ChatActionTyping); err != nil {
		a.log.Warn("send chat action failed", slog.String("action", ChatActionTyping), slog.String("error", err.Error()))
	}
	return nil
}

func (a *Adapter) sendFinalMessage(ctx context.Context, chatID int64, text string) error {
	chunks := splitMessage(text, telegramMaxMessageLen)
	for _, chunk := range chunks {
		if err := a.sendSingleMessage(ctx, chatID, chunk); err != nil {
			return err
		}
	}
	return nil
}

// sendSingleMessage sends a single message (must be within Telegram limits)
// with one retry after a 429 rate limit.
func (a *Adapter) sendSingleMessage(ctx context.Context, chatID int64, text string) error {
	_, err := a.client.SendMessage(ctx, chatID, text)
	if err == nil {
		return nil
	}
	var apiErr *apiError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != 429 {
		return err
	}
	wait := apiErr.RetryAfter
	if wait <= 0 {
		wait = time.Second
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
	}
	_, err = a.client.SendMessage(ctx, chatID, text)
	return err
}

func nextAllowedAt(now time.Time, retryAfter time.Duration) time.Time {
	if retryAfter <= 0 {
		retryAfter = time.Second
	}
	return now.Add(retryAfter)
}

// draftIDFromRunID derives a stable non-zero draft ID from the run ID string.
func draftIDFromRunID(runID string) int64 {
	h := fnv.New64a()
	h.Write([]byte(runID))
	id := int64(h.Sum64() >> 1) // shift to keep positive
	if id == 0 {
		id = 1
	}
	return id
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

const (
	approvalCallbackPrefix  = "approve:"
	rejectionCallbackPrefix = "reject:"
	tabSelectCallbackPrefix = "tabsel:"
	tabDenyCallbackPrefix   = "tabdeny:"
	tabCancelCallbackPrefix = "tabcancel:"
)

// DeliverApprovalRequest sends an inline keyboard to the Telegram chat with
// Approve and Reject buttons for the given tool call.
func (a *Adapter) DeliverApprovalRequest(ctx context.Context, req flow.ApprovalRequest) error {
	chatID, ok := ChatIDFromSessionKey(req.SessionKey)
	if !ok || !a.isAllowedChatID(chatID) {
		return nil
	}
	if req.ApprovalType == approvals.ApprovalTypeBrowserTabSelection && len(req.TabCandidates) > 0 {
		text := "Выберите вкладку, к которой подключить агента"
		rows := make([][]InlineKeyboardButton, 0, len(req.TabCandidates))
		for _, candidate := range req.TabCandidates {
			label := candidate.DisplayLabel
			if strings.TrimSpace(label) == "" {
				label = strings.TrimSpace(candidate.Title)
			}
			if strings.TrimSpace(label) == "" {
				label = strings.TrimSpace(candidate.Domain)
			}
			rows = append(rows, []InlineKeyboardButton{{
				Text:         label,
				CallbackData: tabSelectCallbackPrefix + candidate.CandidateToken,
			}})
		}
		rows = append(rows,
			[]InlineKeyboardButton{{Text: "Запретить", CallbackData: tabDenyCallbackPrefix + req.ApprovalID}},
			[]InlineKeyboardButton{{Text: "Отмена", CallbackData: tabCancelCallbackPrefix + req.ApprovalID}},
		)
		markup := &InlineKeyboardMarkup{InlineKeyboard: rows}
		if _, err := a.client.SendMessageWithReplyMarkup(ctx, chatID, text, markup); err != nil {
			return fmt.Errorf("send browser tab selection request: %w", err)
		}
		a.log.Info("browser tab selection request sent", slog.String("approval_id", req.ApprovalID), slog.Int("candidate_count", len(req.TabCandidates)))
		return nil
	}

	text := fmt.Sprintf("Tool call requires approval:\n\nTool: %s\nArgs: %s\n\nApprove or reject?", req.ToolName, req.ArgsJSON)
	markup := &InlineKeyboardMarkup{
		InlineKeyboard: [][]InlineKeyboardButton{
			{
				{Text: "Allow", CallbackData: approvalCallbackPrefix + req.ToolCallID},
				{Text: "Deny", CallbackData: rejectionCallbackPrefix + req.ToolCallID},
			},
		},
	}
	if _, err := a.client.SendMessageWithReplyMarkup(ctx, chatID, text, markup); err != nil {
		return fmt.Errorf("send approval request: %w", err)
	}
	a.log.Info("approval request sent", slog.String("tool_call_id", req.ToolCallID), slog.String("tool_name", req.ToolName))
	return nil
}

// handleCallbackQuery processes inline keyboard button presses for approval decisions.
func (a *Adapter) handleCallbackQuery(ctx context.Context, query *CallbackQuery) {
	if query == nil || query.Data == "" {
		return
	}
	if query.Message != nil {
		chatID := query.Message.Chat.ID
		if !a.isAllowedChatID(chatID) {
			a.log.Warn("callback query ignored for disallowed chat", slog.Int64("chat_id", chatID))
			return
		}
	}

	if strings.HasPrefix(query.Data, authStartCallbackPrefix) {
		provider := strings.TrimPrefix(query.Data, authStartCallbackPrefix)
		a.answerCallbackQuerySafe(ctx, query.ID, "Opening connection flow")
		if err := a.handleAuthCallback(ctx, query, provider); err != nil {
			a.log.Warn("provider auth callback failed", slog.String("provider", provider), slog.String("error", err.Error()))
			if query.Message != nil {
				if _, sendErr := a.client.SendMessage(ctx, query.Message.Chat.ID, "Could not start auth for "+providerLabel(provider)+"\n\nError: "+err.Error()); sendErr != nil {
					a.log.Warn("send provider auth failure failed", slog.String("error", sendErr.Error()))
				}
			}
		}
		return
	}

	if strings.HasPrefix(query.Data, tabSelectCallbackPrefix) {
		a.answerCallbackQuerySafe(ctx, query.ID, "Обрабатываю выбор...")
		if err := a.handleTabSelectionCallback(ctx, query); err != nil {
			a.log.Warn("browser tab selection callback failed", slog.String("error", err.Error()))
			if query.Message != nil {
				if _, sendErr := a.client.SendMessage(ctx, query.Message.Chat.ID, "Не удалось выбрать вкладку.\n\nОшибка: "+err.Error()); sendErr != nil {
					a.log.Warn("send browser tab selection failure failed", slog.String("error", sendErr.Error()))
				}
			}
		}
		return
	}

	if strings.HasPrefix(query.Data, tabDenyCallbackPrefix) || strings.HasPrefix(query.Data, tabCancelCallbackPrefix) {
		a.answerCallbackQuerySafe(ctx, query.ID, "Закрываю запрос...")
		if err := a.handleTabRejectionCallback(ctx, query); err != nil {
			a.log.Warn("browser tab rejection callback failed", slog.String("error", err.Error()))
			if query.Message != nil {
				if _, sendErr := a.client.SendMessage(ctx, query.Message.Chat.ID, "Не удалось закрыть запрос выбора вкладки.\n\nОшибка: "+err.Error()); sendErr != nil {
					a.log.Warn("send browser tab rejection failure failed", slog.String("error", sendErr.Error()))
				}
			}
		}
		return
	}

	var toolCallID string
	var approved bool
	switch {
	case strings.HasPrefix(query.Data, approvalCallbackPrefix):
		toolCallID = strings.TrimPrefix(query.Data, approvalCallbackPrefix)
		approved = true
	case strings.HasPrefix(query.Data, rejectionCallbackPrefix):
		toolCallID = strings.TrimPrefix(query.Data, rejectionCallbackPrefix)
		approved = false
	default:
		a.log.Warn("unknown callback query data", slog.String("data", query.Data))
		return
	}

	a.answerCallbackQuerySafe(ctx, query.ID, "Обрабатываю решение...")

	if a.approvalGate == nil {
		a.log.Error("approval gate not configured, cannot resolve approval")
		return
	}
	resolvedBy := "telegram"
	if query.From != nil {
		resolvedBy = fmt.Sprintf("telegram_user:%d", query.From.ID)
	}
	if !a.approvalGate.ResolveWithChannel(toolCallID, approved, "telegram", resolvedBy) {
		a.log.Warn("no pending approval found for tool call", slog.String("tool_call_id", toolCallID))
	}

	if a.approvalService != nil {
		_, _, err := a.approvalService.ResolveByToolCall(ctx, approvals.ResolveByToolCallParams{
			ToolCallID:       toolCallID,
			Approved:         approved,
			ResolvedVia:      approvals.ResolvedViaTelegram,
			ResolvedBy:       resolvedBy,
			ResolutionReason: telegramResolutionReason(approved),
			ResolvedAt:       time.Now().UTC(),
			ActorType:        "telegram",
			ActorID:          resolvedBy,
		})
		if err != nil {
			a.log.Warn("resolve durable approval via telegram failed", slog.String("tool_call_id", toolCallID), slog.String("error", err.Error()))
		}
	}

	action := "approved"
	if !approved {
		action = "rejected"
	}
	a.log.Info("approval resolved via telegram", slog.String("tool_call_id", toolCallID), slog.String("action", action))
}

func (a *Adapter) answerCallbackQuerySafe(ctx context.Context, callbackQueryID, text string) {
	if a == nil || a.client == nil || strings.TrimSpace(callbackQueryID) == "" {
		return
	}
	if err := a.client.AnswerCallbackQuery(ctx, callbackQueryID, text); err != nil {
		a.log.Warn("answer callback query failed", slog.String("error", err.Error()))
	}
}

func (a *Adapter) handleTabSelectionCallback(ctx context.Context, query *CallbackQuery) error {
	if a.singleTabSelector == nil {
		return fmt.Errorf("single tab selector is not configured")
	}
	payload := strings.TrimPrefix(query.Data, tabSelectCallbackPrefix)
	candidateToken := strings.TrimSpace(payload)
	if candidateToken == "" {
		return fmt.Errorf("invalid browser tab selection callback payload")
	}

	resolvedBy := "telegram"
	if query.From != nil {
		resolvedBy = fmt.Sprintf("telegram_user:%d", query.From.ID)
	}

	result, err := a.singleTabSelector.ActivateFromCandidateToken(ctx, candidateToken, singletab.ActivateFromApprovalParams{
		ResolvedVia:    approvals.ResolvedViaTelegram,
		ResolvedBy:     resolvedBy,
		ActorType:      "telegram",
		ActorID:        resolvedBy,
		ResolvedAt:     time.Now().UTC(),
	})
	if err != nil {
		return err
	}

	if query.Message != nil {
		text := fmt.Sprintf("Агент подключён к вкладке: %s\nURL: %s\nSession: active", firstNonEmpty(result.Session.CurrentTitle, result.Candidate.Title), firstNonEmpty(result.Session.CurrentURL, result.Candidate.CurrentURL))
		if _, sendErr := a.client.SendMessage(ctx, query.Message.Chat.ID, text); sendErr != nil {
			a.log.Warn("send browser tab selection confirmation failed", slog.String("error", sendErr.Error()))
		}
	}
	return nil
}

func (a *Adapter) handleTabRejectionCallback(ctx context.Context, query *CallbackQuery) error {
	if a.approvalService == nil {
		return fmt.Errorf("approval service is not configured")
	}

	data := query.Data
	resolutionReason := "browser tab selection denied in telegram"
	switch {
	case strings.HasPrefix(data, tabDenyCallbackPrefix):
		data = strings.TrimPrefix(data, tabDenyCallbackPrefix)
	case strings.HasPrefix(data, tabCancelCallbackPrefix):
		data = strings.TrimPrefix(data, tabCancelCallbackPrefix)
		resolutionReason = "browser tab selection cancelled in telegram"
	default:
		return fmt.Errorf("invalid browser tab rejection callback payload")
	}
	approvalID := strings.TrimSpace(data)
	if approvalID == "" {
		return fmt.Errorf("approval id is required")
	}

	resolvedBy := "telegram"
	if query.From != nil {
		resolvedBy = fmt.Sprintf("telegram_user:%d", query.From.ID)
	}

	_, _, err := a.approvalService.ResolveByApprovalID(ctx, approvals.ResolveByApprovalIDParams{
		ApprovalID:       approvalID,
		Approved:         false,
		ResolvedVia:      approvals.ResolvedViaTelegram,
		ResolvedBy:       resolvedBy,
		ResolutionReason: resolutionReason,
		ResolvedAt:       time.Now().UTC(),
		ActorType:        "telegram",
		ActorID:          resolvedBy,
	})
	if err != nil {
		return err
	}

	if query.Message != nil {
		text := "Подключение агента к вкладке отменено."
		if _, sendErr := a.client.SendMessage(ctx, query.Message.Chat.ID, text); sendErr != nil {
			a.log.Warn("send browser tab rejection confirmation failed", slog.String("error", sendErr.Error()))
		}
	}
	return nil
}

func telegramResolutionReason(approved bool) string {
	if approved {
		return "approved in telegram"
	}
	return "rejected in telegram"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
