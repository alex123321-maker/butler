package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/butler/butler/apps/tool-browser-local/internal/client"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/logger"
)

var supportedSingleTabTools = map[string]bool{
	"single_tab.bind":            true,
	"single_tab.status":          true,
	"single_tab.navigate":        true,
	"single_tab.reload":          true,
	"single_tab.go_back":         true,
	"single_tab.go_forward":      true,
	"single_tab.click":           true,
	"single_tab.fill":            true,
	"single_tab.type":            true,
	"single_tab.press_keys":      true,
	"single_tab.scroll":          true,
	"single_tab.wait_for":        true,
	"single_tab.extract_text":    true,
	"single_tab.capture_visible": true,
	"single_tab.release":         true,
}

type sessionReader interface {
	GetSession(ctx context.Context, sessionID string) (client.SessionEnvelope, error)
	GetActiveSession(ctx context.Context, sessionKey string) (client.SessionEnvelope, error)
	GetRun(ctx context.Context, runID string) (client.RunEnvelope, error)
	CreateBindRequest(ctx context.Context, params client.CreateBindRequestParams) (client.CreateBindRequestEnvelope, error)
	ReleaseSession(ctx context.Context, sessionID string) (client.SessionEnvelope, bool, error)
	UpdateSessionState(ctx context.Context, sessionID string, params client.UpdateSessionStateParams) (client.SessionEnvelope, error)
	CreateBrowserCaptureArtifact(ctx context.Context, params client.CreateBrowserCaptureArtifactParams) (client.ArtifactEnvelope, error)
}

type actionDispatcher interface {
	DispatchAction(ctx context.Context, params client.DispatchActionParams) (client.DispatchActionEnvelope, error)
}

type Server struct {
	runtimev1.UnimplementedToolRuntimeServiceServer

	client     sessionReader
	dispatcher actionDispatcher
	log        *slog.Logger
}

type sessionArgs struct {
	SessionID  string `json:"session_id"`
	SessionKey string `json:"session_key,omitempty"`
}

type navigateArgs struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
}

type selectorArgs struct {
	SessionID string `json:"session_id"`
	Selector  string `json:"selector"`
	Value     string `json:"value,omitempty"`
}

type bindArgs struct {
	SessionKey        string `json:"session_key,omitempty"`
	WaitTimeoutMS     int    `json:"wait_timeout_ms,omitempty"`
	BrowserInstanceID string `json:"browser_instance_id,omitempty"`
}

func NewServer(client sessionReader, dispatcher actionDispatcher, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	return &Server{client: client, dispatcher: dispatcher, log: logger.WithComponent(log, "tool-browser-local-runtime")}
}

func (s *Server) Execute(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	call := req.GetToolCall()
	if call == nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "tool_call is required")}, nil
	}
	if !supportedSingleTabTools[call.GetToolName()] {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "unsupported tool for browser-local runtime")}, nil
	}

	switch call.GetToolName() {
	case "single_tab.status":
		return s.executeStatus(ctx, req)
	case "single_tab.release":
		return s.executeRelease(ctx, req)
	case "single_tab.bind":
		return s.executeBind(ctx, req)
	case "single_tab.navigate":
		return s.executeAction(ctx, req, "navigate")
	case "single_tab.reload":
		return s.executeAction(ctx, req, "reload")
	case "single_tab.go_back":
		return s.executeAction(ctx, req, "go_back")
	case "single_tab.go_forward":
		return s.executeAction(ctx, req, "go_forward")
	case "single_tab.click":
		return s.executeAction(ctx, req, "click")
	case "single_tab.fill":
		return s.executeAction(ctx, req, "fill")
	case "single_tab.type":
		return s.executeAction(ctx, req, "type")
	case "single_tab.press_keys":
		return s.executeAction(ctx, req, "press_keys")
	case "single_tab.scroll":
		return s.executeAction(ctx, req, "scroll")
	case "single_tab.wait_for":
		return s.executeAction(ctx, req, "wait_for")
	case "single_tab.extract_text":
		return s.executeAction(ctx, req, "extract_text")
	case "single_tab.capture_visible":
		return s.executeAction(ctx, req, "capture_visible")
	default:
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "unsupported tool for browser-local runtime")}, nil
	}
}

func (s *Server) executeStatus(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	var args sessionArgs
	if err := json.Unmarshal([]byte(req.GetToolCall().GetArgsJson()), &args); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "args_json must decode into single_tab.status args")}, nil
	}
	sessionID, session, err := s.resolveSessionEnvelope(ctx, req.GetToolCall(), args.SessionID, args.SessionKey)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, classifyAPIError(err), err.Error())}, nil
	}
	if session.SingleTabSession != nil && strings.TrimSpace(stringValue(session.SingleTabSession["single_tab_session_id"])) == "" {
		session.SingleTabSession["single_tab_session_id"] = sessionID
	}
	return &runtimev1.ExecuteResponse{Result: completedResult(req, mustJSON(session.SingleTabSession))}, nil
}

func (s *Server) executeRelease(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	var args sessionArgs
	if err := json.Unmarshal([]byte(req.GetToolCall().GetArgsJson()), &args); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "args_json must decode into single_tab.release args")}, nil
	}
	sessionID, _, err := s.resolveSessionEnvelope(ctx, req.GetToolCall(), args.SessionID, args.SessionKey)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, classifyAPIError(err), err.Error())}, nil
	}
	session, changed, err := s.client.ReleaseSession(ctx, sessionID)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, classifyAPIError(err), err.Error())}, nil
	}
	payload := map[string]any{
		"released":           changed,
		"single_tab_session": session.SingleTabSession,
	}
	return &runtimev1.ExecuteResponse{Result: completedResult(req, mustJSON(payload))}, nil
}

func (s *Server) executeBind(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	call := req.GetToolCall()
	if call == nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "tool_call is required")}, nil
	}
	var args bindArgs
	if raw := strings.TrimSpace(call.GetArgsJson()); raw != "" {
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "args_json must decode into single_tab.bind args")}, nil
		}
	}
	if strings.TrimSpace(call.GetRunId()) == "" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "run_id is required for single_tab.bind")}, nil
	}

	sessionKey := strings.TrimSpace(args.SessionKey)
	if sessionKey == "" {
		resolvedSessionKey, err := s.lookupRunSessionKey(ctx, call.GetRunId())
		if err != nil {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, classifyAPIError(err), err.Error())}, nil
		}
		sessionKey = resolvedSessionKey
	}
	if sessionKey == "" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, "run does not contain session_key")}, nil
	}

	binding, err := s.ensureBoundSession(ctx, call, sessionKey, strings.TrimSpace(args.BrowserInstanceID), normalizeBindWaitTimeout(args.WaitTimeoutMS), "single_tab.bind_runtime")
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, classifyAPIError(err), bindErrorMessage(err))}, nil
	}
	if binding.PendingApproval {
		return &runtimev1.ExecuteResponse{Result: completedResult(req, mustJSON(map[string]any{
			"approval_required": true,
			"approval_id":       binding.ApprovalID,
			"status":            "PENDING_APPROVAL",
		}))}, nil
	}

	payload := map[string]any{
		"approval_required":  false,
		"approval_id":        binding.ApprovalID,
		"session_id":         binding.SessionID,
		"status":             stringValue(binding.Session.SingleTabSession["status"]),
		"single_tab_session": binding.Session.SingleTabSession,
	}
	return &runtimev1.ExecuteResponse{Result: completedResult(req, mustJSON(payload))}, nil
}

func (s *Server) executeAction(ctx context.Context, req *runtimev1.ExecuteRequest, actionType string) (*runtimev1.ExecuteResponse, error) {
	if s.dispatcher == nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, "browser bridge dispatcher is not configured")}, nil
	}

	call := req.GetToolCall()
	var rawArgs map[string]any
	if err := json.Unmarshal([]byte(call.GetArgsJson()), &rawArgs); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "args_json must decode into tool args")}, nil
	}
	sessionID, session, err := s.resolveSessionEnvelope(ctx, call, stringValue(rawArgs["session_id"]), stringValue(rawArgs["session_key"]))
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, classifyAPIError(err), sessionResolutionErrorMessage(call.GetToolName(), err))}, nil
	}
	if status := strings.TrimSpace(stringValue(session.SingleTabSession["status"])); status != "" && status != "ACTIVE" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_APPROVAL_ERROR, "single-tab session is not active")}, nil
	}

	boundTabRef := strings.TrimSpace(stringValue(session.SingleTabSession["bound_tab_ref"]))
	if boundTabRef == "" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, "single-tab session is missing bound_tab_ref")}, nil
	}

	dispatchEnvelope, err := s.dispatcher.DispatchAction(ctx, client.DispatchActionParams{
		SingleTabSessionID: sessionID,
		BoundTabRef:        boundTabRef,
		ActionType:         actionType,
		ArgsJSON:           call.GetArgsJson(),
	})
	if err != nil {
		if recoveryResp, recovered := s.tryRecoverAction(ctx, req, call, session, actionType, err); recovered {
			return recoveryResp, nil
		}
		s.handleDispatchFailure(ctx, sessionID, err)
		return &runtimev1.ExecuteResponse{Result: failedResult(req, classifyDispatchError(err), actionErrorMessage(call.GetToolName(), err))}, nil
	}

	action := dispatchEnvelope.Action
	resultJSON := stringValue(action["result_json"])
	if resultJSON == "" {
		resultJSON = mustJSON(action)
	}

	if actionType == "capture_visible" {
		materializedJSON, err := s.materializeCaptureArtifact(ctx, call, session, sessionID, action, resultJSON)
		if err != nil {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, err.Error())}, nil
		}
		resultJSON = materializedJSON
	}

	_, _ = s.client.UpdateSessionState(ctx, sessionID, client.UpdateSessionStateParams{
		Status:       firstNonEmptyString(stringValue(action["session_status"]), "ACTIVE"),
		CurrentURL:   stringValue(action["current_url"]),
		CurrentTitle: stringValue(action["current_title"]),
		LastSeenAt:   time.Now().UTC().Format(time.RFC3339),
	})

	return &runtimev1.ExecuteResponse{Result: completedResult(req, resultJSON)}, nil
}

type bindSessionResult struct {
	Session         client.SessionEnvelope
	SessionID       string
	ApprovalID      string
	PendingApproval bool
}

func (s *Server) tryRecoverAction(ctx context.Context, req *runtimev1.ExecuteRequest, call *toolbrokerv1.ToolCall, session client.SessionEnvelope, actionType string, actionErr error) (*runtimev1.ExecuteResponse, bool) {
	if !isRecoverableActionError(actionErr) {
		return nil, false
	}

	sessionKey := strings.TrimSpace(stringValue(session.SingleTabSession["session_key"]))
	if sessionKey == "" && call != nil && strings.TrimSpace(call.GetRunId()) != "" {
		resolvedSessionKey, err := s.lookupRunSessionKey(ctx, call.GetRunId())
		if err == nil {
			sessionKey = resolvedSessionKey
		}
	}
	if sessionKey == "" {
		return nil, false
	}

	browserInstanceID := strings.TrimSpace(stringValue(session.SingleTabSession["browser_instance_id"]))
	binding, err := s.ensureBoundSession(ctx, call, sessionKey, browserInstanceID, normalizeBindWaitTimeout(0), "single_tab.action_recovery")
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResultDetailed(
			req,
			classifyDispatchError(actionErr),
			actionErrorMessage(call.GetToolName(), actionErr),
			true,
			map[string]any{
				"recovery_attempted": true,
				"recovery_tool":      "single_tab.bind",
				"original_tool":      call.GetToolName(),
				"session_key":        sessionKey,
			},
		)}, true
	}
	if binding.PendingApproval {
		return &runtimev1.ExecuteResponse{Result: failedResultDetailed(
			req,
			commonv1.ErrorClass_ERROR_CLASS_APPROVAL_ERROR,
			fmt.Sprintf("tab reconnection was requested; wait for tab selection approval and then retry %s once", strings.TrimSpace(call.GetToolName())),
			true,
			map[string]any{
				"approval_required": true,
				"approval_id":       binding.ApprovalID,
				"recovery_tool":     "single_tab.bind",
				"original_tool":     call.GetToolName(),
				"session_key":       sessionKey,
			},
		)}, true
	}

	recoveredSessionID := strings.TrimSpace(binding.SessionID)
	recoveredBoundTabRef := strings.TrimSpace(stringValue(binding.Session.SingleTabSession["bound_tab_ref"]))
	if recoveredSessionID == "" || recoveredBoundTabRef == "" {
		return &runtimev1.ExecuteResponse{Result: failedResultDetailed(
			req,
			commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR,
			fmt.Sprintf("tab reconnection completed but %s could not resume because the recovered session is incomplete", strings.TrimSpace(call.GetToolName())),
			false,
			map[string]any{
				"recovery_attempted": true,
				"recovery_tool":      "single_tab.bind",
				"original_tool":      call.GetToolName(),
			},
		)}, true
	}

	dispatchEnvelope, err := s.dispatcher.DispatchAction(ctx, client.DispatchActionParams{
		SingleTabSessionID: recoveredSessionID,
		BoundTabRef:        recoveredBoundTabRef,
		ActionType:         actionType,
		ArgsJSON:           call.GetArgsJson(),
	})
	if err != nil {
		s.handleDispatchFailure(ctx, recoveredSessionID, err)
		return &runtimev1.ExecuteResponse{Result: failedResultDetailed(
			req,
			classifyDispatchError(err),
			fmt.Sprintf("automatic tab reconnection succeeded, but %s still failed: %s", strings.TrimSpace(call.GetToolName()), actionErrorMessage(call.GetToolName(), err)),
			true,
			map[string]any{
				"recovery_attempted": true,
				"recovery_tool":      "single_tab.bind",
				"original_tool":      call.GetToolName(),
				"session_id":         recoveredSessionID,
			},
		)}, true
	}

	action := dispatchEnvelope.Action
	resultJSON := stringValue(action["result_json"])
	if resultJSON == "" {
		resultJSON = mustJSON(action)
	}
	if actionType == "capture_visible" {
		materializedJSON, err := s.materializeCaptureArtifact(ctx, call, binding.Session, recoveredSessionID, action, resultJSON)
		if err != nil {
			return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, err.Error())}, true
		}
		resultJSON = materializedJSON
	}

	_, _ = s.client.UpdateSessionState(ctx, recoveredSessionID, client.UpdateSessionStateParams{
		Status:       firstNonEmptyString(stringValue(action["session_status"]), "ACTIVE"),
		CurrentURL:   stringValue(action["current_url"]),
		CurrentTitle: stringValue(action["current_title"]),
		LastSeenAt:   time.Now().UTC().Format(time.RFC3339),
	})

	return &runtimev1.ExecuteResponse{Result: completedResult(req, resultJSON)}, true
}

func (s *Server) materializeCaptureArtifact(ctx context.Context, call *toolbrokerv1.ToolCall, session client.SessionEnvelope, sessionID string, action map[string]any, resultJSON string) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(resultJSON), &payload); err != nil {
		return "", err
	}
	imageRef := strings.TrimSpace(stringValue(payload["image_ref"]))
	if imageRef == "" {
		return resultJSON, nil
	}
	if !strings.HasPrefix(imageRef, "data:image/") {
		return resultJSON, nil
	}

	artifact, err := s.client.CreateBrowserCaptureArtifact(ctx, client.CreateBrowserCaptureArtifactParams{
		RunID:              call.GetRunId(),
		SessionKey:         stringValue(session.SingleTabSession["session_key"]),
		ToolCallID:         call.GetToolCallId(),
		SingleTabSessionID: sessionID,
		CurrentURL:         firstNonEmptyString(stringValue(action["current_url"]), stringValue(session.SingleTabSession["current_url"])),
		CurrentTitle:       firstNonEmptyString(stringValue(action["current_title"]), stringValue(session.SingleTabSession["current_title"])),
		ImageDataURL:       imageRef,
	})
	if err != nil {
		return "", err
	}
	artifactID := strings.TrimSpace(stringValue(artifact.Artifact["artifact_id"]))
	if artifactID == "" {
		return "", fmt.Errorf("browser capture artifact response is missing artifact_id")
	}
	payload["image_ref"] = artifactID
	payload["artifact_id"] = artifactID
	return mustJSON(payload), nil
}

func (s *Server) handleDispatchFailure(ctx context.Context, sessionID string, err error) {
	var bridgeErr *client.BrowserBridgeAPIError
	if !errorAsBridge(err, &bridgeErr) {
		return
	}
	if bridgeErr.Code != "tab_closed" && bridgeErr.Code != "host_unavailable" {
		return
	}

	status := "HOST_DISCONNECTED"
	reason := "browser bridge is unavailable"
	if bridgeErr.Code == "tab_closed" {
		status = "TAB_CLOSED"
		reason = "bound browser tab is closed"
	}
	_, _ = s.client.UpdateSessionState(ctx, sessionID, client.UpdateSessionStateParams{
		Status:       status,
		StatusReason: reason,
		LastSeenAt:   time.Now().UTC().Format(time.RFC3339),
	})
}

func classifyAPIError(err error) commonv1.ErrorClass {
	var apiErr *client.APIError
	if ok := errorAs(err, &apiErr); ok {
		switch apiErr.StatusCode {
		case 400:
			return commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR
		case 404:
			return commonv1.ErrorClass_ERROR_CLASS_TOOL_ERROR
		case 409:
			return commonv1.ErrorClass_ERROR_CLASS_APPROVAL_ERROR
		case 503:
			return commonv1.ErrorClass_ERROR_CLASS_TOOL_ERROR
		default:
			return commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR
		}
	}
	return commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR
}

func classifyDispatchError(err error) commonv1.ErrorClass {
	var bridgeErr *client.BrowserBridgeAPIError
	if !errorAsBridge(err, &bridgeErr) {
		return commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR
	}
	switch bridgeErr.Code {
	case "invalid_request", "selector_not_found", "action_not_allowed":
		return commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR
	case "tab_closed", "host_unavailable":
		return commonv1.ErrorClass_ERROR_CLASS_TOOL_ERROR
	default:
		return commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR
	}
}

func errorAs(err error, target any) bool {
	switch t := target.(type) {
	case **client.APIError:
		apiErr, ok := err.(*client.APIError)
		if ok {
			*t = apiErr
			return true
		}
	}
	return false
}

func errorAsBridge(err error, target any) bool {
	switch t := target.(type) {
	case **client.BrowserBridgeAPIError:
		apiErr, ok := err.(*client.BrowserBridgeAPIError)
		if ok {
			*t = apiErr
			return true
		}
	}
	return false
}

func completedResult(req *runtimev1.ExecuteRequest, resultJSON string) *toolbrokerv1.ToolResult {
	call := req.GetToolCall()
	result := &toolbrokerv1.ToolResult{
		Status:     "completed",
		ResultJson: resultJSON,
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if call != nil {
		result.ToolCallId = call.GetToolCallId()
		result.RunId = call.GetRunId()
		result.ToolName = call.GetToolName()
	}
	return result
}

func failedResult(req *runtimev1.ExecuteRequest, class commonv1.ErrorClass, message string) *toolbrokerv1.ToolResult {
	return failedResultDetailed(req, class, message, false, nil)
}

func failedResultDetailed(req *runtimev1.ExecuteRequest, class commonv1.ErrorClass, message string, retryable bool, details map[string]any) *toolbrokerv1.ToolResult {
	call := req.GetToolCall()
	result := &toolbrokerv1.ToolResult{
		Status:     "failed",
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Error:      &toolbrokerv1.ToolError{ErrorClass: class, Message: message, Retryable: retryable, DetailsJson: mustJSON(detailsOrEmpty(details))},
	}
	if call != nil {
		result.ToolCallId = call.GetToolCallId()
		result.RunId = call.GetRunId()
		result.ToolName = call.GetToolName()
	}
	return result
}

func mustJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func detailsOrEmpty(details map[string]any) map[string]any {
	if len(details) == 0 {
		return map[string]any{}
	}
	return details
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeBindWaitTimeout(timeoutMS int) time.Duration {
	if timeoutMS <= 0 {
		timeoutMS = 90000
	}
	if timeoutMS < 1000 {
		timeoutMS = 1000
	}
	if timeoutMS > 180000 {
		timeoutMS = 180000
	}
	return time.Duration(timeoutMS) * time.Millisecond
}

func isRecoverableActionError(err error) bool {
	var bridgeErr *client.BrowserBridgeAPIError
	if !errorAsBridge(err, &bridgeErr) || bridgeErr == nil {
		return false
	}
	switch strings.TrimSpace(bridgeErr.Code) {
	case "host_unavailable", "tab_closed", "session_not_active":
		return true
	default:
		return false
	}
}

func sessionMatchesApproval(singleTabSession map[string]any, approvalID string) bool {
	if strings.TrimSpace(approvalID) == "" {
		return true
	}
	return strings.TrimSpace(stringValue(singleTabSession["approval_id"])) == strings.TrimSpace(approvalID)
}

func (s *Server) tryGetActiveSession(ctx context.Context, sessionKey string) (client.SessionEnvelope, bool) {
	session, err := s.client.GetActiveSession(ctx, sessionKey)
	if err != nil {
		var apiErr *client.APIError
		if ok := errorAs(err, &apiErr); ok && apiErr.StatusCode == 404 {
			return client.SessionEnvelope{}, false
		}
		return client.SessionEnvelope{}, false
	}
	if strings.TrimSpace(stringValue(session.SingleTabSession["single_tab_session_id"])) == "" {
		return client.SessionEnvelope{}, false
	}
	return session, true
}

func (s *Server) resolveSessionEnvelope(ctx context.Context, call *toolbrokerv1.ToolCall, sessionIDRaw, sessionKeyRaw string) (string, client.SessionEnvelope, error) {
	sessionID := strings.TrimSpace(sessionIDRaw)
	sessionKey := strings.TrimSpace(sessionKeyRaw)

	if sessionID != "" {
		session, err := s.client.GetSession(ctx, sessionID)
		if err == nil {
			return sessionID, session, nil
		}
		var apiErr *client.APIError
		if ok := errorAs(err, &apiErr); ok && apiErr.StatusCode == 404 {
			if active, ok := s.tryGetActiveSession(ctx, sessionID); ok {
				resolvedID := strings.TrimSpace(stringValue(active.SingleTabSession["single_tab_session_id"]))
				if resolvedID != "" {
					return resolvedID, active, nil
				}
			}
		} else {
			return "", client.SessionEnvelope{}, err
		}
	}

	if sessionKey != "" {
		if active, ok := s.tryGetActiveSession(ctx, sessionKey); ok {
			resolvedID := strings.TrimSpace(stringValue(active.SingleTabSession["single_tab_session_id"]))
			if resolvedID != "" {
				return resolvedID, active, nil
			}
		}
	}

	if call != nil && strings.TrimSpace(call.GetRunId()) != "" {
		runEnvelope, err := s.client.GetRun(ctx, call.GetRunId())
		if err == nil {
			runSessionKey := strings.TrimSpace(stringValue(runEnvelope.Run["session_key"]))
			if runSessionKey != "" {
				if active, ok := s.tryGetActiveSession(ctx, runSessionKey); ok {
					resolvedID := strings.TrimSpace(stringValue(active.SingleTabSession["single_tab_session_id"]))
					if resolvedID != "" {
						return resolvedID, active, nil
					}
				}
			}
		}
	}

	if sessionID != "" {
		return "", client.SessionEnvelope{}, &client.APIError{StatusCode: 404, Message: "single tab session not found"}
	}
	return "", client.SessionEnvelope{}, &client.APIError{StatusCode: 404, Message: "single tab session not found"}
}

func bindErrorMessage(err error) string {
	var apiErr *client.APIError
	if ok := errorAs(err, &apiErr); !ok || apiErr == nil {
		return err.Error()
	}

	if strings.TrimSpace(apiErr.Code) == "host_unavailable" &&
		strings.Contains(strings.ToLower(strings.TrimSpace(apiErr.Message)), "extension bind relay response timed out") {
		return "extension bind relay response timed out: open Butler Chromium Bridge popup and click Connect relay, then retry single_tab.bind"
	}
	return err.Error()
}

func (s *Server) lookupRunSessionKey(ctx context.Context, runID string) (string, error) {
	runEnvelope, err := s.client.GetRun(ctx, runID)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(stringValue(runEnvelope.Run["session_key"])), nil
}

func (s *Server) ensureBoundSession(ctx context.Context, call *toolbrokerv1.ToolCall, sessionKey, browserInstanceID string, waitTimeout time.Duration, requestSource string) (bindSessionResult, error) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return bindSessionResult{}, fmt.Errorf("session key is required")
	}
	if active, ok := s.tryGetActiveSession(ctx, sessionKey); ok {
		return bindSessionResult{
			Session:    active,
			SessionID:  stringValue(active.SingleTabSession["single_tab_session_id"]),
			ApprovalID: stringValue(active.SingleTabSession["approval_id"]),
		}, nil
	}

	toolCallID := ""
	runID := ""
	if call != nil {
		toolCallID = call.GetToolCallId()
		runID = call.GetRunId()
	}
	bindEnvelope, err := s.client.CreateBindRequest(ctx, client.CreateBindRequestParams{
		RunID:                    runID,
		SessionKey:               sessionKey,
		ToolCallID:               toolCallID,
		RequestSource:            firstNonEmptyString(requestSource, "single_tab.bind_runtime"),
		BrowserHint:              "tool-browser-local",
		BrowserInstanceID:        strings.TrimSpace(browserInstanceID),
		DiscoverTabsViaExtension: true,
	})
	if err != nil {
		if existing, ok := s.tryGetActiveSession(ctx, sessionKey); ok {
			return bindSessionResult{
				Session:    existing,
				SessionID:  stringValue(existing.SingleTabSession["single_tab_session_id"]),
				ApprovalID: stringValue(existing.SingleTabSession["approval_id"]),
			}, nil
		}
		return bindSessionResult{}, err
	}

	approvalID := strings.TrimSpace(stringValue(bindEnvelope.Approval["approval_id"]))
	if waitTimeout > 0 {
		deadline := time.Now().UTC().Add(waitTimeout)
		for time.Now().UTC().Before(deadline) {
			active, ok := s.tryGetActiveSession(ctx, sessionKey)
			if ok && sessionMatchesApproval(active.SingleTabSession, approvalID) {
				return bindSessionResult{
					Session:    active,
					SessionID:  stringValue(active.SingleTabSession["single_tab_session_id"]),
					ApprovalID: approvalID,
				}, nil
			}
			select {
			case <-ctx.Done():
				return bindSessionResult{}, fmt.Errorf("single_tab.bind interrupted while waiting for approval")
			case <-time.After(1200 * time.Millisecond):
			}
		}
	}

	return bindSessionResult{
		ApprovalID:      approvalID,
		PendingApproval: true,
	}, nil
}

func sessionResolutionErrorMessage(toolName string, err error) string {
	var apiErr *client.APIError
	if ok := errorAs(err, &apiErr); !ok || apiErr == nil {
		return err.Error()
	}
	if apiErr.StatusCode == 404 {
		return fmt.Sprintf("no active single-tab session is bound; run single_tab.bind and then retry %s once", strings.TrimSpace(toolName))
	}
	return err.Error()
}

func actionErrorMessage(toolName string, err error) string {
	var bridgeErr *client.BrowserBridgeAPIError
	if ok := errorAsBridge(err, &bridgeErr); !ok || bridgeErr == nil {
		return err.Error()
	}

	toolName = strings.TrimSpace(toolName)
	switch strings.TrimSpace(bridgeErr.Code) {
	case "host_unavailable":
		message := strings.ToLower(strings.TrimSpace(bridgeErr.Message))
		if strings.Contains(message, "heartbeat timed out") {
			return fmt.Sprintf("browser extension heartbeat timed out; run single_tab.bind and then retry %s once", toolName)
		}
		return fmt.Sprintf("browser tab connection is unavailable; run single_tab.bind and then retry %s once", toolName)
	case "session_not_active", "tab_closed":
		return fmt.Sprintf("bound browser tab is no longer active; run single_tab.bind and then retry %s once", toolName)
	default:
		return err.Error()
	}
}
