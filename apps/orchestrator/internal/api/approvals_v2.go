package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	singletab "github.com/butler/butler/apps/orchestrator/internal/singletab"
)

type approvalsStore interface {
	ListApprovals(ctx context.Context, status, runID, sessionKey string, limit, offset int) ([]approvals.Record, error)
	GetApprovalByID(ctx context.Context, approvalID string) (approvals.Record, error)
	ListTabCandidates(ctx context.Context, approvalID string) ([]approvals.TabCandidate, error)
}

type approvalsResolver interface {
	ResolveByToolCall(ctx context.Context, params approvals.ResolveByToolCallParams) (approvals.Record, bool, error)
}

type approvalsSelector interface {
	ActivateFromApproval(ctx context.Context, params singletab.ActivateFromApprovalParams) (singletab.ActivationResult, error)
}

type ApprovalsServer struct {
	store    approvalsStore
	resolver approvalsResolver
	selector approvalsSelector
}

func NewApprovalsServer(store approvalsStore, resolver approvalsResolver, selector approvalsSelector) *ApprovalsServer {
	return &ApprovalsServer{store: store, resolver: resolver, selector: selector}
}

func (s *ApprovalsServer) HandleListApprovals() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approvals store is not configured"})
			return
		}

		query := r.URL.Query()
		status := strings.TrimSpace(strings.ToLower(query.Get("status")))
		runID := strings.TrimSpace(query.Get("run_id"))
		sessionKey := strings.TrimSpace(query.Get("session_key"))
		limit := 50
		offset := 0
		if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be integer"})
				return
			}
			limit = value
		}
		if raw := strings.TrimSpace(query.Get("offset")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "offset must be integer"})
				return
			}
			offset = value
		}

		items, err := s.store.ListApprovals(r.Context(), status, runID, sessionKey, limit, offset)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list approvals"})
			return
		}
		result := make([]approvalDTO, 0, len(items))
		for _, item := range items {
			result = append(result, toApprovalDTO(item, nil))
		}
		writeJSON(w, http.StatusOK, map[string]any{"approvals": result})
	})
}

func (s *ApprovalsServer) HandleGetApproval(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.store == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approvals store is not configured"})
			return
		}
		approvalID := extractPathParam(r.URL.Path, prefix)
		if approvalID == "" || strings.Contains(approvalID, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval id is required"})
			return
		}
		rec, err := s.store.GetApprovalByID(r.Context(), approvalID)
		if err != nil {
			if err == approvals.ErrApprovalNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get approval"})
			return
		}
		candidates, err := s.store.ListTabCandidates(r.Context(), approvalID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get approval tab candidates"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"approval": toApprovalDTO(rec, candidates)})
	})
}

func (s *ApprovalsServer) HandleApprove(prefix string) http.Handler {
	return s.handleResolve(prefix, "/approve", true)
}

func (s *ApprovalsServer) HandleReject(prefix string) http.Handler {
	return s.handleResolve(prefix, "/reject", false)
}

func (s *ApprovalsServer) HandleSelectTab(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.store == nil || s.selector == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "tab selection resolver is not configured"})
			return
		}

		approvalID := extractPathParam(r.URL.Path, prefix)
		approvalID = strings.TrimSuffix(approvalID, "/select-tab")
		if approvalID == "" || strings.Contains(approvalID, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval id is required"})
			return
		}

		var request selectTabRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
			return
		}
		if strings.TrimSpace(request.CandidateToken) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "candidate_token is required"})
			return
		}

		actorID := "web"
		if user := strings.TrimSpace(r.Header.Get("X-Butler-Actor")); user != "" {
			actorID = user
		}

		result, err := s.selector.ActivateFromApproval(r.Context(), singletab.ActivateFromApprovalParams{
			ApprovalID:        approvalID,
			CandidateToken:    request.CandidateToken,
			ResolvedVia:       approvals.ResolvedViaWeb,
			ResolvedBy:        actorID,
			ActorType:         "web",
			ActorID:           actorID,
			CurrentURL:        request.CurrentURL,
			CurrentTitle:      request.CurrentTitle,
			BrowserInstanceID: request.BrowserInstanceID,
			HostID:            request.HostID,
			ResolvedAt:        time.Now().UTC(),
		})
		if err != nil {
			switch err {
			case approvals.ErrApprovalNotFound, approvals.ErrTabCandidateNotFound:
				writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
				return
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to select browser tab"})
				return
			}
		}

		candidates, listErr := s.store.ListTabCandidates(r.Context(), approvalID)
		if listErr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get approval tab candidates"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"approval":           toApprovalDTO(result.Approval, candidates),
			"single_tab_session": toSingleTabSessionDTO(result.Session),
			"changed":            result.Changed,
		})
	})
}

func (s *ApprovalsServer) handleResolve(prefix, actionSuffix string, approved bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.store == nil || s.resolver == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "approvals resolver is not configured"})
			return
		}

		approvalID := extractPathParam(r.URL.Path, prefix)
		approvalID = strings.TrimSuffix(approvalID, actionSuffix)
		if approvalID == "" || strings.Contains(approvalID, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "approval id is required"})
			return
		}

		record, err := s.store.GetApprovalByID(r.Context(), approvalID)
		if err != nil {
			if err == approvals.ErrApprovalNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load approval"})
			return
		}

		actorID := "web"
		if user := strings.TrimSpace(r.Header.Get("X-Butler-Actor")); user != "" {
			actorID = user
		}

		resolved, changed, resolveErr := s.resolver.ResolveByToolCall(r.Context(), approvals.ResolveByToolCallParams{
			ToolCallID:       record.ToolCallID,
			Approved:         approved,
			ResolvedVia:      approvals.ResolvedViaWeb,
			ResolvedBy:       actorID,
			ResolutionReason: webResolutionReason(approved),
			ResolvedAt:       time.Now().UTC(),
			ActorType:        "web",
			ActorID:          actorID,
		})
		if resolveErr != nil {
			if resolveErr == approvals.ErrApprovalNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "approval not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve approval"})
			return
		}

		statusCode := http.StatusOK
		if !changed {
			statusCode = http.StatusConflict
		}
		candidates, err := s.store.ListTabCandidates(r.Context(), approvalID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get approval tab candidates"})
			return
		}
		writeJSON(w, statusCode, map[string]any{"approval": toApprovalDTO(resolved, candidates), "changed": changed})
	})
}

type selectTabRequest struct {
	CandidateToken    string `json:"candidate_token"`
	BrowserInstanceID string `json:"browser_instance_id"`
	HostID            string `json:"host_id"`
	CurrentURL        string `json:"current_url"`
	CurrentTitle      string `json:"current_title"`
}

type approvalDTO struct {
	ApprovalID       string                    `json:"approval_id"`
	RunID            string                    `json:"run_id"`
	SessionKey       string                    `json:"session_key"`
	ToolCallID       string                    `json:"tool_call_id"`
	ApprovalType     string                    `json:"approval_type"`
	Status           string                    `json:"status"`
	RequestedVia     string                    `json:"requested_via"`
	ResolvedVia      string                    `json:"resolved_via"`
	ToolName         string                    `json:"tool_name"`
	ArgsJSON         string                    `json:"args_json"`
	PayloadJSON      string                    `json:"payload_json"`
	RiskLevel        string                    `json:"risk_level"`
	Summary          string                    `json:"summary"`
	DetailsJSON      string                    `json:"details_json"`
	RequestedAt      string                    `json:"requested_at"`
	ResolvedAt       *string                   `json:"resolved_at"`
	ResolvedBy       string                    `json:"resolved_by"`
	ResolutionReason string                    `json:"resolution_reason"`
	ExpiresAt        *string                   `json:"expires_at"`
	UpdatedAt        string                    `json:"updated_at"`
	TabCandidates    []approvalTabCandidateDTO `json:"tab_candidates,omitempty"`
}

type singleTabSessionDTO struct {
	SingleTabSessionID string  `json:"single_tab_session_id"`
	SessionKey         string  `json:"session_key"`
	ApprovalID         string  `json:"approval_id"`
	Status             string  `json:"status"`
	BoundTabRef        string  `json:"bound_tab_ref"`
	CurrentURL         string  `json:"current_url"`
	CurrentTitle       string  `json:"current_title"`
	SelectedVia        string  `json:"selected_via"`
	SelectedBy         string  `json:"selected_by"`
	ActivatedAt        *string `json:"activated_at,omitempty"`
	UpdatedAt          string  `json:"updated_at"`
}

type approvalTabCandidateDTO struct {
	CandidateToken string  `json:"candidate_token"`
	Title          string  `json:"title"`
	Domain         string  `json:"domain"`
	CurrentURL     string  `json:"current_url"`
	FaviconURL     string  `json:"favicon_url"`
	DisplayLabel   string  `json:"display_label"`
	Status         string  `json:"status"`
	SelectedAt     *string `json:"selected_at,omitempty"`
}

func toApprovalDTO(record approvals.Record, candidates []approvals.TabCandidate) approvalDTO {
	item := approvalDTO{
		ApprovalID:       record.ApprovalID,
		RunID:            record.RunID,
		SessionKey:       record.SessionKey,
		ToolCallID:       record.ToolCallID,
		ApprovalType:     record.ApprovalType,
		Status:           record.Status,
		RequestedVia:     record.RequestedVia,
		ResolvedVia:      record.ResolvedVia,
		ToolName:         record.ToolName,
		ArgsJSON:         record.ArgsJSON,
		PayloadJSON:      record.PayloadJSON,
		RiskLevel:        record.RiskLevel,
		Summary:          record.Summary,
		DetailsJSON:      record.DetailsJSON,
		RequestedAt:      record.RequestedAt.UTC().Format(time.RFC3339),
		ResolvedBy:       record.ResolvedBy,
		ResolutionReason: record.ResolutionReason,
		UpdatedAt:        record.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if record.ResolvedAt != nil {
		value := record.ResolvedAt.UTC().Format(time.RFC3339)
		item.ResolvedAt = &value
	}
	if record.ExpiresAt != nil {
		value := record.ExpiresAt.UTC().Format(time.RFC3339)
		item.ExpiresAt = &value
	}
	if len(candidates) > 0 {
		item.TabCandidates = make([]approvalTabCandidateDTO, 0, len(candidates))
		for _, candidate := range candidates {
			dto := approvalTabCandidateDTO{
				CandidateToken: candidate.CandidateToken,
				Title:          candidate.Title,
				Domain:         candidate.Domain,
				CurrentURL:     candidate.CurrentURL,
				FaviconURL:     candidate.FaviconURL,
				DisplayLabel:   candidate.DisplayLabel,
				Status:         candidate.Status,
			}
			if candidate.SelectedAt != nil {
				value := candidate.SelectedAt.UTC().Format(time.RFC3339)
				dto.SelectedAt = &value
			}
			item.TabCandidates = append(item.TabCandidates, dto)
		}
	}
	return item
}

func toSingleTabSessionDTO(record singletab.Record) singleTabSessionDTO {
	item := singleTabSessionDTO{
		SingleTabSessionID: record.SingleTabSessionID,
		SessionKey:         record.SessionKey,
		ApprovalID:         record.ApprovalID,
		Status:             record.Status,
		BoundTabRef:        record.BoundTabRef,
		CurrentURL:         record.CurrentURL,
		CurrentTitle:       record.CurrentTitle,
		SelectedVia:        record.SelectedVia,
		SelectedBy:         record.SelectedBy,
		UpdatedAt:          record.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if record.ActivatedAt != nil {
		value := record.ActivatedAt.UTC().Format(time.RFC3339)
		item.ActivatedAt = &value
	}
	return item
}

func webResolutionReason(approved bool) string {
	if approved {
		return "approved in web ui"
	}
	return "rejected in web ui"
}
