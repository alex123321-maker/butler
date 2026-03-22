package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	singletab "github.com/butler/butler/apps/orchestrator/internal/singletab"
)

const defaultRelayHeartbeatTTL = 90 * time.Second

type singleTabCoordinator interface {
	CreateBindRequest(ctx context.Context, params singletab.CreateBindRequestParams) (singletab.CreateBindRequestResult, error)
	GetActiveSession(ctx context.Context, sessionKey string) (singletab.Record, error)
	GetSession(ctx context.Context, sessionID string) (singletab.Record, error)
	ReleaseSession(ctx context.Context, params singletab.ReleaseSessionParams) (singletab.Record, bool, error)
	UpdateSessionState(ctx context.Context, params singletab.UpdateSessionStateParams) (singletab.Record, error)
}

type SingleTabServer struct {
	coordinator       singleTabCoordinator
	relay             *ExtensionActionRelay
	relayHeartbeatTTL time.Duration
}

func NewSingleTabServer(coordinator singleTabCoordinator) *SingleTabServer {
	return &SingleTabServer{
		coordinator:       coordinator,
		relay:             NewExtensionActionRelay(),
		relayHeartbeatTTL: defaultRelayHeartbeatTTL,
	}
}

func (s *SingleTabServer) SetRelayHeartbeatTTL(ttl time.Duration) {
	if s == nil {
		return
	}
	if ttl <= 0 {
		s.relayHeartbeatTTL = 0
		return
	}
	s.relayHeartbeatTTL = ttl
}

func (s *SingleTabServer) HandleCreateBindRequest() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.coordinator == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "single tab coordinator is not configured"})
			return
		}

		var request createBindRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
			return
		}
		if strings.TrimSpace(request.RunID) == "" || strings.TrimSpace(request.SessionKey) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "run_id and session_key are required"})
			return
		}
		if len(request.TabCandidates) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tab_candidates are required"})
			return
		}

		candidates := make([]singletab.BindCandidateInput, 0, len(request.TabCandidates))
		for _, candidate := range request.TabCandidates {
			candidates = append(candidates, singletab.BindCandidateInput{
				InternalTabRef: candidate.InternalTabRef,
				Title:          candidate.Title,
				Domain:         candidate.Domain,
				CurrentURL:     candidate.CurrentURL,
				FaviconURL:     candidate.FaviconURL,
				DisplayLabel:   candidate.DisplayLabel,
			})
		}

		result, err := s.coordinator.CreateBindRequest(r.Context(), singletab.CreateBindRequestParams{
			RunID:         request.RunID,
			SessionKey:    request.SessionKey,
			ToolCallID:    request.ToolCallID,
			RequestedVia:  request.RequestedVia,
			Candidates:    candidates,
			BrowserHint:   request.BrowserHint,
			RequestSource: request.RequestSource,
		})
		if err != nil {
			switch err {
			case singletab.ErrActiveSessionBusy:
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create bind request"})
				return
			}
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"approval": toApprovalDTO(result.Approval, result.Candidates),
		})
	})
}

func (s *SingleTabServer) HandleGetActiveSession() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.coordinator == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "single tab coordinator is not configured"})
			return
		}
		sessionKey := strings.TrimSpace(r.URL.Query().Get("session_key"))
		if sessionKey == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_key is required"})
			return
		}

		record, err := s.coordinator.GetActiveSession(r.Context(), sessionKey)
		if err != nil {
			switch err {
			case singletab.ErrSessionNotFound:
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "single tab session not found"})
				return
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get single tab session"})
				return
			}
		}
		browserInstanceID, hostID := extensionIdentityFromRequest(r)
		if isExtensionRelayPath(r.URL.Path) && browserInstanceID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "browser_instance_id is required", "code": "invalid_request"})
			return
		}
		if browserInstanceID != "" {
			record, err = s.claimOrValidateBrowserOwnership(r.Context(), record, browserInstanceID, hostID, true)
			if err != nil {
				relayErr := &ExtensionRelayError{Code: "action_not_allowed", Message: err.Error()}
				if errors.As(err, &relayErr) {
					writeJSON(w, mapRelayErrorStatus(relayErr.Code), map[string]string{"error": relayErr.Message, "code": relayErr.Code})
					return
				}
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update single tab session owner", "code": "session_update_failed"})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"single_tab_session": toSingleTabSessionDTO(record)})
	})
}

func (s *SingleTabServer) HandleGetSession(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.coordinator == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "single tab coordinator is not configured"})
			return
		}
		sessionID := extractPathParam(r.URL.Path, prefix)
		if sessionID == "" || strings.Contains(sessionID, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id is required"})
			return
		}

		record, err := s.coordinator.GetSession(r.Context(), sessionID)
		if err != nil {
			switch err {
			case singletab.ErrSessionNotFound:
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "single tab session not found"})
				return
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get single tab session"})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"single_tab_session": toSingleTabSessionDTO(record)})
	})
}

func (s *SingleTabServer) HandleReleaseSession(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.coordinator == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "single tab coordinator is not configured"})
			return
		}
		sessionID := extractPathParam(r.URL.Path, prefix)
		sessionID = strings.TrimSuffix(sessionID, "/release")
		if sessionID == "" || strings.Contains(sessionID, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id is required"})
			return
		}

		actorID := "web"
		if user := strings.TrimSpace(r.Header.Get("X-Butler-Actor")); user != "" {
			actorID = user
		}
		record, changed, err := s.coordinator.ReleaseSession(r.Context(), singletab.ReleaseSessionParams{
			SingleTabSessionID: sessionID,
			ReleasedBy:         actorID,
			ReleasedVia:        "web",
			ActorType:          "web",
			ActorID:            actorID,
		})
		if err != nil {
			switch err {
			case singletab.ErrSessionNotFound:
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "single tab session not found"})
				return
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to release single tab session"})
				return
			}
		}
		statusCode := http.StatusOK
		if !changed {
			statusCode = http.StatusConflict
		}
		writeJSON(w, statusCode, map[string]any{
			"single_tab_session": toSingleTabSessionDTO(record),
			"changed":            changed,
		})
	})
}

func (s *SingleTabServer) HandleUpdateSessionState(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.coordinator == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "single tab coordinator is not configured"})
			return
		}
		sessionID := extractPathParam(r.URL.Path, prefix)
		sessionID = strings.TrimSuffix(sessionID, "/state")
		if sessionID == "" || strings.Contains(sessionID, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session id is required"})
			return
		}

		var request updateSessionStateRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body"})
			return
		}

		params := singletab.UpdateSessionStateParams{
			SingleTabSessionID: sessionID,
			Status:             request.Status,
			StatusReason:       request.StatusReason,
			CurrentURL:         request.CurrentURL,
			CurrentTitle:       request.CurrentTitle,
			BrowserInstanceID:  request.BrowserInstanceID,
			HostID:             request.HostID,
		}
		if strings.TrimSpace(request.LastSeenAt) != "" {
			lastSeenAt, err := time.Parse(time.RFC3339, request.LastSeenAt)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "last_seen_at must be RFC3339"})
				return
			}
			params.LastSeenAt = lastSeenAt
		}
		if strings.TrimSpace(request.ReleasedAt) != "" {
			releasedAt, err := time.Parse(time.RFC3339, request.ReleasedAt)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "released_at must be RFC3339"})
				return
			}
			params.ReleasedAt = &releasedAt
		}

		record, err := s.coordinator.UpdateSessionState(r.Context(), params)
		if err != nil {
			switch err {
			case singletab.ErrSessionNotFound:
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "single tab session not found"})
				return
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update single tab session"})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"single_tab_session": toSingleTabSessionDTO(record)})
	})
}

func (s *SingleTabServer) HandleRelayDispatchAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.coordinator == nil || s.relay == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "single tab dispatch relay is not configured"})
			return
		}

		var request relayDispatchActionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body", "code": "invalid_request"})
			return
		}
		if strings.TrimSpace(request.SingleTabSessionID) == "" || strings.TrimSpace(request.BoundTabRef) == "" || strings.TrimSpace(request.ActionType) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "single_tab_session_id, bound_tab_ref, and action_type are required", "code": "invalid_request"})
			return
		}

		session, err := s.coordinator.GetSession(r.Context(), request.SingleTabSessionID)
		if err != nil {
			if err == singletab.ErrSessionNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "single tab session not found", "code": "session_not_found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get single tab session", "code": "session_lookup_failed"})
			return
		}
		if session.Status != singletab.StatusActive {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "single-tab session is not active", "code": "session_not_active"})
			return
		}
		if strings.TrimSpace(session.BoundTabRef) != strings.TrimSpace(request.BoundTabRef) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bound tab mismatch for single-tab session", "code": "action_not_allowed"})
			return
		}
		if s.isRelayHeartbeatStale(session, time.Now().UTC()) {
			_, _ = s.coordinator.UpdateSessionState(r.Context(), singletab.UpdateSessionStateParams{
				SingleTabSessionID: session.SingleTabSessionID,
				Status:             singletab.StatusHostDisconnected,
				StatusReason:       "extension heartbeat timed out",
				CurrentURL:         session.CurrentURL,
				CurrentTitle:       session.CurrentTitle,
				BrowserInstanceID:  session.BrowserInstanceID,
				HostID:             session.HostID,
				LastSeenAt:         time.Now().UTC(),
			})
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "extension heartbeat timed out",
				"code":  "host_unavailable",
			})
			return
		}

		result, err := s.relay.Dispatch(r.Context(), session.SessionKey, ExtensionDispatchParams{
			SingleTabSessionID: request.SingleTabSessionID,
			BoundTabRef:        request.BoundTabRef,
			ActionType:         request.ActionType,
			ArgsJSON:           request.ArgsJSON,
			BrowserInstanceID:  strictBrowserInstanceID(session.BrowserInstanceID),
		})
		if err != nil {
			relayErr := &ExtensionRelayError{Code: "host_unavailable", Message: "extension relay dispatch failed"}
			if errors.As(err, &relayErr) {
				writeJSON(w, mapRelayErrorStatus(relayErr.Code), map[string]string{"error": relayErr.Message, "code": relayErr.Code})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "extension relay dispatch failed", "code": "dispatch_failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"action": result})
	})
}

func (s *SingleTabServer) HandleExtensionPollNextAction() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.coordinator == nil || s.relay == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "single tab extension relay is not configured"})
			return
		}

		sessionKey := strings.TrimSpace(r.URL.Query().Get("session_key"))
		if sessionKey == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "session_key is required", "code": "invalid_request"})
			return
		}
		record, err := s.coordinator.GetActiveSession(r.Context(), sessionKey)
		if err != nil {
			if err == singletab.ErrSessionNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "single tab session not found", "code": "session_not_found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get single tab session", "code": "session_lookup_failed"})
			return
		}
		browserInstanceID, hostID := extensionIdentityFromRequest(r)
		if browserInstanceID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "browser_instance_id is required", "code": "invalid_request"})
			return
		}
		record, err = s.claimOrValidateBrowserOwnership(r.Context(), record, browserInstanceID, hostID, true)
		if err != nil {
			relayErr := &ExtensionRelayError{Code: "action_not_allowed", Message: err.Error()}
			if errors.As(err, &relayErr) {
				writeJSON(w, mapRelayErrorStatus(relayErr.Code), map[string]string{"error": relayErr.Message, "code": relayErr.Code})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update single tab session owner", "code": "session_update_failed"})
			return
		}

		timeout := parsePollTimeout(r.URL.Query().Get("timeout_ms"))
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		pending, ok, err := s.relay.PollNext(ctx, sessionKey)
		if err != nil {
			relayErr := &ExtensionRelayError{Code: "host_unavailable", Message: "extension relay poll failed"}
			if errors.As(err, &relayErr) {
				writeJSON(w, mapRelayErrorStatus(relayErr.Code), map[string]string{"error": relayErr.Message, "code": relayErr.Code})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "extension relay poll failed", "code": "poll_failed"})
			return
		}
		if !ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"dispatch": pending})
	})
}

func (s *SingleTabServer) HandleExtensionResolveAction(prefix string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if s.relay == nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "single tab extension relay is not configured"})
			return
		}

		dispatchID := extractPathParam(r.URL.Path, prefix)
		dispatchID = strings.TrimSuffix(dispatchID, "/result")
		if dispatchID == "" || strings.Contains(dispatchID, "/") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dispatch id is required", "code": "invalid_request"})
			return
		}
		browserInstanceID, _ := extensionIdentityFromRequest(r)
		if browserInstanceID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "browser_instance_id is required", "code": "invalid_request"})
			return
		}

		var request extensionResolveActionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request body", "code": "invalid_request"})
			return
		}

		var resolveErr error
		if request.OK {
			resolveErr = s.relay.ResolveSuccess(dispatchID, browserInstanceID, request.Result)
		} else {
			errCode := "runtime_error"
			errMessage := "extension action failed"
			if request.Error != nil {
				if strings.TrimSpace(request.Error.Code) != "" {
					errCode = strings.TrimSpace(request.Error.Code)
				}
				if strings.TrimSpace(request.Error.Message) != "" {
					errMessage = strings.TrimSpace(request.Error.Message)
				}
			}
			resolveErr = s.relay.ResolveError(dispatchID, browserInstanceID, errCode, errMessage)
		}
		if resolveErr != nil {
			if resolveErr == ErrExtensionDispatchNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "dispatch not found", "code": "dispatch_not_found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve dispatch", "code": "dispatch_resolve_failed"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"accepted": true})
	})
}

type createBindRequest struct {
	RunID         string                     `json:"run_id"`
	SessionKey    string                     `json:"session_key"`
	ToolCallID    string                     `json:"tool_call_id"`
	RequestedVia  string                     `json:"requested_via"`
	BrowserHint   string                     `json:"browser_hint"`
	RequestSource string                     `json:"request_source"`
	TabCandidates []createBindCandidateEntry `json:"tab_candidates"`
}

type createBindCandidateEntry struct {
	InternalTabRef string `json:"internal_tab_ref"`
	Title          string `json:"title"`
	Domain         string `json:"domain"`
	CurrentURL     string `json:"current_url"`
	FaviconURL     string `json:"favicon_url"`
	DisplayLabel   string `json:"display_label"`
}

type updateSessionStateRequest struct {
	Status            string `json:"status"`
	StatusReason      string `json:"status_reason"`
	CurrentURL        string `json:"current_url"`
	CurrentTitle      string `json:"current_title"`
	BrowserInstanceID string `json:"browser_instance_id"`
	HostID            string `json:"host_id"`
	LastSeenAt        string `json:"last_seen_at"`
	ReleasedAt        string `json:"released_at"`
}

type relayDispatchActionRequest struct {
	SingleTabSessionID string `json:"single_tab_session_id"`
	BoundTabRef        string `json:"bound_tab_ref"`
	ActionType         string `json:"action_type"`
	ArgsJSON           string `json:"args_json,omitempty"`
}

type extensionResolveActionRequest struct {
	OK     bool                         `json:"ok"`
	Result map[string]any               `json:"result,omitempty"`
	Error  *extensionResolveActionError `json:"error,omitempty"`
}

type extensionResolveActionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func parsePollTimeout(raw string) time.Duration {
	timeoutMS, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || timeoutMS <= 0 {
		timeoutMS = 25000
	}
	if timeoutMS < 1000 {
		timeoutMS = 1000
	}
	if timeoutMS > 55000 {
		timeoutMS = 55000
	}
	return time.Duration(timeoutMS) * time.Millisecond
}

func extensionIdentityFromRequest(r *http.Request) (string, string) {
	if r == nil {
		return "", ""
	}
	browserInstanceID := strings.TrimSpace(r.URL.Query().Get("browser_instance_id"))
	if browserInstanceID == "" {
		browserInstanceID = strings.TrimSpace(r.Header.Get("X-Butler-Browser-Instance"))
	}
	hostID := strings.TrimSpace(r.URL.Query().Get("host_id"))
	if hostID == "" {
		hostID = strings.TrimSpace(r.Header.Get("X-Butler-Host"))
	}
	return browserInstanceID, hostID
}

func isExtensionRelayPath(path string) bool {
	return strings.HasPrefix(strings.TrimSpace(path), "/api/v2/extension/")
}

func isProvisionalBrowserInstanceID(value string) bool {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return true
	}
	return strings.HasPrefix(normalized, "single-tab-bind-")
}

func strictBrowserInstanceID(value string) string {
	if isProvisionalBrowserInstanceID(value) {
		return ""
	}
	return strings.TrimSpace(value)
}

func (s *SingleTabServer) claimOrValidateBrowserOwnership(ctx context.Context, record singletab.Record, browserInstanceID, hostID string, touchHeartbeat bool) (singletab.Record, error) {
	requested := strings.TrimSpace(browserInstanceID)
	if requested == "" {
		return record, nil
	}
	existing := strings.TrimSpace(record.BrowserInstanceID)
	if existing != "" && !isProvisionalBrowserInstanceID(existing) && existing != requested {
		return singletab.Record{}, &ExtensionRelayError{Code: "action_not_allowed", Message: "single-tab session is bound to another browser instance"}
	}

	params := singletab.UpdateSessionStateParams{
		SingleTabSessionID: record.SingleTabSessionID,
		Status:             record.Status,
		StatusReason:       record.StatusReason,
		CurrentURL:         record.CurrentURL,
		CurrentTitle:       record.CurrentTitle,
		BrowserInstanceID:  requested,
		HostID:             firstNonEmptyAPI(hostID, record.HostID),
	}
	if touchHeartbeat {
		params.LastSeenAt = time.Now().UTC()
	}
	updated, err := s.coordinator.UpdateSessionState(ctx, params)
	if err != nil {
		return singletab.Record{}, err
	}
	return updated, nil
}

func firstNonEmptyAPI(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func (s *SingleTabServer) isRelayHeartbeatStale(record singletab.Record, now time.Time) bool {
	if s == nil || s.relayHeartbeatTTL <= 0 {
		return false
	}
	if strictBrowserInstanceID(record.BrowserInstanceID) == "" {
		return false
	}
	if record.LastSeenAt == nil {
		return true
	}
	heartbeatAge := now.Sub(record.LastSeenAt.UTC())
	return heartbeatAge > s.relayHeartbeatTTL
}

func mapRelayErrorStatus(code string) int {
	switch strings.TrimSpace(code) {
	case "invalid_request", "selector_not_found", "action_not_allowed":
		return http.StatusBadRequest
	case "session_not_active", "tab_closed":
		return http.StatusConflict
	case "host_unavailable":
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}
