package approval

import (
	"context"
	"testing"
	"time"
)

type memoryRepo struct {
	recordsByID       map[string]Record
	recordsByToolCall map[string]Record
	events            []Event
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		recordsByID:       map[string]Record{},
		recordsByToolCall: map[string]Record{},
	}
}

func (m *memoryRepo) CreateApproval(_ context.Context, params CreateParams) (Record, error) {
	rec := Record{
		ApprovalID:   params.ApprovalID,
		RunID:        params.RunID,
		SessionKey:   params.SessionKey,
		ToolCallID:   params.ToolCallID,
		Status:       StatusPending,
		RequestedVia: params.RequestedVia,
		ToolName:     params.ToolName,
		ArgsJSON:     params.ArgsJSON,
		RiskLevel:    params.RiskLevel,
		Summary:      params.Summary,
		DetailsJSON:  params.DetailsJSON,
		RequestedAt:  params.RequestedAt,
		ExpiresAt:    params.ExpiresAt,
		UpdatedAt:    params.RequestedAt,
	}
	m.recordsByID[rec.ApprovalID] = rec
	m.recordsByToolCall[rec.ToolCallID] = rec
	return rec, nil
}

func (m *memoryRepo) GetApprovalByToolCallID(_ context.Context, toolCallID string) (Record, error) {
	rec, ok := m.recordsByToolCall[toolCallID]
	if !ok {
		return Record{}, ErrApprovalNotFound
	}
	return rec, nil
}

func (m *memoryRepo) GetApprovalByID(_ context.Context, approvalID string) (Record, error) {
	rec, ok := m.recordsByID[approvalID]
	if !ok {
		return Record{}, ErrApprovalNotFound
	}
	return rec, nil
}

func (m *memoryRepo) ResolveApproval(_ context.Context, params ResolveParams) (Record, error) {
	rec, ok := m.recordsByID[params.ApprovalID]
	if !ok {
		return Record{}, ErrApprovalNotFound
	}
	if rec.Status != params.ExpectedStatus {
		return Record{}, ErrApprovalStatusConflict
	}
	rec.Status = params.Status
	rec.ResolvedVia = params.ResolvedVia
	rec.ResolvedBy = params.ResolvedBy
	rec.ResolutionReason = params.ResolutionReason
	rec.ResolvedAt = &params.ResolvedAt
	rec.UpdatedAt = params.ResolvedAt
	m.recordsByID[params.ApprovalID] = rec
	m.recordsByToolCall[rec.ToolCallID] = rec
	return rec, nil
}

func (m *memoryRepo) InsertEvent(_ context.Context, event Event) error {
	m.events = append(m.events, event)
	return nil
}

func (m *memoryRepo) ListApprovals(_ context.Context, status, runID, sessionKey string, limit, offset int) ([]Record, error) {
	items := make([]Record, 0, len(m.recordsByID))
	for _, rec := range m.recordsByID {
		if status != "" && rec.Status != status {
			continue
		}
		if runID != "" && rec.RunID != runID {
			continue
		}
		if sessionKey != "" && rec.SessionKey != sessionKey {
			continue
		}
		items = append(items, rec)
	}
	return items, nil
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

func TestServiceCreateAndResolveApproval(t *testing.T) {
	t.Parallel()

	repo := newMemoryRepo()
	gate := &fakeGate{}
	svc := NewService(repo, gate)

	now := time.Date(2026, 3, 19, 13, 0, 0, 0, time.UTC)
	rec, err := svc.CreatePendingApproval(context.Background(), CreatePendingParams{
		RunID:        "run-1",
		SessionKey:   "telegram:chat:1",
		ToolCallID:   "tool-1",
		RequestedVia: RequestedViaTelegram,
		ToolName:     "http.request",
		ArgsJSON:     `{"url":"https://example.com"}`,
		RiskLevel:    "medium",
		Summary:      "approval required",
		RequestedAt:  now,
	})
	if err != nil {
		t.Fatalf("CreatePendingApproval returned error: %v", err)
	}
	if rec.Status != StatusPending {
		t.Fatalf("expected pending status, got %q", rec.Status)
	}

	updated, changed, err := svc.ResolveByToolCall(context.Background(), ResolveByToolCallParams{
		ToolCallID:       "tool-1",
		Approved:         true,
		ResolvedVia:      ResolvedViaTelegram,
		ResolvedBy:       "telegram_user:42",
		ResolutionReason: "approved in telegram",
		ResolvedAt:       now.Add(time.Minute),
		ActorType:        "telegram",
		ActorID:          "telegram_user:42",
	})
	if err != nil {
		t.Fatalf("ResolveByToolCall returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true for first resolution")
	}
	if updated.Status != StatusApproved {
		t.Fatalf("expected approved status, got %q", updated.Status)
	}
	if !gate.resolved["tool-1"] {
		t.Fatal("expected approval gate to be resolved as approved")
	}
	if len(repo.events) < 2 {
		t.Fatalf("expected at least requested+resolved events, got %d", len(repo.events))
	}
}

func TestServiceResolveIdempotentAfterAlreadyResolved(t *testing.T) {
	t.Parallel()

	repo := newMemoryRepo()
	gate := &fakeGate{}
	svc := NewService(repo, gate)
	now := time.Now().UTC()

	_, _ = svc.CreatePendingApproval(context.Background(), CreatePendingParams{
		RunID:        "run-2",
		SessionKey:   "telegram:chat:2",
		ToolCallID:   "tool-2",
		RequestedVia: RequestedViaTelegram,
		ToolName:     "http.request",
		RequestedAt:  now,
	})

	_, changed, err := svc.ResolveByToolCall(context.Background(), ResolveByToolCallParams{ToolCallID: "tool-2", Approved: false, ResolvedVia: ResolvedViaTelegram, ResolvedAt: now.Add(time.Second)})
	if err != nil || !changed {
		t.Fatalf("expected first resolve to change status, changed=%v err=%v", changed, err)
	}
	_, changed, err = svc.ResolveByToolCall(context.Background(), ResolveByToolCallParams{ToolCallID: "tool-2", Approved: true, ResolvedVia: ResolvedViaTelegram, ResolvedAt: now.Add(2 * time.Second)})
	if err != nil {
		t.Fatalf("second resolve returned error: %v", err)
	}
	if changed {
		t.Fatal("expected second resolve to be idempotent changed=false")
	}
	if gate.resolved["tool-2"] {
		t.Fatal("expected gate to stay rejected from first resolution")
	}
}
