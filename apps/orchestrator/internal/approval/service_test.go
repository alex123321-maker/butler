package approval

import (
	"context"
	"testing"
	"time"
)

type memoryRepo struct {
	recordsByID       map[string]Record
	recordsByToolCall map[string]Record
	tabCandidates     map[string][]TabCandidate
	events            []Event
}

func newMemoryRepo() *memoryRepo {
	return &memoryRepo{
		recordsByID:       map[string]Record{},
		recordsByToolCall: map[string]Record{},
		tabCandidates:     map[string][]TabCandidate{},
	}
}

func (m *memoryRepo) CreateApproval(_ context.Context, params CreateParams) (Record, error) {
	rec := Record{
		ApprovalID:   params.ApprovalID,
		RunID:        params.RunID,
		SessionKey:   params.SessionKey,
		ToolCallID:   params.ToolCallID,
		ApprovalType: params.ApprovalType,
		Status:       StatusPending,
		RequestedVia: params.RequestedVia,
		ToolName:     params.ToolName,
		ArgsJSON:     params.ArgsJSON,
		PayloadJSON:  params.PayloadJSON,
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

func (m *memoryRepo) GetApprovalByCandidateToken(_ context.Context, candidateToken string) (Record, error) {
	for approvalID, candidates := range m.tabCandidates {
		for _, candidate := range candidates {
			if candidate.CandidateToken != candidateToken {
				continue
			}
			rec, ok := m.recordsByID[approvalID]
			if !ok {
				return Record{}, ErrApprovalNotFound
			}
			return rec, nil
		}
	}
	return Record{}, ErrApprovalNotFound
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

func (m *memoryRepo) CreateTabCandidates(_ context.Context, params []CreateTabCandidateParams) error {
	for _, candidate := range params {
		status := candidate.Status
		if status == "" {
			status = "available"
		}
		m.tabCandidates[candidate.ApprovalID] = append(m.tabCandidates[candidate.ApprovalID], TabCandidate{
			ApprovalID:     candidate.ApprovalID,
			CandidateToken: candidate.CandidateToken,
			InternalTabRef: candidate.InternalTabRef,
			Title:          candidate.Title,
			Domain:         candidate.Domain,
			CurrentURL:     candidate.CurrentURL,
			FaviconURL:     candidate.FaviconURL,
			DisplayLabel:   candidate.DisplayLabel,
			Status:         status,
			CreatedAt:      time.Now().UTC(),
		})
	}
	return nil
}

func (m *memoryRepo) ListTabCandidates(_ context.Context, approvalID string) ([]TabCandidate, error) {
	return append([]TabCandidate(nil), m.tabCandidates[approvalID]...), nil
}

func (m *memoryRepo) GetTabCandidateByToken(_ context.Context, candidateToken string) (TabCandidate, error) {
	for _, candidates := range m.tabCandidates {
		for _, candidate := range candidates {
			if candidate.CandidateToken == candidateToken {
				return candidate, nil
			}
		}
	}
	return TabCandidate{}, ErrTabCandidateNotFound
}

func (m *memoryRepo) SelectTabCandidate(_ context.Context, approvalID, candidateToken string, selectedAt time.Time) (TabCandidate, error) {
	items := m.tabCandidates[approvalID]
	for index := range items {
		if items[index].CandidateToken == candidateToken {
			items[index].Status = "selected"
			items[index].SelectedAt = &selectedAt
			for other := range items {
				if other != index && items[other].Status == "available" {
					items[other].Status = "cancelled"
				}
			}
			m.tabCandidates[approvalID] = items
			return items[index], nil
		}
	}
	return TabCandidate{}, ErrTabCandidateNotFound
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
		ApprovalType: ApprovalTypeToolCall,
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
		ApprovalType: ApprovalTypeToolCall,
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

func TestServiceResolveByApprovalID(t *testing.T) {
	t.Parallel()

	repo := newMemoryRepo()
	gate := &fakeGate{}
	svc := NewService(repo, gate)
	now := time.Now().UTC()

	rec, err := svc.CreatePendingApproval(context.Background(), CreatePendingParams{
		RunID:        "run-3",
		SessionKey:   "web:session:3",
		ToolCallID:   "tool-3",
		ApprovalType: ApprovalTypeBrowserTabSelection,
		RequestedVia: RequestedViaBoth,
		ToolName:     "single_tab.bind",
		RequestedAt:  now,
		TabCandidates: []CreateTabCandidateParams{{
			CandidateToken: "tok-1",
			InternalTabRef: "browser-a:1",
		}},
	})
	if err != nil {
		t.Fatalf("CreatePendingApproval returned error: %v", err)
	}

	updated, changed, err := svc.ResolveByApprovalID(context.Background(), ResolveByApprovalIDParams{
		ApprovalID:       rec.ApprovalID,
		Approved:         false,
		ResolvedVia:      ResolvedViaWeb,
		ResolvedBy:       "web",
		ResolutionReason: "cancelled in web ui",
		ResolvedAt:       now.Add(time.Minute),
		ActorType:        "web",
		ActorID:          "web",
	})
	if err != nil {
		t.Fatalf("ResolveByApprovalID returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true")
	}
	if updated.Status != StatusRejected {
		t.Fatalf("expected rejected status, got %q", updated.Status)
	}
	if gate.resolved["tool-3"] {
		t.Fatal("expected gate resolution to remain false")
	}
}

func TestServiceCreateBrowserTabSelectionApprovalWithCandidates(t *testing.T) {
	t.Parallel()

	repo := newMemoryRepo()
	svc := NewService(repo, nil)

	now := time.Date(2026, 3, 22, 9, 0, 0, 0, time.UTC)
	rec, err := svc.CreatePendingApproval(context.Background(), CreatePendingParams{
		RunID:        "run-tab-1",
		SessionKey:   "web:session:1",
		ToolCallID:   "tool-bind-1",
		ApprovalType: ApprovalTypeBrowserTabSelection,
		RequestedVia: RequestedViaBoth,
		ToolName:     "single_tab.bind",
		PayloadJSON:  `{"selection_mode":"single"}`,
		Summary:      "Select a browser tab",
		RequestedAt:  now,
		TabCandidates: []CreateTabCandidateParams{
			{
				CandidateToken: "tabtok-1",
				InternalTabRef: "browser-a:41",
				Title:          "Inbox",
				Domain:         "mail.example.com",
				CurrentURL:     "https://mail.example.com/inbox",
				DisplayLabel:   "Inbox - mail.example.com",
			},
		},
	})
	if err != nil {
		t.Fatalf("CreatePendingApproval returned error: %v", err)
	}
	if rec.ApprovalType != ApprovalTypeBrowserTabSelection {
		t.Fatalf("expected browser_tab_selection approval type, got %q", rec.ApprovalType)
	}
	if rec.PayloadJSON != `{"selection_mode":"single"}` {
		t.Fatalf("expected payload_json to be preserved, got %q", rec.PayloadJSON)
	}
	candidates, err := repo.ListTabCandidates(context.Background(), rec.ApprovalID)
	if err != nil {
		t.Fatalf("ListTabCandidates returned error: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected 1 tab candidate, got %d", len(candidates))
	}
	if candidates[0].CandidateToken != "tabtok-1" {
		t.Fatalf("expected candidate token tabtok-1, got %q", candidates[0].CandidateToken)
	}
}
