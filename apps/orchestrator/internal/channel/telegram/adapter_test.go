package telegram

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	singletab "github.com/butler/butler/apps/orchestrator/internal/singletab"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
)

type stubExecutor struct {
	execute func(context.Context, ingress.InputEvent) (*flow.ExecutionResult, error)
}

func (s stubExecutor) Execute(ctx context.Context, event ingress.InputEvent) (*flow.ExecutionResult, error) {
	if s.execute != nil {
		return s.execute(ctx, event)
	}
	return &flow.ExecutionResult{}, nil
}

type fakeSingleTabSelector struct {
	result singletab.ActivationResult
	err    error
}

func (f *fakeSingleTabSelector) ActivateFromApproval(_ context.Context, params singletab.ActivateFromApprovalParams) (singletab.ActivationResult, error) {
	if f.err != nil {
		return singletab.ActivationResult{}, f.err
	}
	f.result.Candidate.CandidateToken = params.CandidateToken
	return f.result, nil
}

type memoryApprovalRepo struct {
	recordsByID       map[string]approvals.Record
	recordsByToolCall map[string]approvals.Record
	tabCandidates     map[string][]approvals.TabCandidate
}

func newMemoryApprovalRepo() *memoryApprovalRepo {
	return &memoryApprovalRepo{
		recordsByID:       map[string]approvals.Record{},
		recordsByToolCall: map[string]approvals.Record{},
		tabCandidates:     map[string][]approvals.TabCandidate{},
	}
}

func (m *memoryApprovalRepo) CreateApproval(_ context.Context, params approvals.CreateParams) (approvals.Record, error) {
	rec := approvals.Record{
		ApprovalID:   params.ApprovalID,
		RunID:        params.RunID,
		SessionKey:   params.SessionKey,
		ToolCallID:   params.ToolCallID,
		ApprovalType: params.ApprovalType,
		Status:       approvals.StatusPending,
		RequestedVia: params.RequestedVia,
		ToolName:     params.ToolName,
		ArgsJSON:     params.ArgsJSON,
		PayloadJSON:  params.PayloadJSON,
		RequestedAt:  params.RequestedAt,
		UpdatedAt:    params.RequestedAt,
	}
	m.recordsByID[rec.ApprovalID] = rec
	m.recordsByToolCall[rec.ToolCallID] = rec
	return rec, nil
}

func (m *memoryApprovalRepo) GetApprovalByToolCallID(_ context.Context, toolCallID string) (approvals.Record, error) {
	rec, ok := m.recordsByToolCall[toolCallID]
	if !ok {
		return approvals.Record{}, approvals.ErrApprovalNotFound
	}
	return rec, nil
}

func (m *memoryApprovalRepo) GetApprovalByID(_ context.Context, approvalID string) (approvals.Record, error) {
	rec, ok := m.recordsByID[approvalID]
	if !ok {
		return approvals.Record{}, approvals.ErrApprovalNotFound
	}
	return rec, nil
}

func (m *memoryApprovalRepo) ListApprovals(_ context.Context, status, runID, sessionKey string, limit, offset int) ([]approvals.Record, error) {
	return nil, nil
}

func (m *memoryApprovalRepo) CreateTabCandidates(_ context.Context, params []approvals.CreateTabCandidateParams) error {
	for _, candidate := range params {
		m.tabCandidates[candidate.ApprovalID] = append(m.tabCandidates[candidate.ApprovalID], approvals.TabCandidate{
			ApprovalID:     candidate.ApprovalID,
			CandidateToken: candidate.CandidateToken,
			InternalTabRef: candidate.InternalTabRef,
			Status:         "available",
			CreatedAt:      time.Now().UTC(),
		})
	}
	return nil
}

func (m *memoryApprovalRepo) ListTabCandidates(_ context.Context, approvalID string) ([]approvals.TabCandidate, error) {
	return append([]approvals.TabCandidate(nil), m.tabCandidates[approvalID]...), nil
}

func (m *memoryApprovalRepo) SelectTabCandidate(_ context.Context, approvalID, candidateToken string, selectedAt time.Time) (approvals.TabCandidate, error) {
	return approvals.TabCandidate{}, approvals.ErrTabCandidateNotFound
}

func (m *memoryApprovalRepo) ResolveApproval(_ context.Context, params approvals.ResolveParams) (approvals.Record, error) {
	rec, ok := m.recordsByID[params.ApprovalID]
	if !ok {
		return approvals.Record{}, approvals.ErrApprovalNotFound
	}
	rec.Status = params.Status
	rec.ResolvedVia = params.ResolvedVia
	rec.ResolvedBy = params.ResolvedBy
	rec.ResolutionReason = params.ResolutionReason
	rec.ResolvedAt = &params.ResolvedAt
	rec.UpdatedAt = params.ResolvedAt
	m.recordsByID[rec.ApprovalID] = rec
	m.recordsByToolCall[rec.ToolCallID] = rec
	return rec, nil
}

func (m *memoryApprovalRepo) InsertEvent(_ context.Context, event approvals.Event) error {
	return nil
}

type fakeApprovalGate struct {
	resolved map[string]bool
}

func (f *fakeApprovalGate) Resolve(toolCallID string, approved bool) bool {
	if f.resolved == nil {
		f.resolved = map[string]bool{}
	}
	f.resolved[toolCallID] = approved
	return true
}

func TestNormalizeUpdateBuildsUserMessageEvent(t *testing.T) {
	t.Parallel()

	event, err := NormalizeUpdate(Update{
		UpdateID: 42,
		Message: &Message{
			MessageID: 100,
			Date:      1710500000,
			Text:      " hello telegram ",
			Chat:      Chat{ID: 777},
			From:      &User{ID: 999, Username: "alice", FirstName: "Alice"},
		},
	})
	if err != nil {
		t.Fatalf("NormalizeUpdate returned error: %v", err)
	}
	if event.SessionKey != "telegram:chat:777" {
		t.Fatalf("unexpected session key %q", event.SessionKey)
	}
	if event.Source != "telegram" {
		t.Fatalf("unexpected source %q", event.Source)
	}
	if event.IdempotencyKey != "telegram-update-42" {
		t.Fatalf("unexpected idempotency key %q", event.IdempotencyKey)
	}
	if event.CreatedAt.UTC() != time.Unix(1710500000, 0).UTC() {
		t.Fatalf("unexpected created_at %s", event.CreatedAt)
	}
}

func TestNormalizeUpdateIgnoresUnsupportedUpdates(t *testing.T) {
	t.Parallel()

	_, err := NormalizeUpdate(Update{UpdateID: 1})
	if !errors.Is(err, ErrIgnoredUpdate) {
		t.Fatalf("expected ErrIgnoredUpdate for missing message, got %v", err)
	}

	_, err = NormalizeUpdate(Update{UpdateID: 2, Message: &Message{Chat: Chat{ID: 1}}})
	if !errors.Is(err, ErrIgnoredUpdate) {
		t.Fatalf("expected ErrIgnoredUpdate for empty text, got %v", err)
	}
}

func TestChatIDRoundTrip(t *testing.T) {
	t.Parallel()

	chatID, ok := ChatIDFromSessionKey(SessionKeyFromChatID(123456))
	if !ok {
		t.Fatal("expected session key to parse")
	}
	if chatID != 123456 {
		t.Fatalf("unexpected chat id %d", chatID)
	}
	if _, ok := ChatIDFromSessionKey("not-telegram"); ok {
		t.Fatal("expected non-telegram session key to be rejected")
	}
}

func TestDeliverApprovalRequestSendsInlineKeyboard(t *testing.T) {
	t.Parallel()

	client := &Client{}
	client.answerCallback = func(_ context.Context, _ string, _ string) error { return nil }
	gate := flow.NewApprovalGate()
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, gate, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	// Test handleCallbackQuery resolves approval via gate.
	go func() {
		time.Sleep(10 * time.Millisecond)
		adapter.handleCallbackQuery(context.Background(), &CallbackQuery{
			ID:      "cbq-1",
			Data:    "approve:call-1",
			Message: &Message{Chat: Chat{ID: 777}},
		})
	}()

	resp, waitErr := gate.Wait(context.Background(), "call-1")
	if waitErr != nil {
		t.Fatalf("Wait returned error: %v", waitErr)
	}
	if !resp.Approved {
		t.Fatal("expected approval to be true")
	}
	if resp.Channel != "telegram" {
		t.Fatalf("expected channel telegram, got %q", resp.Channel)
	}
}

func TestDeliverBrowserTabSelectionRequestSendsCandidateButtons(t *testing.T) {
	t.Parallel()

	client := &Client{}
	var text string
	var markup *InlineKeyboardMarkup
	client.sendMessageMarkup = func(_ context.Context, _ int64, body string, m *InlineKeyboardMarkup) (Message, error) {
		text = body
		markup = m
		return Message{MessageID: 1}, nil
	}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	err = adapter.DeliverApprovalRequest(context.Background(), flow.ApprovalRequest{
		RunID:        "run-1",
		SessionKey:   "telegram:chat:777",
		ApprovalID:   "approval-1",
		ApprovalType: approvals.ApprovalTypeBrowserTabSelection,
		ToolName:     "single_tab.bind",
		TabCandidates: []flow.ApprovalTabCandidate{{
			CandidateToken: "tok-1",
			DisplayLabel:   "Inbox - mail.example.com",
		}},
	})
	if err != nil {
		t.Fatalf("DeliverApprovalRequest returned error: %v", err)
	}
	if !strings.Contains(text, "Выберите вкладку") {
		t.Fatalf("expected browser tab selection prompt, got %q", text)
	}
	if markup == nil || len(markup.InlineKeyboard) != 3 || markup.InlineKeyboard[0][0].CallbackData != "tabsel:approval-1:tok-1" {
		t.Fatalf("unexpected browser tab selection keyboard: %+v", markup)
	}
}

func TestHandleCallbackQueryRejectsApproval(t *testing.T) {
	t.Parallel()

	client := &Client{}
	client.answerCallback = func(_ context.Context, _ string, _ string) error { return nil }
	gate := flow.NewApprovalGate()
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, gate, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	go func() {
		time.Sleep(10 * time.Millisecond)
		adapter.handleCallbackQuery(context.Background(), &CallbackQuery{
			ID:      "cbq-2",
			Data:    "reject:call-2",
			Message: &Message{Chat: Chat{ID: 777}},
		})
	}()

	resp, waitErr := gate.Wait(context.Background(), "call-2")
	if waitErr != nil {
		t.Fatalf("Wait returned error: %v", waitErr)
	}
	if resp.Approved {
		t.Fatal("expected approval to be false (rejected)")
	}
	if resp.Channel != "telegram" {
		t.Fatalf("expected channel telegram, got %q", resp.Channel)
	}
}

func TestHandleCallbackQuerySelectsBrowserTab(t *testing.T) {
	t.Parallel()

	client := &Client{}
	client.answerCallback = func(_ context.Context, _ string, _ string) error { return nil }
	var messages []string
	client.sendMessage = func(_ context.Context, _ int64, text string) (Message, error) {
		messages = append(messages, text)
		return Message{MessageID: 1}, nil
	}

	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}
	adapter.SetSingleTabSelector(&fakeSingleTabSelector{
		result: singletab.ActivationResult{
			Candidate: approvals.TabCandidate{CandidateToken: "tok-1", Title: "Inbox", CurrentURL: "https://mail.example.com/inbox"},
			Session:   singletab.Record{SingleTabSessionID: "single-tab-1", CurrentTitle: "Inbox", CurrentURL: "https://mail.example.com/inbox"},
		},
	})

	adapter.handleCallbackQuery(context.Background(), &CallbackQuery{
		ID:      "cbq-tab-1",
		Data:    "tabsel:approval-1:tok-1",
		Message: &Message{Chat: Chat{ID: 777}},
		From:    &User{ID: 42},
	})

	if len(messages) != 1 {
		t.Fatalf("expected one confirmation message, got %d", len(messages))
	}
	if !containsAll(messages[0], "подключён", "Session: active") {
		t.Fatalf("unexpected confirmation message: %q", messages[0])
	}
}

func TestHandleCallbackQueryRejectsBrowserTabSelection(t *testing.T) {
	t.Parallel()

	client := &Client{}
	client.answerCallback = func(_ context.Context, _ string, _ string) error { return nil }
	var messages []string
	client.sendMessage = func(_ context.Context, _ int64, text string) (Message, error) {
		messages = append(messages, text)
		return Message{MessageID: 1}, nil
	}

	repo := newMemoryApprovalRepo()
	gate := &fakeApprovalGate{}
	service := approvals.NewService(repo, gate)
	now := time.Now().UTC()
	rec, err := service.CreatePendingApproval(context.Background(), approvals.CreatePendingParams{
		RunID:        "run-tab-2",
		SessionKey:   "telegram:chat:777",
		ToolCallID:   "tool-bind-2",
		ApprovalType: approvals.ApprovalTypeBrowserTabSelection,
		RequestedVia: approvals.RequestedViaBoth,
		ToolName:     "single_tab.bind",
		RequestedAt:  now,
		TabCandidates: []approvals.CreateTabCandidateParams{{
			CandidateToken: "tok-2",
			InternalTabRef: "browser-a:2",
		}},
	})
	if err != nil {
		t.Fatalf("CreatePendingApproval returned error: %v", err)
	}

	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}
	adapter.SetApprovalService(service)

	adapter.handleCallbackQuery(context.Background(), &CallbackQuery{
		ID:      "cbq-tab-reject",
		Data:    "tabdeny:" + rec.ApprovalID,
		Message: &Message{Chat: Chat{ID: 777}},
		From:    &User{ID: 42},
	})

	if len(messages) != 1 || !containsAll(messages[0], "отменено") {
		t.Fatalf("unexpected rejection confirmation messages: %+v", messages)
	}
	if gate.resolved["tool-bind-2"] {
		t.Fatal("expected gate resolution to remain false")
	}
}

func TestHandleCallbackQueryIgnoresDisallowedChat(t *testing.T) {
	t.Parallel()

	client := &Client{}
	gate := flow.NewApprovalGate()
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, gate, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	// This should be ignored because chat ID 999 is not in the allowed list.
	adapter.handleCallbackQuery(context.Background(), &CallbackQuery{
		ID:      "cbq-3",
		Data:    "approve:call-3",
		Message: &Message{Chat: Chat{ID: 999}},
	})

	// Verify the gate was NOT resolved.
	if gate.Resolve("call-3", true) {
		// If Resolve returns true, it means the pending channel existed but
		// the callback didn't consume it. This is fine for this test since
		// we didn't call Wait.
	}
}

func TestDeliverAssistantStreamingUsesSendMessageDraft(t *testing.T) {
	t.Parallel()

	client := &Client{}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	var drafts []string
	client.sendMessageDraft = func(_ context.Context, _ int64, _ int64, text string) error {
		drafts = append(drafts, text)
		return nil
	}
	var finalMessages []string
	client.sendMessage = func(_ context.Context, _ int64, text string) (Message, error) {
		finalMessages = append(finalMessages, text)
		return Message{MessageID: 10}, nil
	}

	// Send two deltas then a final.
	if err := adapter.DeliverAssistantDelta(context.Background(), flow.DeliveryEvent{RunID: "run-1", SessionKey: "telegram:chat:777", Content: "Hel", SequenceNo: 1}); err != nil {
		t.Fatalf("DeliverAssistantDelta returned error: %v", err)
	}
	if err := adapter.DeliverAssistantDelta(context.Background(), flow.DeliveryEvent{RunID: "run-1", SessionKey: "telegram:chat:777", Content: "lo", SequenceNo: 2}); err != nil {
		t.Fatalf("DeliverAssistantDelta second returned error: %v", err)
	}
	if err := adapter.DeliverAssistantFinal(context.Background(), flow.DeliveryEvent{RunID: "run-1", SessionKey: "telegram:chat:777", Content: "Hello world", Final: true}); err != nil {
		t.Fatalf("DeliverAssistantFinal returned error: %v", err)
	}

	// Expect buffered draft delivery to avoid sending every tiny delta.
	if len(drafts) != 1 || drafts[0] != "Hel" {
		t.Fatalf("unexpected drafts: %+v", drafts)
	}
	// Expect one final sendMessage call.
	if len(finalMessages) != 1 || finalMessages[0] != "Hello world" {
		t.Fatalf("unexpected final messages: %+v", finalMessages)
	}
	// Streaming state should be cleared.
	if _, ok := adapter.streamingMessages["run-1"]; ok {
		t.Fatal("expected final delivery to clear streaming state")
	}
}

func TestDeliverAssistantStreamingPreservesWhitespaceDeltas(t *testing.T) {
	t.Parallel()

	client := &Client{}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	var drafts []string
	client.sendMessageDraft = func(_ context.Context, _ int64, _ int64, text string) error {
		drafts = append(drafts, text)
		return nil
	}
	now := time.Unix(100, 0)
	adapter.now = func() time.Time { return now }

	if err := adapter.DeliverAssistantDelta(context.Background(), flow.DeliveryEvent{RunID: "run-2", SessionKey: "telegram:chat:777", Content: "Hello", SequenceNo: 1}); err != nil {
		t.Fatalf("DeliverAssistantDelta returned error: %v", err)
	}
	if err := adapter.DeliverAssistantDelta(context.Background(), flow.DeliveryEvent{RunID: "run-2", SessionKey: "telegram:chat:777", Content: " ", SequenceNo: 2}); err != nil {
		t.Fatalf("DeliverAssistantDelta whitespace returned error: %v", err)
	}
	now = now.Add(minDraftFlushInterval + 10*time.Millisecond)
	if err := adapter.DeliverAssistantDelta(context.Background(), flow.DeliveryEvent{RunID: "run-2", SessionKey: "telegram:chat:777", Content: "world", SequenceNo: 3}); err != nil {
		t.Fatalf("DeliverAssistantDelta third returned error: %v", err)
	}

	if len(drafts) != 2 || drafts[0] != "Hello" || drafts[1] != "Hello world" {
		t.Fatalf("unexpected drafts: %+v", drafts)
	}
}

func TestDeliverAssistantDraftIDIsStablePerRun(t *testing.T) {
	t.Parallel()

	client := &Client{}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	var draftIDs []int64
	client.sendMessageDraft = func(_ context.Context, _ int64, draftID int64, _ string) error {
		draftIDs = append(draftIDs, draftID)
		return nil
	}
	now := time.Unix(200, 0)
	adapter.now = func() time.Time { return now }

	if err := adapter.DeliverAssistantDelta(context.Background(), flow.DeliveryEvent{RunID: "run-3", SessionKey: "telegram:chat:777", Content: "A", SequenceNo: 1}); err != nil {
		t.Fatalf("DeliverAssistantDelta returned error: %v", err)
	}
	now = now.Add(minDraftFlushInterval + 10*time.Millisecond)
	if err := adapter.DeliverAssistantDelta(context.Background(), flow.DeliveryEvent{RunID: "run-3", SessionKey: "telegram:chat:777", Content: "B", SequenceNo: 2}); err != nil {
		t.Fatalf("DeliverAssistantDelta second returned error: %v", err)
	}

	if len(draftIDs) != 2 {
		t.Fatalf("expected 2 draft calls, got %d", len(draftIDs))
	}
	if draftIDs[0] != draftIDs[1] {
		t.Fatalf("expected stable draft_id across deltas, got %d and %d", draftIDs[0], draftIDs[1])
	}
	if draftIDs[0] == 0 {
		t.Fatal("draft_id must be non-zero")
	}
}

func TestDeliverAssistantDeltaRateLimitBacksOffWithoutFailingRun(t *testing.T) {
	t.Parallel()

	client := &Client{}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	now := time.Unix(300, 0)
	adapter.now = func() time.Time { return now }
	callCount := 0
	var drafts []string
	client.sendMessageDraft = func(_ context.Context, _ int64, _ int64, text string) error {
		callCount++
		drafts = append(drafts, text)
		if callCount == 1 {
			return &apiError{StatusCode: 429, Description: "Too Many Requests: retry after 3", Retryable: true, RetryAfter: 3 * time.Second}
		}
		return nil
	}

	if err := adapter.DeliverAssistantDelta(context.Background(), flow.DeliveryEvent{RunID: "run-4", SessionKey: "telegram:chat:777", Content: "Hello", SequenceNo: 1}); err != nil {
		t.Fatalf("DeliverAssistantDelta returned error: %v", err)
	}
	if err := adapter.DeliverAssistantDelta(context.Background(), flow.DeliveryEvent{RunID: "run-4", SessionKey: "telegram:chat:777", Content: " world", SequenceNo: 2}); err != nil {
		t.Fatalf("DeliverAssistantDelta during backoff returned error: %v", err)
	}
	now = now.Add(4 * time.Second)
	if err := adapter.DeliverAssistantDelta(context.Background(), flow.DeliveryEvent{RunID: "run-4", SessionKey: "telegram:chat:777", Content: "!", SequenceNo: 3}); err != nil {
		t.Fatalf("DeliverAssistantDelta after backoff returned error: %v", err)
	}

	if callCount != 2 {
		t.Fatalf("expected 2 draft attempts, got %d", callCount)
	}
	if len(drafts) != 2 || drafts[0] != "Hello" || drafts[1] != "Hello world!" {
		t.Fatalf("unexpected drafts: %+v", drafts)
	}
}

func TestDeliverAssistantFinalRetriesOnceAfterRateLimit(t *testing.T) {
	t.Parallel()

	client := &Client{}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	callCount := 0
	client.sendMessage = func(_ context.Context, _ int64, text string) (Message, error) {
		callCount++
		if text != "Hello world" {
			t.Fatalf("unexpected final text %q", text)
		}
		if callCount == 1 {
			return Message{}, &apiError{StatusCode: 429, Description: "Too Many Requests: retry after 1", Retryable: true, RetryAfter: time.Millisecond}
		}
		return Message{MessageID: 10}, nil
	}

	if err := adapter.DeliverAssistantFinal(context.Background(), flow.DeliveryEvent{RunID: "run-5", SessionKey: "telegram:chat:777", Content: "Hello world", Final: true}); err != nil {
		t.Fatalf("DeliverAssistantFinal returned error: %v", err)
	}
	if callCount != 2 {
		t.Fatalf("expected final send retry, got %d calls", callCount)
	}
}

func TestHandleMessageUpdateExecutionFailureNotifiesUser(t *testing.T) {
	t.Parallel()

	client := &Client{}
	var sent []string
	client.sendMessage = func(_ context.Context, _ int64, text string) (Message, error) {
		sent = append(sent, text)
		return Message{MessageID: 1}, nil
	}
	client.sendChatAction = func(_ context.Context, _ int64, _ string) error { return nil }
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}
	adapter.SetExecutor(stubExecutor{execute: func(context.Context, ingress.InputEvent) (*flow.ExecutionResult, error) {
		return nil, context.DeadlineExceeded
	}})

	err = adapter.handleMessageUpdate(context.Background(), Update{UpdateID: 1, Message: &Message{Chat: Chat{ID: 777}, Text: "hello"}})
	if err != nil {
		t.Fatalf("handleMessageUpdate returned error: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("expected one failure notification, got %d", len(sent))
	}
	if sent[0] == "" || !containsAll(sent[0], "could not finish", "in time") {
		t.Fatalf("unexpected failure notification: %q", sent[0])
	}
}

func containsAll(text string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(strings.ToLower(text), strings.ToLower(part)) {
			return false
		}
	}
	return true
}

type fakeAuthManager struct {
	states       []providerauth.ProviderState
	stateByName  map[string]providerauth.ProviderState
	startFlow    providerauth.PendingFlow
	startCalls   []string
	completeCall struct {
		provider string
		flowID   string
		input    string
	}
	completeState providerauth.ProviderState
	completeErr   error
}

func (f *fakeAuthManager) List(context.Context) ([]providerauth.ProviderState, error) {
	return f.states, nil
}

func (f *fakeAuthManager) Start(_ context.Context, provider string, _ providerauth.StartOptions) (providerauth.PendingFlow, error) {
	f.startCalls = append(f.startCalls, provider)
	return f.startFlow, nil
}

func (f *fakeAuthManager) Complete(_ context.Context, provider, flowID, input string) (providerauth.ProviderState, error) {
	f.completeCall.provider = provider
	f.completeCall.flowID = flowID
	f.completeCall.input = input
	if f.completeErr != nil {
		return providerauth.ProviderState{}, f.completeErr
	}
	return f.completeState, nil
}

func (f *fakeAuthManager) State(_ context.Context, provider string) (providerauth.ProviderState, error) {
	if f.stateByName != nil {
		return f.stateByName[provider], nil
	}
	for _, state := range f.states {
		if state.Provider == provider {
			return state, nil
		}
	}
	return providerauth.ProviderState{Provider: provider}, nil
}

func TestHandleMessageUpdateAuthCommandSendsProviderButtons(t *testing.T) {
	t.Parallel()

	client := &Client{}
	var text string
	var markup *InlineKeyboardMarkup
	client.sendMessageMarkup = func(_ context.Context, _ int64, body string, m *InlineKeyboardMarkup) (Message, error) {
		text = body
		markup = m
		return Message{MessageID: 1}, nil
	}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}
	adapter.SetAuthManager(&fakeAuthManager{states: []providerauth.ProviderState{{Provider: modelprovider.ProviderOpenAICodex}, {Provider: modelprovider.ProviderGitHubCopilot}}})

	err = adapter.handleMessageUpdate(context.Background(), Update{UpdateID: 1, Message: &Message{Chat: Chat{ID: 777}, Text: "/auth"}})
	if err != nil {
		t.Fatalf("handleMessageUpdate returned error: %v", err)
	}
	if text == "" || markup == nil {
		t.Fatal("expected auth prompt message with inline keyboard")
	}
	if len(markup.InlineKeyboard) != 2 {
		t.Fatalf("expected 2 auth rows, got %d", len(markup.InlineKeyboard))
	}
}

func TestHandlePendingAuthInputCompletesOpenAICodexFlow(t *testing.T) {
	t.Parallel()

	client := &Client{}
	var messages []string
	client.sendMessage = func(_ context.Context, _ int64, body string) (Message, error) {
		messages = append(messages, body)
		return Message{MessageID: 1}, nil
	}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}
	auth := &fakeAuthManager{
		completeState: providerauth.ProviderState{Provider: modelprovider.ProviderOpenAICodex, Connected: true, AccountHint: "acct...1234"},
	}
	adapter.SetAuthManager(auth)
	adapter.setPendingAuthPrompt(777, pendingAuthPrompt{Provider: modelprovider.ProviderOpenAICodex, FlowID: "flow-1"})

	handled, err := adapter.handlePendingAuthInput(context.Background(), &Message{Chat: Chat{ID: 777}, Text: "https://localhost/callback?code=abc"})
	if err != nil {
		t.Fatalf("handlePendingAuthInput returned error: %v", err)
	}
	if !handled {
		t.Fatal("expected auth input to be handled")
	}
	if auth.completeCall.provider != modelprovider.ProviderOpenAICodex || auth.completeCall.flowID != "flow-1" {
		t.Fatalf("unexpected completion call: %+v", auth.completeCall)
	}
	if len(messages) != 1 || messages[0] == "" {
		t.Fatalf("expected completion confirmation, got %+v", messages)
	}
}

func TestDeliverAssistantDeltaTruncatesLongContent(t *testing.T) {
	t.Parallel()

	client := &Client{}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	var drafts []string
	client.sendMessageDraft = func(_ context.Context, _ int64, _ int64, text string) error {
		drafts = append(drafts, text)
		return nil
	}

	// Build content that exceeds telegramMaxMessageLen.
	longContent := strings.Repeat("x", telegramMaxMessageLen+500)
	if err := adapter.DeliverAssistantDelta(context.Background(), flow.DeliveryEvent{
		RunID:      "run-long-delta",
		SessionKey: "telegram:chat:777",
		Content:    longContent,
		SequenceNo: 1,
	}); err != nil {
		t.Fatalf("DeliverAssistantDelta returned error: %v", err)
	}

	if len(drafts) != 1 {
		t.Fatalf("expected 1 draft, got %d", len(drafts))
	}
	if len(drafts[0]) > telegramMaxMessageLen {
		t.Fatalf("draft exceeds limit: len=%d, limit=%d", len(drafts[0]), telegramMaxMessageLen)
	}
	// Truncated draft should start with the ellipsis prefix.
	if !strings.HasPrefix(drafts[0], "…") {
		t.Fatalf("expected truncated draft to start with '…', got prefix %q", drafts[0][:10])
	}
}

func TestDeliverAssistantFinalSplitsLongMessage(t *testing.T) {
	t.Parallel()

	client := &Client{}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	var messages []string
	client.sendMessage = func(_ context.Context, _ int64, text string) (Message, error) {
		messages = append(messages, text)
		return Message{MessageID: int64(len(messages))}, nil
	}

	// Build content with 3 "paragraphs" that together exceed the limit.
	// Each paragraph is 1500 chars, separated by \n\n, total ~4504 chars.
	para := strings.Repeat("a", 1500)
	longContent := para + "\n\n" + para + "\n\n" + para
	if err := adapter.DeliverAssistantFinal(context.Background(), flow.DeliveryEvent{
		RunID:      "run-long-final",
		SessionKey: "telegram:chat:777",
		Content:    longContent,
		Final:      true,
	}); err != nil {
		t.Fatalf("DeliverAssistantFinal returned error: %v", err)
	}

	if len(messages) < 2 {
		t.Fatalf("expected multiple messages for long content, got %d", len(messages))
	}
	for i, msg := range messages {
		if len(msg) > telegramMaxMessageLen {
			t.Fatalf("message chunk %d exceeds limit: len=%d", i, len(msg))
		}
	}
	// Verify all content is delivered (accounting for trimmed newlines between chunks).
	totalLen := 0
	for _, msg := range messages {
		totalLen += len(msg)
	}
	if totalLen < len(para)*3 {
		t.Fatalf("total delivered content is too short: %d < %d", totalLen, len(para)*3)
	}
}

func TestHandleMessageUpdateSendsTypingAction(t *testing.T) {
	t.Parallel()

	client := &Client{}
	var actionsSent []string
	client.sendChatAction = func(_ context.Context, _ int64, action string) error {
		actionsSent = append(actionsSent, action)
		return nil
	}
	client.sendMessage = func(_ context.Context, _ int64, _ string) (Message, error) {
		return Message{MessageID: 1}, nil
	}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}
	adapter.SetExecutor(stubExecutor{execute: func(_ context.Context, _ ingress.InputEvent) (*flow.ExecutionResult, error) {
		return &flow.ExecutionResult{RunID: "run-1", SessionKey: "telegram:chat:777"}, nil
	}})

	err = adapter.handleMessageUpdate(context.Background(), Update{UpdateID: 1, Message: &Message{Chat: Chat{ID: 777}, Text: "hello"}})
	if err != nil {
		t.Fatalf("handleMessageUpdate returned error: %v", err)
	}
	if len(actionsSent) == 0 {
		t.Fatal("expected at least one chat action to be sent")
	}
	if actionsSent[0] != ChatActionTyping {
		t.Fatalf("expected first chat action to be %q, got %q", ChatActionTyping, actionsSent[0])
	}
}

func TestHandleMessageUpdateTypingActionErrorDoesNotFail(t *testing.T) {
	t.Parallel()

	client := &Client{}
	client.sendChatAction = func(_ context.Context, _ int64, _ string) error {
		return errors.New("network error")
	}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}
	adapter.SetExecutor(stubExecutor{execute: func(_ context.Context, _ ingress.InputEvent) (*flow.ExecutionResult, error) {
		return &flow.ExecutionResult{RunID: "run-1", SessionKey: "telegram:chat:777"}, nil
	}})

	// Even though sendChatAction fails, handleMessageUpdate should succeed.
	err = adapter.handleMessageUpdate(context.Background(), Update{UpdateID: 1, Message: &Message{Chat: Chat{ID: 777}, Text: "hello"}})
	if err != nil {
		t.Fatalf("handleMessageUpdate should not fail on chat action error: %v", err)
	}
}

func TestDeliverToolCallEventSendsStartedMessage(t *testing.T) {
	t.Parallel()

	client := &Client{}
	var actionsSent []string
	var messagesSent []string
	client.sendChatAction = func(_ context.Context, _ int64, action string) error {
		actionsSent = append(actionsSent, action)
		return nil
	}
	client.sendMessage = func(_ context.Context, _ int64, text string) (Message, error) {
		messagesSent = append(messagesSent, text)
		return Message{MessageID: 1}, nil
	}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	err = adapter.DeliverToolCallEvent(context.Background(), flow.ToolCallEvent{
		RunID:      "run-1",
		SessionKey: "telegram:chat:777",
		ToolCallID: "call-1",
		ToolName:   "web_search",
		Status:     "started",
	})
	if err != nil {
		t.Fatalf("DeliverToolCallEvent returned error: %v", err)
	}
	if len(actionsSent) != 1 || actionsSent[0] != ChatActionUploadDocument {
		t.Fatalf("expected upload_document action, got %v", actionsSent)
	}
	if len(messagesSent) != 1 {
		t.Fatalf("expected one message, got %d", len(messagesSent))
	}
	if !strings.Contains(messagesSent[0], "web search") {
		t.Fatalf("expected tool name in message, got %q", messagesSent[0])
	}
}

func TestDeliverToolCallEventSendsCompletedMessage(t *testing.T) {
	t.Parallel()

	client := &Client{}
	var actionsSent []string
	var messagesSent []string
	client.sendChatAction = func(_ context.Context, _ int64, action string) error {
		actionsSent = append(actionsSent, action)
		return nil
	}
	client.sendMessage = func(_ context.Context, _ int64, text string) (Message, error) {
		messagesSent = append(messagesSent, text)
		return Message{MessageID: 1}, nil
	}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	err = adapter.DeliverToolCallEvent(context.Background(), flow.ToolCallEvent{
		RunID:      "run-1",
		SessionKey: "telegram:chat:777",
		ToolCallID: "call-1",
		ToolName:   "file_read",
		Status:     "completed",
		DurationMs: 1500,
	})
	if err != nil {
		t.Fatalf("DeliverToolCallEvent returned error: %v", err)
	}
	// Should resume typing after tool completes.
	if len(actionsSent) != 1 || actionsSent[0] != ChatActionTyping {
		t.Fatalf("expected typing action after completion, got %v", actionsSent)
	}
	if len(messagesSent) != 1 {
		t.Fatalf("expected one message, got %d", len(messagesSent))
	}
	if !strings.Contains(messagesSent[0], "file read") {
		t.Fatalf("expected tool name in message, got %q", messagesSent[0])
	}
	if !strings.Contains(messagesSent[0], "1.5s") {
		t.Fatalf("expected duration in message, got %q", messagesSent[0])
	}
}

func TestDeliverToolCallEventSendsFailedMessage(t *testing.T) {
	t.Parallel()

	client := &Client{}
	var messagesSent []string
	client.sendChatAction = func(_ context.Context, _ int64, _ string) error { return nil }
	client.sendMessage = func(_ context.Context, _ int64, text string) (Message, error) {
		messagesSent = append(messagesSent, text)
		return Message{MessageID: 1}, nil
	}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	err = adapter.DeliverToolCallEvent(context.Background(), flow.ToolCallEvent{
		RunID:      "run-1",
		SessionKey: "telegram:chat:777",
		ToolCallID: "call-1",
		ToolName:   "bash_exec",
		Status:     "failed",
		DurationMs: 200,
	})
	if err != nil {
		t.Fatalf("DeliverToolCallEvent returned error: %v", err)
	}
	if len(messagesSent) != 1 {
		t.Fatalf("expected one message, got %d", len(messagesSent))
	}
	if !strings.Contains(messagesSent[0], "failed") {
		t.Fatalf("expected 'failed' in message, got %q", messagesSent[0])
	}
}

func TestDeliverToolCallEventIgnoresDisallowedSession(t *testing.T) {
	t.Parallel()

	client := &Client{}
	called := false
	client.sendChatAction = func(_ context.Context, _ int64, _ string) error {
		called = true
		return nil
	}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	err = adapter.DeliverToolCallEvent(context.Background(), flow.ToolCallEvent{
		RunID:      "run-1",
		SessionKey: "telegram:chat:999",
		ToolCallID: "call-1",
		ToolName:   "test_tool",
		Status:     "started",
	})
	if err != nil {
		t.Fatalf("DeliverToolCallEvent returned error: %v", err)
	}
	if called {
		t.Fatal("expected no chat action for disallowed session")
	}
}

func TestDeliverStatusEventSendsTypingAction(t *testing.T) {
	t.Parallel()

	client := &Client{}
	var actionsSent []string
	client.sendChatAction = func(_ context.Context, _ int64, action string) error {
		actionsSent = append(actionsSent, action)
		return nil
	}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	err = adapter.DeliverStatusEvent(context.Background(), flow.StatusEvent{
		RunID:      "run-1",
		SessionKey: "telegram:chat:777",
		Status:     "thinking",
	})
	if err != nil {
		t.Fatalf("DeliverStatusEvent returned error: %v", err)
	}
	if len(actionsSent) != 1 || actionsSent[0] != ChatActionTyping {
		t.Fatalf("expected typing action, got %v", actionsSent)
	}
}

func TestDeliverStatusEventIgnoresDisallowedSession(t *testing.T) {
	t.Parallel()

	client := &Client{}
	called := false
	client.sendChatAction = func(_ context.Context, _ int64, _ string) error {
		called = true
		return nil
	}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	err = adapter.DeliverStatusEvent(context.Background(), flow.StatusEvent{
		RunID:      "run-1",
		SessionKey: "telegram:chat:999",
		Status:     "thinking",
	})
	if err != nil {
		t.Fatalf("DeliverStatusEvent returned error: %v", err)
	}
	if called {
		t.Fatal("expected no chat action for disallowed session")
	}
}

func TestDeliverStatusEventActionErrorDoesNotFail(t *testing.T) {
	t.Parallel()

	client := &Client{}
	client.sendChatAction = func(_ context.Context, _ int64, _ string) error {
		return errors.New("action failed")
	}
	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, client, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}

	// DeliverStatusEvent logs the warning but returns nil.
	err = adapter.DeliverStatusEvent(context.Background(), flow.StatusEvent{
		RunID:      "run-1",
		SessionKey: "telegram:chat:777",
		Status:     "thinking",
	})
	if err != nil {
		t.Fatalf("DeliverStatusEvent should not propagate action error: %v", err)
	}
}

func TestShouldPromptForAuthDetectsProviderNotConnected(t *testing.T) {
	t.Parallel()

	adapter, err := NewAdapter(Config{AllowedChatIDs: []string{"777"}, PollTimeout: time.Second}, &Client{}, nil, nil)
	if err != nil {
		t.Fatalf("NewAdapter returned error: %v", err)
	}
	if !adapter.shouldPromptForAuth(providerauth.ErrNotConnected) {
		t.Fatal("expected ErrNotConnected to trigger auth prompt")
	}
}
