package singletab

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
)

type approvalStore interface {
	GetApprovalByID(ctx context.Context, approvalID string) (approvals.Record, error)
	GetApprovalByCandidateToken(ctx context.Context, candidateToken string) (approvals.Record, error)
	ListTabCandidates(ctx context.Context, approvalID string) ([]approvals.TabCandidate, error)
	GetTabCandidateByToken(ctx context.Context, candidateToken string) (approvals.TabCandidate, error)
	SelectTabCandidate(ctx context.Context, approvalID, candidateToken string, selectedAt time.Time) (approvals.TabCandidate, error)
	ResolveApproval(ctx context.Context, params approvals.ResolveParams) (approvals.Record, error)
	InsertEvent(ctx context.Context, event approvals.Event) error
}

type Gate interface {
	Resolve(toolCallID string, approved bool) bool
}

type approvalCreator interface {
	CreatePendingApproval(ctx context.Context, params approvals.CreatePendingParams) (approvals.Record, error)
}

type deliverySink interface {
	DeliverApprovalRequest(ctx context.Context, req flow.ApprovalRequest) error
}

type Service struct {
	approvals approvalStore
	creator   approvalCreator
	sessions  Repository
	gate      Gate
	delivery  deliverySink
}

func NewService(approvals approvalStore, sessions Repository, gate Gate) *Service {
	return &Service{approvals: approvals, sessions: sessions, gate: gate}
}

func (s *Service) SetApprovalCreator(creator approvalCreator) {
	if s != nil {
		s.creator = creator
	}
}

func (s *Service) SetDeliverySink(delivery deliverySink) {
	if s != nil {
		s.delivery = delivery
	}
}

type BindCandidateInput struct {
	InternalTabRef string
	Title          string
	Domain         string
	CurrentURL     string
	FaviconURL     string
	DisplayLabel   string
}

type CreateBindRequestParams struct {
	RunID         string
	SessionKey    string
	ToolCallID    string
	RequestedVia  string
	Candidates    []BindCandidateInput
	RequestedAt   time.Time
	ExpiresAt     *time.Time
	BrowserHint   string
	RequestSource string
}

type CreateBindRequestResult struct {
	Approval   approvals.Record
	Candidates []approvals.TabCandidate
}

func (s *Service) CreateBindRequest(ctx context.Context, params CreateBindRequestParams) (CreateBindRequestResult, error) {
	if s == nil || s.creator == nil || s.approvals == nil || s.sessions == nil || s.delivery == nil {
		return CreateBindRequestResult{}, fmt.Errorf("single tab bind request service is not configured")
	}
	if strings.TrimSpace(params.RunID) == "" {
		return CreateBindRequestResult{}, fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(params.SessionKey) == "" {
		return CreateBindRequestResult{}, fmt.Errorf("session_key is required")
	}
	if len(params.Candidates) == 0 {
		return CreateBindRequestResult{}, fmt.Errorf("at least one tab candidate is required")
	}
	if _, err := s.sessions.GetActiveSessionBySessionKey(ctx, params.SessionKey); err == nil {
		return CreateBindRequestResult{}, ErrActiveSessionBusy
	} else if err != ErrSessionNotFound {
		return CreateBindRequestResult{}, err
	}
	if params.RequestedAt.IsZero() {
		params.RequestedAt = time.Now().UTC()
	}
	if strings.TrimSpace(params.RequestedVia) == "" {
		params.RequestedVia = approvals.RequestedViaBoth
	}
	toolCallID := strings.TrimSpace(params.ToolCallID)
	if toolCallID == "" {
		toolCallID = newBindToolCallID()
	}

	payloadJSON, err := json.Marshal(map[string]any{
		"kind":            "browser_tab_selection",
		"selection_mode":  "single",
		"candidate_count": len(params.Candidates),
		"request_source":  strings.TrimSpace(params.RequestSource),
		"browser_hint":    strings.TrimSpace(params.BrowserHint),
	})
	if err != nil {
		return CreateBindRequestResult{}, fmt.Errorf("marshal bind payload: %w", err)
	}

	createCandidates := make([]approvals.CreateTabCandidateParams, 0, len(params.Candidates))
	for _, candidate := range params.Candidates {
		displayLabel := strings.TrimSpace(candidate.DisplayLabel)
		if displayLabel == "" {
			displayLabel = strings.TrimSpace(candidate.Title)
			if domain := strings.TrimSpace(candidate.Domain); domain != "" {
				if displayLabel != "" {
					displayLabel += " - " + domain
				} else {
					displayLabel = domain
				}
			}
		}
		createCandidates = append(createCandidates, approvals.CreateTabCandidateParams{
			CandidateToken: newCandidateToken(),
			InternalTabRef: candidate.InternalTabRef,
			Title:          candidate.Title,
			Domain:         candidate.Domain,
			CurrentURL:     candidate.CurrentURL,
			FaviconURL:     candidate.FaviconURL,
			DisplayLabel:   displayLabel,
			Status:         "available",
		})
	}

	record, err := s.creator.CreatePendingApproval(ctx, approvals.CreatePendingParams{
		RunID:         params.RunID,
		SessionKey:    params.SessionKey,
		ToolCallID:    toolCallID,
		ApprovalType:  approvals.ApprovalTypeBrowserTabSelection,
		RequestedVia:  params.RequestedVia,
		ToolName:      "single_tab.bind",
		ArgsJSON:      "{}",
		PayloadJSON:   string(payloadJSON),
		RiskLevel:     "medium",
		Summary:       "Select one browser tab to bind the agent",
		TabCandidates: createCandidates,
		RequestedAt:   params.RequestedAt,
		ExpiresAt:     params.ExpiresAt,
	})
	if err != nil {
		return CreateBindRequestResult{}, err
	}

	candidates, err := s.approvals.ListTabCandidates(ctx, record.ApprovalID)
	if err != nil {
		return CreateBindRequestResult{}, err
	}

	deliveryCandidates := make([]flow.ApprovalTabCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		deliveryCandidates = append(deliveryCandidates, flow.ApprovalTabCandidate{
			CandidateToken: candidate.CandidateToken,
			Title:          candidate.Title,
			Domain:         candidate.Domain,
			CurrentURL:     candidate.CurrentURL,
			FaviconURL:     candidate.FaviconURL,
			DisplayLabel:   candidate.DisplayLabel,
			Status:         candidate.Status,
		})
	}
	if err := s.delivery.DeliverApprovalRequest(ctx, flow.ApprovalRequest{
		RunID:         record.RunID,
		SessionKey:    record.SessionKey,
		ApprovalID:    record.ApprovalID,
		ApprovalType:  record.ApprovalType,
		ToolCallID:    record.ToolCallID,
		ToolName:      record.ToolName,
		ArgsJSON:      record.ArgsJSON,
		PayloadJSON:   record.PayloadJSON,
		TabCandidates: deliveryCandidates,
	}); err != nil {
		return CreateBindRequestResult{}, err
	}

	return CreateBindRequestResult{Approval: record, Candidates: candidates}, nil
}

func (s *Service) GetActiveSession(ctx context.Context, sessionKey string) (Record, error) {
	if s == nil || s.sessions == nil {
		return Record{}, fmt.Errorf("single tab session store is not configured")
	}
	if strings.TrimSpace(sessionKey) == "" {
		return Record{}, fmt.Errorf("session_key is required")
	}
	return s.sessions.GetActiveSessionBySessionKey(ctx, sessionKey)
}

func (s *Service) GetSession(ctx context.Context, sessionID string) (Record, error) {
	if s == nil || s.sessions == nil {
		return Record{}, fmt.Errorf("single tab session store is not configured")
	}
	if strings.TrimSpace(sessionID) == "" {
		return Record{}, fmt.Errorf("session_id is required")
	}
	return s.sessions.GetSessionByID(ctx, sessionID)
}

type ReleaseSessionParams struct {
	SingleTabSessionID string
	ReleasedBy         string
	ReleasedVia        string
	ActorType          string
	ActorID            string
	ReleasedAt         time.Time
}

type UpdateSessionStateParams struct {
	SingleTabSessionID string
	Status             string
	StatusReason       string
	CurrentURL         string
	CurrentTitle       string
	BrowserInstanceID  string
	HostID             string
	LastSeenAt         time.Time
	ReleasedAt         *time.Time
}

func (s *Service) ReleaseSession(ctx context.Context, params ReleaseSessionParams) (Record, bool, error) {
	if s == nil || s.sessions == nil {
		return Record{}, false, fmt.Errorf("single tab session store is not configured")
	}
	if strings.TrimSpace(params.SingleTabSessionID) == "" {
		return Record{}, false, fmt.Errorf("session_id is required")
	}
	if params.ReleasedAt.IsZero() {
		params.ReleasedAt = time.Now().UTC()
	}
	if strings.TrimSpace(params.ReleasedVia) == "" {
		params.ReleasedVia = approvals.ResolvedViaWeb
	}
	if strings.TrimSpace(params.ActorType) == "" {
		params.ActorType = "web"
	}
	if strings.TrimSpace(params.ActorID) == "" {
		params.ActorID = params.ReleasedBy
	}

	record, err := s.sessions.GetSessionByID(ctx, params.SingleTabSessionID)
	if err != nil {
		return Record{}, false, err
	}
	if record.Status != StatusActive {
		return record, false, nil
	}

	updated, err := s.sessions.UpdateSessionStatus(ctx, UpdateStatusParams{
		SingleTabSessionID: record.SingleTabSessionID,
		Status:             StatusRevokedByUser,
		StatusReason:       "released by user",
		SelectedVia:        firstNonEmpty(params.ReleasedVia, record.SelectedVia),
		SelectedBy:         firstNonEmpty(params.ReleasedBy, record.SelectedBy),
		CurrentURL:         record.CurrentURL,
		CurrentTitle:       record.CurrentTitle,
		ReleasedAt:         &params.ReleasedAt,
		UpdatedAt:          params.ReleasedAt,
	})
	if err != nil {
		return Record{}, false, err
	}

	if s.approvals != nil && strings.TrimSpace(record.ApprovalID) != "" {
		_ = s.approvals.InsertEvent(ctx, approvals.Event{
			ApprovalID:   record.ApprovalID,
			RunID:        record.CreatedByRunID,
			SessionKey:   record.SessionKey,
			EventType:    "single_tab_session_released",
			StatusBefore: StatusActive,
			StatusAfter:  StatusRevokedByUser,
			ActorType:    params.ActorType,
			ActorID:      params.ActorID,
			Reason:       "single-tab session released by user",
			MetadataJSON: fmt.Sprintf(`{"single_tab_session_id":%q}`, record.SingleTabSessionID),
			CreatedAt:    params.ReleasedAt,
		})
	}

	return updated, true, nil
}

func (s *Service) UpdateSessionState(ctx context.Context, params UpdateSessionStateParams) (Record, error) {
	if s == nil || s.sessions == nil {
		return Record{}, fmt.Errorf("single tab session store is not configured")
	}
	if strings.TrimSpace(params.SingleTabSessionID) == "" {
		return Record{}, fmt.Errorf("session_id is required")
	}

	record, err := s.sessions.GetSessionByID(ctx, params.SingleTabSessionID)
	if err != nil {
		return Record{}, err
	}

	updatedAt := params.LastSeenAt
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	status := firstNonEmpty(params.Status, record.Status)
	currentURL := firstNonEmpty(params.CurrentURL, record.CurrentURL)
	currentTitle := firstNonEmpty(params.CurrentTitle, record.CurrentTitle)
	statusReason := firstNonEmpty(params.StatusReason, record.StatusReason)
	lastSeenAt := updatedAt

	return s.sessions.UpdateSessionStatus(ctx, UpdateStatusParams{
		SingleTabSessionID: record.SingleTabSessionID,
		Status:             status,
		StatusReason:       statusReason,
		SelectedVia:        record.SelectedVia,
		SelectedBy:         record.SelectedBy,
		CurrentURL:         currentURL,
		CurrentTitle:       currentTitle,
		BrowserInstanceID:  firstNonEmpty(params.BrowserInstanceID, record.BrowserInstanceID),
		HostID:             firstNonEmpty(params.HostID, record.HostID),
		LastSeenAt:         &lastSeenAt,
		ActivatedAt:        record.ActivatedAt,
		ReleasedAt:         params.ReleasedAt,
		ExpiresAt:          record.ExpiresAt,
		UpdatedAt:          updatedAt,
	})
}

type ActivateFromApprovalParams struct {
	ApprovalID        string
	CandidateToken    string
	ResolvedVia       string
	ResolvedBy        string
	ActorType         string
	ActorID           string
	CurrentURL        string
	CurrentTitle      string
	BrowserInstanceID string
	HostID            string
	ResolvedAt        time.Time
}

type ActivationResult struct {
	Approval  approvals.Record
	Candidate approvals.TabCandidate
	Session   Record
	Changed   bool
}

func (s *Service) ActivateFromApproval(ctx context.Context, params ActivateFromApprovalParams) (ActivationResult, error) {
	if s == nil || s.approvals == nil || s.sessions == nil {
		return ActivationResult{}, fmt.Errorf("single tab bind service is not configured")
	}
	if strings.TrimSpace(params.ApprovalID) == "" {
		return ActivationResult{}, fmt.Errorf("approval_id is required")
	}
	if strings.TrimSpace(params.CandidateToken) == "" {
		return ActivationResult{}, fmt.Errorf("candidate_token is required")
	}
	if params.ResolvedAt.IsZero() {
		params.ResolvedAt = time.Now().UTC()
	}
	if strings.TrimSpace(params.ResolvedVia) == "" {
		params.ResolvedVia = approvals.ResolvedViaWeb
	}
	if strings.TrimSpace(params.ActorType) == "" {
		params.ActorType = "web"
	}
	if strings.TrimSpace(params.ActorID) == "" {
		params.ActorID = params.ResolvedBy
	}

	approvalRecord, err := s.approvals.GetApprovalByID(ctx, params.ApprovalID)
	if err != nil {
		return ActivationResult{}, err
	}
	if approvalRecord.ApprovalType != approvals.ApprovalTypeBrowserTabSelection {
		return ActivationResult{}, fmt.Errorf("approval %s is not a browser tab selection", params.ApprovalID)
	}

	if approvalRecord.Status == approvals.StatusApproved {
		existing, err := s.sessions.GetSessionByApprovalID(ctx, approvalRecord.ApprovalID)
		if err == nil {
			selected, selErr := s.selectedCandidate(ctx, approvalRecord.ApprovalID, params.CandidateToken)
			if selErr != nil {
				return ActivationResult{}, selErr
			}
			return ActivationResult{Approval: approvalRecord, Candidate: selected, Session: existing, Changed: false}, nil
		}
		if err != ErrSessionNotFound {
			return ActivationResult{}, err
		}
	}
	if approvalRecord.Status != approvals.StatusPending && approvalRecord.Status != approvals.StatusApproved {
		return ActivationResult{}, fmt.Errorf("approval %s is not pending", params.ApprovalID)
	}

	selectedCandidate, err := s.approvals.SelectTabCandidate(ctx, approvalRecord.ApprovalID, params.CandidateToken, params.ResolvedAt)
	if err != nil {
		return ActivationResult{}, err
	}

	if approvalRecord.Status == approvals.StatusPending {
		updated, resolveErr := s.approvals.ResolveApproval(ctx, approvals.ResolveParams{
			ApprovalID:       approvalRecord.ApprovalID,
			ExpectedStatus:   approvals.StatusPending,
			Status:           approvals.StatusApproved,
			ResolvedVia:      params.ResolvedVia,
			ResolvedBy:       params.ResolvedBy,
			ResolutionReason: "browser tab selected by user",
			ResolvedAt:       params.ResolvedAt,
		})
		if resolveErr != nil {
			if resolveErr == approvals.ErrApprovalStatusConflict {
				reloaded, getErr := s.approvals.GetApprovalByID(ctx, approvalRecord.ApprovalID)
				if getErr != nil {
					return ActivationResult{}, getErr
				}
				approvalRecord = reloaded
			} else {
				return ActivationResult{}, resolveErr
			}
		} else {
			approvalRecord = updated
		}
	}

	sessionRecord, err := s.sessions.CreateSession(ctx, CreateParams{
		SingleTabSessionID: newSingleTabSessionID(),
		SessionKey:         approvalRecord.SessionKey,
		CreatedByRunID:     approvalRecord.RunID,
		ApprovalID:         approvalRecord.ApprovalID,
		Status:             StatusActive,
		BoundTabRef:        selectedCandidate.InternalTabRef,
		BrowserInstanceID:  firstNonEmpty(params.BrowserInstanceID, approvalRecord.ToolCallID),
		HostID:             params.HostID,
		SelectedVia:        params.ResolvedVia,
		SelectedBy:         params.ResolvedBy,
		CurrentURL:         firstNonEmpty(params.CurrentURL, selectedCandidate.CurrentURL),
		CurrentTitle:       firstNonEmpty(params.CurrentTitle, selectedCandidate.Title),
		StatusReason:       "tab selected and activated",
		CreatedAt:          params.ResolvedAt,
		ActivatedAt:        &params.ResolvedAt,
	})
	if err != nil {
		if err == ErrActiveSessionBusy {
			existing, getErr := s.sessions.GetSessionByApprovalID(ctx, approvalRecord.ApprovalID)
			if getErr != nil {
				return ActivationResult{}, err
			}
			return ActivationResult{Approval: approvalRecord, Candidate: selectedCandidate, Session: existing, Changed: false}, nil
		}
		return ActivationResult{}, err
	}

	_ = s.approvals.InsertEvent(ctx, approvals.Event{
		ApprovalID:   approvalRecord.ApprovalID,
		RunID:        approvalRecord.RunID,
		SessionKey:   approvalRecord.SessionKey,
		EventType:    "browser_tab_selected",
		StatusBefore: approvals.StatusPending,
		StatusAfter:  approvals.StatusApproved,
		ActorType:    params.ActorType,
		ActorID:      params.ActorID,
		Reason:       "browser tab selected by user",
		MetadataJSON: fmt.Sprintf(`{"candidate_token":%q,"single_tab_session_id":%q}`, selectedCandidate.CandidateToken, sessionRecord.SingleTabSessionID),
		CreatedAt:    params.ResolvedAt,
	})

	if s.gate != nil && strings.TrimSpace(approvalRecord.ToolCallID) != "" {
		s.gate.Resolve(approvalRecord.ToolCallID, true)
	}

	return ActivationResult{
		Approval:  approvalRecord,
		Candidate: selectedCandidate,
		Session:   sessionRecord,
		Changed:   true,
	}, nil
}

func (s *Service) ActivateFromCandidateToken(ctx context.Context, candidateToken string, params ActivateFromApprovalParams) (ActivationResult, error) {
	if s == nil || s.approvals == nil {
		return ActivationResult{}, fmt.Errorf("single tab bind service is not configured")
	}
	candidateToken = strings.TrimSpace(candidateToken)
	if candidateToken == "" {
		return ActivationResult{}, fmt.Errorf("candidate_token is required")
	}

	candidate, err := s.approvals.GetTabCandidateByToken(ctx, candidateToken)
	if err != nil {
		return ActivationResult{}, err
	}
	if strings.TrimSpace(params.ApprovalID) == "" {
		params.ApprovalID = candidate.ApprovalID
	}
	params.CandidateToken = candidateToken
	return s.ActivateFromApproval(ctx, params)
}

func (s *Service) selectedCandidate(ctx context.Context, approvalID, candidateToken string) (approvals.TabCandidate, error) {
	candidates, err := s.approvals.ListTabCandidates(ctx, approvalID)
	if err != nil {
		return approvals.TabCandidate{}, err
	}
	for _, candidate := range candidates {
		if candidate.CandidateToken == candidateToken || candidate.Status == "selected" {
			return candidate, nil
		}
	}
	return approvals.TabCandidate{}, approvals.ErrTabCandidateNotFound
}

func newSingleTabSessionID() string {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("single-tab-%d", time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("single-tab-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(raw[:]))
}

func newBindToolCallID() string {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("single-tab-bind-%d", time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("single-tab-bind-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(raw[:]))
}

func newCandidateToken() string {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("tabtok-%d", time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("tabtok-%s", hex.EncodeToString(raw[:]))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
