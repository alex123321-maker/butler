package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
)

const authStartCallbackPrefix = "auth:start:"

var providerAuthOrder = []string{
	modelprovider.ProviderOpenAICodex,
	modelprovider.ProviderGitHubCopilot,
}

type ProviderAuthManager interface {
	List(context.Context) ([]providerauth.ProviderState, error)
	Start(context.Context, string, providerauth.StartOptions) (providerauth.PendingFlow, error)
	Complete(context.Context, string, string, string) (providerauth.ProviderState, error)
	State(context.Context, string) (providerauth.ProviderState, error)
}

type pendingAuthPrompt struct {
	Provider string
	FlowID   string
}

func (a *Adapter) SetAuthManager(manager ProviderAuthManager) {
	a.auth = manager
}

func (a *Adapter) handleCommand(ctx context.Context, message *Message) (bool, error) {
	if message == nil {
		return false, nil
	}
	command, ok := telegramCommand(message.Text)
	if !ok {
		return false, nil
	}
	switch command {
	case "auth":
		return true, a.deliverProviderAuthPrompt(ctx, message.Chat.ID, "Let's connect a model provider for this Telegram chat.")
	default:
		return false, nil
	}
}

func telegramCommand(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "/") {
		return "", false
	}
	first := trimmed
	if idx := strings.IndexAny(first, " \t\n"); idx >= 0 {
		first = first[:idx]
	}
	first = strings.TrimPrefix(first, "/")
	if bot, _, ok := strings.Cut(first, "@"); ok {
		first = bot
	}
	first = strings.TrimSpace(strings.ToLower(first))
	if first == "" {
		return "", false
	}
	return first, true
}

func (a *Adapter) deliverProviderAuthPrompt(ctx context.Context, chatID int64, reason string) error {
	if a.auth == nil {
		_, err := a.client.SendMessage(ctx, chatID, "Provider connection is not configured in this Butler deployment.")
		return err
	}
	states, err := a.auth.List(ctx)
	if err != nil {
		return fmt.Errorf("list provider auth states: %w", err)
	}
	text := formatProviderAuthPrompt(states, reason)
	markup := providerAuthKeyboard(states)
	if _, err := a.client.SendMessageWithReplyMarkup(ctx, chatID, text, markup); err != nil {
		return fmt.Errorf("send provider auth prompt: %w", err)
	}
	return nil
}

func providerAuthKeyboard(states []providerauth.ProviderState) *InlineKeyboardMarkup {
	stateByProvider := make(map[string]providerauth.ProviderState, len(states))
	for _, state := range states {
		stateByProvider[state.Provider] = state
	}
	rows := make([][]InlineKeyboardButton, 0, len(providerAuthOrder))
	for _, provider := range providerAuthOrder {
		state := stateByProvider[provider]
		rows = append(rows, []InlineKeyboardButton{{
			Text:         providerButtonText(state),
			CallbackData: authStartCallbackPrefix + provider,
		}})
	}
	return &InlineKeyboardMarkup{InlineKeyboard: rows}
}

func providerButtonText(state providerauth.ProviderState) string {
	label := providerLabel(state.Provider)
	if state.Connected {
		return "Reconnect " + label
	}
	return "Connect " + label
}

func formatProviderAuthPrompt(states []providerauth.ProviderState, reason string) string {
	var lines []string
	if trimmed := strings.TrimSpace(reason); trimmed != "" {
		lines = append(lines, trimmed, "")
	}
	lines = append(lines, "Available providers:")
	stateByProvider := make(map[string]providerauth.ProviderState, len(states))
	for _, state := range states {
		stateByProvider[state.Provider] = state
	}
	for _, provider := range providerAuthOrder {
		lines = append(lines, "- "+providerStateSummary(stateByProvider[provider]))
	}
	lines = append(lines, "", "Tap a button below to start connecting.")
	return strings.Join(lines, "\n")
}

func providerStateSummary(state providerauth.ProviderState) string {
	label := providerLabel(state.Provider)
	status := "not connected yet"
	if state.Connected {
		status = "connected"
		if hint := strings.TrimSpace(state.AccountHint); hint != "" {
			status += " (" + hint + ")"
		}
	}
	if pending := state.Pending; pending != nil {
		status += "; setup " + pending.Status
	}
	return label + ": " + status
}

func providerLabel(provider string) string {
	switch provider {
	case modelprovider.ProviderOpenAICodex:
		return "OpenAI Codex"
	case modelprovider.ProviderGitHubCopilot:
		return "GitHub Copilot"
	default:
		return provider
	}
}

func (a *Adapter) handleAuthCallback(ctx context.Context, query *CallbackQuery, provider string) error {
	if a.auth == nil {
		return fmt.Errorf("provider auth manager is not configured")
	}
	if query == nil || query.Message == nil {
		return ErrIgnoredUpdate
	}
	chatID := query.Message.Chat.ID
	flow, err := a.auth.Start(ctx, provider, providerauth.StartOptions{})
	if err != nil {
		return fmt.Errorf("start provider auth: %w", err)
	}
	switch provider {
	case modelprovider.ProviderOpenAICodex:
		a.setPendingAuthPrompt(chatID, pendingAuthPrompt{Provider: provider, FlowID: flow.ID})
	case modelprovider.ProviderGitHubCopilot:
		a.clearPendingAuthPrompt(chatID)
		go a.watchProviderAuth(chatID, provider, flow.ID, flow.ExpiresAt)
	default:
		a.clearPendingAuthPrompt(chatID)
	}
	if _, err := a.client.SendMessage(ctx, chatID, formatAuthStartMessage(provider, flow)); err != nil {
		return fmt.Errorf("send provider auth start message: %w", err)
	}
	return nil
}

func formatAuthStartMessage(provider string, flow providerauth.PendingFlow) string {
	switch provider {
	case modelprovider.ProviderOpenAICodex:
		lines := []string{
			"OpenAI Codex connection started.",
			"",
			"Open this link in your browser:",
			flow.AuthURL,
			"",
			"Finish the sign-in flow.",
			"Then send me either the full redirect URL or just the code here in chat.",
		}
		return strings.Join(lines, "\n")
	case modelprovider.ProviderGitHubCopilot:
		lines := []string{
			"GitHub Copilot connection started.",
			"",
			"Open this link in your browser:",
			flow.VerificationURI,
			"When asked, enter this code:",
			flow.UserCode,
			"",
			"I'll keep checking in the background and will confirm here when GitHub Copilot is ready.",
		}
		return strings.Join(lines, "\n")
	default:
		return providerLabel(provider) + " connection started."
	}
}

func (a *Adapter) handlePendingAuthInput(ctx context.Context, message *Message) (bool, error) {
	if a.auth == nil || message == nil {
		return false, nil
	}
	prompt, ok := a.pendingAuthPrompt(message.Chat.ID)
	if !ok {
		return false, nil
	}
	state, err := a.auth.Complete(ctx, prompt.Provider, prompt.FlowID, strings.TrimSpace(message.Text))
	if err != nil {
		if errors.Is(err, providerauth.ErrFlowNotFound) {
			a.clearPendingAuthPrompt(message.Chat.ID)
			_, sendErr := a.client.SendMessage(ctx, message.Chat.ID, "That connection flow is no longer active. Send /auth to start again.")
			if sendErr != nil {
				return true, sendErr
			}
			return true, nil
		}
		_, sendErr := a.client.SendMessage(ctx, message.Chat.ID, "I could not complete auth yet. Paste the full redirect URL or code again, or send /auth to restart.\n\nError: "+err.Error())
		if sendErr != nil {
			return true, sendErr
		}
		return true, nil
	}
	a.clearPendingAuthPrompt(message.Chat.ID)
	_, err = a.client.SendMessage(ctx, message.Chat.ID, formatAuthCompletionMessage(state))
	if err != nil {
		return true, err
	}
	return true, nil
}

func formatAuthCompletionMessage(state providerauth.ProviderState) string {
	label := providerLabel(state.Provider)
	message := "Done: " + label + " is now connected."
	if hint := strings.TrimSpace(state.AccountHint); hint != "" {
		message += " Signed in as: " + hint + "."
	}
	return message
}

func (a *Adapter) watchProviderAuth(chatID int64, provider, flowID string, expiresAt time.Time) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	deadline := expiresAt
	if deadline.IsZero() {
		deadline = time.Now().UTC().Add(2 * time.Minute)
	}
	for {
		if time.Now().UTC().After(deadline) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_, _ = a.client.SendMessage(ctx, chatID, providerLabel(provider)+" connection timed out. Send /auth to try again.")
			cancel()
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		state, err := a.auth.State(ctx, provider)
		cancel()
		if err == nil {
			if state.Connected {
				ctx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, _ = a.client.SendMessage(ctx, chatID, formatAuthCompletionMessage(state))
				sendCancel()
				return
			}
			if state.Pending != nil && state.Pending.ID != flowID {
				return
			}
			if state.Pending != nil {
				switch state.Pending.Status {
				case providerauth.FlowStatusFailed, providerauth.FlowStatusExpired, providerauth.FlowStatusCancelled:
					text := providerLabel(provider) + " did not finish connecting. Send /auth to try again."
					if detail := strings.TrimSpace(state.Pending.Error); detail != "" {
						text += "\n\nError: " + detail
					}
					ctx, sendCancel := context.WithTimeout(context.Background(), 5*time.Second)
					_, _ = a.client.SendMessage(ctx, chatID, text)
					sendCancel()
					return
				}
			} else if !state.Connected {
				return
			}
		}
		<-ticker.C
	}
}

func (a *Adapter) pendingAuthPrompt(chatID int64) (pendingAuthPrompt, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	prompt, ok := a.authPrompts[chatID]
	return prompt, ok
}

func (a *Adapter) setPendingAuthPrompt(chatID int64, prompt pendingAuthPrompt) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.authPrompts[chatID] = prompt
}

func (a *Adapter) clearPendingAuthPrompt(chatID int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.authPrompts, chatID)
}

func (a *Adapter) shouldPromptForAuth(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, providerauth.ErrNotConnected) {
		return true
	}
	var transportErr *transport.Error
	if errors.As(err, &transportErr) {
		if strings.Contains(strings.ToLower(transportErr.Message), strings.ToLower(providerauth.ErrNotConnected.Error())) {
			return true
		}
	}
	return strings.Contains(strings.ToLower(err.Error()), strings.ToLower(providerauth.ErrNotConnected.Error()))
}

func formatApprovalMessage(toolName, argsJSON string) string {
	lines := []string{
		"Approval needed before I run a tool",
		"",
		"Tool: " + strings.TrimSpace(toolName),
	}
	if args := formatApprovalArgs(argsJSON); args != "" {
		lines = append(lines, "", "Arguments:", args)
	}
	lines = append(lines, "", "Do you want me to run this tool call?")
	return strings.Join(lines, "\n")
}

func formatApprovalArgs(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "{}"
	}
	var decoded any
	if json.Unmarshal([]byte(trimmed), &decoded) == nil {
		pretty, err := json.MarshalIndent(decoded, "", "  ")
		if err == nil {
			trimmed = string(pretty)
		}
	}
	if len(trimmed) > 800 {
		trimmed = trimmed[:800] + "..."
	}
	return trimmed
}
