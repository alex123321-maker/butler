package singletab

import (
	"context"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
)

type memoryApprovalStore struct {
	record     approvals.Record
	candidates []approvals.TabCandidate
	events     []approvals.Event
}

func (m *memoryApprovalStore) GetApprovalByID(_ context.Context, approvalID string) (approvals.Record, error) {
	if m.record.ApprovalID != approvalID {
		return approvals.Record{}, approvals.ErrApprovalNotFound
	}
	return m.record, nil
}

func (m *memoryApprovalStore) ListTabCandidates(_ context.Context, approvalID string) ([]approvals.TabCandidate, error) {
	if m.record.ApprovalID != approvalID {
		return nil, approvals.ErrApprovalNotFound
	}
	return append([]approvals.TabCandidate(nil), m.candidates...), nil
}

func (m *memoryApprovalStore) SelectTabCandidate(_ context.Context, approvalID, candidateToken string, selectedAt time.Time) (approvals.TabCandidate, error) {
	if m.record.ApprovalID != approvalID {
		return approvals.TabCandidate{}, approvals.ErrApprovalNotFound
	}
	for index := range m.candidates {
		if m.candidates[index].CandidateToken == candidateToken {
			m.candidates[index].Status = "selected"
			m.candidates[index].SelectedAt = &selectedAt
			return m.candidates[index], nil
		}
	}
	return approvals.TabCandidate{}, approvals.ErrTabCandidateNotFound
}

func (m *memoryApprovalStore) ResolveApproval(_ context.Context, params approvals.ResolveParams) (approvals.Record, error) {
	if m.record.ApprovalID != params.ApprovalID {
		return approvals.Record{}, approvals.ErrApprovalNotFound
	}
	m.record.Status = params.Status
	m.record.ResolvedVia = params.ResolvedVia
	m.record.ResolvedBy = params.ResolvedBy
	m.record.ResolutionReason = params.ResolutionReason
	m.record.ResolvedAt = &params.ResolvedAt
	m.record.UpdatedAt = params.ResolvedAt
	return m.record, nil
}

func (m *memoryApprovalStore) InsertEvent(_ context.Context, event approvals.Event) error {
	m.events = append(m.events, event)
	return nil
}

type fakeApprovalCreator struct {
	record     approvals.Record
	candidates []approvals.CreateTabCandidateParams
}

func (f *fakeApprovalCreator) CreatePendingApproval(_ context.Context, params approvals.CreatePendingParams) (approvals.Record, error) {
	f.candidates = append([]approvals.CreateTabCandidateParams(nil), params.TabCandidates...)
	f.record = approvals.Record{
		ApprovalID:   "approval-bind-1",
		RunID:        params.RunID,
		SessionKey:   params.SessionKey,
		ToolCallID:   params.ToolCallID,
		ApprovalType: params.ApprovalType,
		Status:       approvals.StatusPending,
		RequestedVia: params.RequestedVia,
		ToolName:     params.ToolName,
		ArgsJSON:     params.ArgsJSON,
		PayloadJSON:  params.PayloadJSON,
		Summary:      params.Summary,
		RequestedAt:  params.RequestedAt,
		UpdatedAt:    params.RequestedAt,
	}
	return f.record, nil
}

type fakeDeliverySink struct {
	requests []flow.ApprovalRequest
}

func (f *fakeDeliverySink) DeliverApprovalRequest(_ context.Context, req flow.ApprovalRequest) error {
	f.requests = append(f.requests, req)
	return nil
}

type memorySessionRepo struct {
	byID         map[string]Record
	byApprovalID map[string]Record
}

func newMemorySessionRepo() *memorySessionRepo {
	return &memorySessionRepo{
		byID:         map[string]Record{},
		byApprovalID: map[string]Record{},
	}
}

func (m *memorySessionRepo) CreateSession(_ context.Context, params CreateParams) (Record, error) {
	record := Record{
		SingleTabSessionID: params.SingleTabSessionID,
		SessionKey:         params.SessionKey,
		CreatedByRunID:     params.CreatedByRunID,
		ApprovalID:         params.ApprovalID,
		Status:             params.Status,
		BoundTabRef:        params.BoundTabRef,
		BrowserInstanceID:  params.BrowserInstanceID,
		HostID:             params.HostID,
		SelectedVia:        params.SelectedVia,
		SelectedBy:         params.SelectedBy,
		CurrentURL:         params.CurrentURL,
		CurrentTitle:       params.CurrentTitle,
		StatusReason:       params.StatusReason,
		MetadataJSON:       params.MetadataJSON,
		CreatedAt:          params.CreatedAt,
		ActivatedAt:        params.ActivatedAt,
		ExpiresAt:          params.ExpiresAt,
		UpdatedAt:          params.CreatedAt,
	}
	m.byID[record.SingleTabSessionID] = record
	if record.ApprovalID != "" {
		m.byApprovalID[record.ApprovalID] = record
	}
	return record, nil
}

func (m *memorySessionRepo) GetSessionByID(_ context.Context, sessionID string) (Record, error) {
	record, ok := m.byID[sessionID]
	if !ok {
		return Record{}, ErrSessionNotFound
	}
	return record, nil
}

func (m *memorySessionRepo) GetSessionByApprovalID(_ context.Context, approvalID string) (Record, error) {
	record, ok := m.byApprovalID[approvalID]
	if !ok {
		return Record{}, ErrSessionNotFound
	}
	return record, nil
}

func (m *memorySessionRepo) GetActiveSessionBySessionKey(_ context.Context, sessionKey string) (Record, error) {
	for _, record := range m.byID {
		if record.SessionKey == sessionKey && record.Status == StatusActive {
			return record, nil
		}
	}
	return Record{}, ErrSessionNotFound
}

func (m *memorySessionRepo) UpdateSessionStatus(_ context.Context, params UpdateStatusParams) (Record, error) {
	record, ok := m.byID[params.SingleTabSessionID]
	if !ok {
		return Record{}, ErrSessionNotFound
	}
	record.Status = params.Status
	record.StatusReason = params.StatusReason
	record.SelectedVia = params.SelectedVia
	record.SelectedBy = params.SelectedBy
	record.CurrentURL = params.CurrentURL
	record.CurrentTitle = params.CurrentTitle
	record.LastSeenAt = params.LastSeenAt
	record.ActivatedAt = params.ActivatedAt
	record.ReleasedAt = params.ReleasedAt
	record.ExpiresAt = params.ExpiresAt
	record.UpdatedAt = params.UpdatedAt
	m.byID[record.SingleTabSessionID] = record
	return record, nil
}

type fakeGate struct {
	resolved map[string]bool
}

func (f *fakeGate) Resolve(toolCallID string, approved bool) bool {
	if f.resolved == nil {
		f.resolved = map[string]bool{}
	}
	f.resolved[toolCallID] = approved
	return true
}

func TestActivateFromApprovalCreatesActiveSessionAndResolvesGate(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	approvalStore := &memoryApprovalStore{
		record: approvals.Record{
			ApprovalID:   "approval-1",
			RunID:        "run-1",
			SessionKey:   "telegram:chat:1",
			ToolCallID:   "tool-bind-1",
			ApprovalType: approvals.ApprovalTypeBrowserTabSelection,
			Status:       approvals.StatusPending,
			RequestedVia: approvals.RequestedViaBoth,
			ToolName:     "single_tab.bind",
			RequestedAt:  now,
			UpdatedAt:    now,
		},
		candidates: []approvals.TabCandidate{{
			ApprovalID:     "approval-1",
			CandidateToken: "tok-1",
			InternalTabRef: "browser-a:42",
			Title:          "Inbox",
			Domain:         "mail.example.com",
			CurrentURL:     "https://mail.example.com/inbox",
			Status:         "available",
			CreatedAt:      now,
		}},
	}
	sessionRepo := newMemorySessionRepo()
	gate := &fakeGate{}
	svc := NewService(approvalStore, sessionRepo, gate)

	result, err := svc.ActivateFromApproval(context.Background(), ActivateFromApprovalParams{
		ApprovalID:     "approval-1",
		CandidateToken: "tok-1",
		ResolvedVia:    approvals.ResolvedViaTelegram,
		ResolvedBy:     "telegram_user:42",
		ActorType:      "telegram",
		ActorID:        "telegram_user:42",
		ResolvedAt:     now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ActivateFromApproval returned error: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected changed=true")
	}
	if result.Session.Status != StatusActive {
		t.Fatalf("expected active session, got %q", result.Session.Status)
	}
	if result.Session.BoundTabRef != "browser-a:42" {
		t.Fatalf("unexpected bound_tab_ref %q", result.Session.BoundTabRef)
	}
	if !gate.resolved["tool-bind-1"] {
		t.Fatal("expected gate to be resolved for the bind tool call")
	}
	if len(approvalStore.events) == 0 {
		t.Fatal("expected approval event to be recorded")
	}
}

func TestCreateBindRequestCreatesTypedApprovalAndDeliversCandidates(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 22, 13, 0, 0, 0, time.UTC)
	approvalStore := &memoryApprovalStore{
		record: approvals.Record{
			ApprovalID:   "approval-bind-1",
			RunID:        "run-2",
			SessionKey:   "telegram:chat:2",
			ToolCallID:   "single-tab-bind-1",
			ApprovalType: approvals.ApprovalTypeBrowserTabSelection,
			Status:       approvals.StatusPending,
			RequestedVia: approvals.RequestedViaBoth,
			ToolName:     "single_tab.bind",
			RequestedAt:  now,
			UpdatedAt:    now,
		},
		candidates: []approvals.TabCandidate{{
			ApprovalID:     "approval-bind-1",
			CandidateToken: "tabtok-a",
			InternalTabRef: "browser-a:17",
			Title:          "Inbox",
			Domain:         "mail.example.com",
			CurrentURL:     "https://mail.example.com/inbox",
			DisplayLabel:   "Inbox - mail.example.com",
			Status:         "available",
			CreatedAt:      now,
		}},
	}
	sessionRepo := newMemorySessionRepo()
	creator := &fakeApprovalCreator{}
	delivery := &fakeDeliverySink{}
	svc := NewService(approvalStore, sessionRepo, &fakeGate{})
	svc.SetApprovalCreator(creator)
	svc.SetDeliverySink(delivery)

	result, err := svc.CreateBindRequest(context.Background(), CreateBindRequestParams{
		RunID:        "run-2",
		SessionKey:   "telegram:chat:2",
		RequestedVia: approvals.RequestedViaBoth,
		Candidates: []BindCandidateInput{{
			InternalTabRef: "browser-a:17",
			Title:          "Inbox",
			Domain:         "mail.example.com",
			CurrentURL:     "https://mail.example.com/inbox",
		}},
		RequestedAt:   now,
		RequestSource: "browser-bridge",
	})
	if err != nil {
		t.Fatalf("CreateBindRequest returned error: %v", err)
	}
	if result.Approval.ApprovalType != approvals.ApprovalTypeBrowserTabSelection {
		t.Fatalf("expected browser tab selection approval type, got %q", result.Approval.ApprovalType)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(result.Candidates))
	}
	if len(creator.candidates) != 1 || creator.candidates[0].CandidateToken == "" {
		t.Fatal("expected generated candidate token to be passed into approval creation")
	}
	if len(delivery.requests) != 1 {
		t.Fatalf("expected one approval delivery, got %d", len(delivery.requests))
	}
	if delivery.requests[0].ApprovalType != approvals.ApprovalTypeBrowserTabSelection {
		t.Fatalf("expected delivered approval type %q, got %q", approvals.ApprovalTypeBrowserTabSelection, delivery.requests[0].ApprovalType)
	}
	if len(delivery.requests[0].TabCandidates) != 1 {
		t.Fatalf("expected one delivered tab candidate, got %d", len(delivery.requests[0].TabCandidates))
	}
}

func TestReleaseSessionMarksSessionRevoked(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 22, 14, 0, 0, 0, time.UTC)
	approvalStore := &memoryApprovalStore{
		record: approvals.Record{
			ApprovalID: "approval-release-1",
			RunID:      "run-3",
			SessionKey: "web:session:3",
			UpdatedAt:  now,
		},
	}
	sessionRepo := newMemorySessionRepo()
	_, err := sessionRepo.CreateSession(context.Background(), CreateParams{
		SingleTabSessionID: "single-tab-release-1",
		SessionKey:         "web:session:3",
		CreatedByRunID:     "run-3",
		ApprovalID:         "approval-release-1",
		Status:             StatusActive,
		BoundTabRef:        "browser-a:31",
		CurrentURL:         "https://example.com",
		CurrentTitle:       "Example",
		SelectedVia:        "web",
		SelectedBy:         "alice",
		CreatedAt:          now,
	})
	if err != nil {
		t.Fatalf("CreateSession returned error: %v", err)
	}

	svc := NewService(approvalStore, sessionRepo, &fakeGate{})
	updated, changed, err := svc.ReleaseSession(context.Background(), ReleaseSessionParams{
		SingleTabSessionID: "single-tab-release-1",
		ReleasedBy:         "alice",
		ReleasedVia:        approvals.ResolvedViaWeb,
		ActorType:          "web",
		ActorID:            "alice",
		ReleasedAt:         now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ReleaseSession returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if updated.Status != StatusRevokedByUser {
		t.Fatalf("expected revoked status, got %q", updated.Status)
	}
	if updated.ReleasedAt == nil {
		t.Fatal("expected released_at to be set")
	}
}
