package approval

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Gate interface {
	Resolve(toolCallID string, approved bool) bool
}

type Service struct {
	repo Repository
	gate Gate
}

func NewService(repo Repository, gate Gate) *Service {
	return &Service{repo: repo, gate: gate}
}

type CreatePendingParams struct {
	RunID         string
	SessionKey    string
	ToolCallID    string
	ApprovalType  string
	RequestedVia  string
	ToolName      string
	ArgsJSON      string
	PayloadJSON   string
	RiskLevel     string
	Summary       string
	DetailsJSON   string
	TabCandidates []CreateTabCandidateParams
	RequestedAt   time.Time
	ExpiresAt     *time.Time
}

type ResolveByToolCallParams struct {
	ToolCallID       string
	Approved         bool
	ResolvedVia      string
	ResolvedBy       string
	ResolutionReason string
	ResolvedAt       time.Time
	ActorType        string
	ActorID          string
}

type ResolveByApprovalIDParams struct {
	ApprovalID       string
	Approved         bool
	ResolvedVia      string
	ResolvedBy       string
	ResolutionReason string
	ResolvedAt       time.Time
	ActorType        string
	ActorID          string
}

func (s *Service) CreatePendingApproval(ctx context.Context, params CreatePendingParams) (Record, error) {
	if s == nil || s.repo == nil {
		return Record{}, fmt.Errorf("approval service is not configured")
	}
	if strings.TrimSpace(params.RunID) == "" || strings.TrimSpace(params.SessionKey) == "" || strings.TrimSpace(params.ToolCallID) == "" {
		return Record{}, fmt.Errorf("run_id, session_key and tool_call_id are required")
	}
	if strings.TrimSpace(params.ToolName) == "" {
		return Record{}, fmt.Errorf("tool_name is required")
	}
	if params.RequestedAt.IsZero() {
		params.RequestedAt = time.Now().UTC()
	}
	if strings.TrimSpace(params.ApprovalType) == "" {
		params.ApprovalType = ApprovalTypeToolCall
	}
	if strings.TrimSpace(params.RequestedVia) == "" {
		params.RequestedVia = RequestedViaTelegram
	}
	if strings.TrimSpace(params.PayloadJSON) == "" {
		params.PayloadJSON = "{}"
	}
	if params.ApprovalType == ApprovalTypeBrowserTabSelection && len(params.TabCandidates) == 0 {
		return Record{}, fmt.Errorf("browser_tab_selection approvals require at least one tab candidate")
	}

	record, err := s.repo.CreateApproval(ctx, CreateParams{
		ApprovalID:   newApprovalID(),
		RunID:        params.RunID,
		SessionKey:   params.SessionKey,
		ToolCallID:   params.ToolCallID,
		ApprovalType: params.ApprovalType,
		RequestedVia: params.RequestedVia,
		ToolName:     params.ToolName,
		ArgsJSON:     params.ArgsJSON,
		PayloadJSON:  params.PayloadJSON,
		RiskLevel:    params.RiskLevel,
		Summary:      params.Summary,
		DetailsJSON:  params.DetailsJSON,
		RequestedAt:  params.RequestedAt,
		ExpiresAt:    params.ExpiresAt,
	})
	if err != nil {
		return Record{}, err
	}

	if len(params.TabCandidates) > 0 {
		for index := range params.TabCandidates {
			params.TabCandidates[index].ApprovalID = record.ApprovalID
		}
		if err := s.repo.CreateTabCandidates(ctx, params.TabCandidates); err != nil {
			return Record{}, err
		}
	}

	_ = s.repo.InsertEvent(ctx, Event{
		ApprovalID:   record.ApprovalID,
		RunID:        record.RunID,
		SessionKey:   record.SessionKey,
		EventType:    "approval_requested",
		StatusBefore: "",
		StatusAfter:  StatusPending,
		ActorType:    "system",
		ActorID:      "orchestrator",
		Reason:       "approval required before tool execution",
		MetadataJSON: fmt.Sprintf(`{"phase":"pre_wait","approval_type":%q}`, record.ApprovalType),
		CreatedAt:    params.RequestedAt,
	})

	return record, nil
}

func (s *Service) ResolveByToolCall(ctx context.Context, params ResolveByToolCallParams) (Record, bool, error) {
	if s == nil || s.repo == nil {
		return Record{}, false, fmt.Errorf("approval service is not configured")
	}
	toolCallID := strings.TrimSpace(params.ToolCallID)
	if toolCallID == "" {
		return Record{}, false, fmt.Errorf("tool_call_id is required")
	}
	if params.ResolvedAt.IsZero() {
		params.ResolvedAt = time.Now().UTC()
	}
	resolvedVia := strings.TrimSpace(params.ResolvedVia)
	if resolvedVia == "" {
		resolvedVia = ResolvedViaSystem
	}
	actorType := strings.TrimSpace(params.ActorType)
	if actorType == "" {
		actorType = "system"
	}
	record, err := s.repo.GetApprovalByToolCallID(ctx, toolCallID)
	if err != nil {
		return Record{}, false, err
	}

	return s.resolveRecord(ctx, record, params.Approved, resolvedVia, params.ResolvedBy, params.ResolutionReason, params.ResolvedAt, actorType, params.ActorID)
}

func (s *Service) ResolveByApprovalID(ctx context.Context, params ResolveByApprovalIDParams) (Record, bool, error) {
	if s == nil || s.repo == nil {
		return Record{}, false, fmt.Errorf("approval service is not configured")
	}
	approvalID := strings.TrimSpace(params.ApprovalID)
	if approvalID == "" {
		return Record{}, false, fmt.Errorf("approval_id is required")
	}
	if params.ResolvedAt.IsZero() {
		params.ResolvedAt = time.Now().UTC()
	}
	resolvedVia := strings.TrimSpace(params.ResolvedVia)
	if resolvedVia == "" {
		resolvedVia = ResolvedViaSystem
	}
	actorType := strings.TrimSpace(params.ActorType)
	if actorType == "" {
		actorType = "system"
	}

	record, err := s.repo.GetApprovalByID(ctx, approvalID)
	if err != nil {
		return Record{}, false, err
	}

	return s.resolveRecord(ctx, record, params.Approved, resolvedVia, params.ResolvedBy, params.ResolutionReason, params.ResolvedAt, actorType, params.ActorID)
}

func (s *Service) resolveRecord(ctx context.Context, record Record, approved bool, resolvedVia, resolvedBy, resolutionReason string, resolvedAt time.Time, actorType, actorID string) (Record, bool, error) {
	status := StatusRejected
	if approved {
		status = StatusApproved
	}

	if record.Status != StatusPending {
		if s.gate != nil {
			s.gate.Resolve(record.ToolCallID, record.Status == StatusApproved)
		}
		return record, false, nil
	}

	updated, err := s.repo.ResolveApproval(ctx, ResolveParams{
		ApprovalID:       record.ApprovalID,
		ExpectedStatus:   StatusPending,
		Status:           status,
		ResolvedVia:      resolvedVia,
		ResolvedBy:       resolvedBy,
		ResolutionReason: resolutionReason,
		ResolvedAt:       resolvedAt,
	})
	if err != nil {
		if err == ErrApprovalStatusConflict {
			reloaded, getErr := s.repo.GetApprovalByID(ctx, record.ApprovalID)
			if getErr != nil {
				return Record{}, false, getErr
			}
			if s.gate != nil {
				s.gate.Resolve(record.ToolCallID, reloaded.Status == StatusApproved)
			}
			return reloaded, false, nil
		}
		return Record{}, false, err
	}

	_ = s.repo.InsertEvent(ctx, Event{
		ApprovalID:   updated.ApprovalID,
		RunID:        updated.RunID,
		SessionKey:   updated.SessionKey,
		EventType:    "approval_resolved",
		StatusBefore: StatusPending,
		StatusAfter:  status,
		ActorType:    actorType,
		ActorID:      actorID,
		Reason:       resolutionReason,
		MetadataJSON: fmt.Sprintf(`{"resolved_via":%q,"approved":%t}`, resolvedVia, approved),
		CreatedAt:    resolvedAt,
	})

	if s.gate != nil {
		s.gate.Resolve(record.ToolCallID, approved)
	}

	return updated, true, nil
}

func newApprovalID() string {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("approval-%d", time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("approval-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(raw[:]))
}
