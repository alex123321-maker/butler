package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
)

type approvalsStore interface {
	ListApprovals(ctx context.Context, status, runID, sessionKey string, limit, offset int) ([]approvals.Record, error)
	GetApprovalByID(ctx context.Context, approvalID string) (approvals.Record, error)
}

type approvalsResolver interface {
	ResolveByToolCall(ctx context.Context, params approvals.ResolveByToolCallParams) (approvals.Record, bool, error)
}

type ApprovalsServer struct {
	store    approvalsStore
	resolver approvalsResolver
}

func NewApprovalsServer(store approvalsStore, resolver approvalsResolver) *ApprovalsServer {
	return &ApprovalsServer{store: store, resolver: resolver}
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
			result = append(result, toApprovalDTO(item))
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
		writeJSON(w, http.StatusOK, map[string]any{"approval": toApprovalDTO(rec)})
	})
}

func (s *ApprovalsServer) HandleApprove(prefix string) http.Handler {
	return s.handleResolve(prefix, "/approve", true)
}

func (s *ApprovalsServer) HandleReject(prefix string) http.Handler {
	return s.handleResolve(prefix, "/reject", false)
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
		writeJSON(w, statusCode, map[string]any{"approval": toApprovalDTO(resolved), "changed": changed})
	})
}

type approvalDTO struct {
	ApprovalID       string  `json:"approval_id"`
	RunID            string  `json:"run_id"`
	SessionKey       string  `json:"session_key"`
	ToolCallID       string  `json:"tool_call_id"`
	Status           string  `json:"status"`
	RequestedVia     string  `json:"requested_via"`
	ResolvedVia      string  `json:"resolved_via"`
	ToolName         string  `json:"tool_name"`
	ArgsJSON         string  `json:"args_json"`
	RiskLevel        string  `json:"risk_level"`
	Summary          string  `json:"summary"`
	DetailsJSON      string  `json:"details_json"`
	RequestedAt      string  `json:"requested_at"`
	ResolvedAt       *string `json:"resolved_at"`
	ResolvedBy       string  `json:"resolved_by"`
	ResolutionReason string  `json:"resolution_reason"`
	ExpiresAt        *string `json:"expires_at"`
	UpdatedAt        string  `json:"updated_at"`
}

func toApprovalDTO(record approvals.Record) approvalDTO {
	item := approvalDTO{
		ApprovalID:       record.ApprovalID,
		RunID:            record.RunID,
		SessionKey:       record.SessionKey,
		ToolCallID:       record.ToolCallID,
		Status:           record.Status,
		RequestedVia:     record.RequestedVia,
		ResolvedVia:      record.ResolvedVia,
		ToolName:         record.ToolName,
		ArgsJSON:         record.ArgsJSON,
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
	return item
}

func webResolutionReason(approved bool) string {
	if approved {
		return "approved in web ui"
	}
	return "rejected in web ui"
}
