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
	SessionID string `json:"session_id"`
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
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR, "single_tab.bind requires browser bridge tab discovery wiring")}, nil
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
	if strings.TrimSpace(args.SessionID) == "" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "session_id is required")}, nil
	}
	session, err := s.client.GetSession(ctx, args.SessionID)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, classifyAPIError(err), err.Error())}, nil
	}
	return &runtimev1.ExecuteResponse{Result: completedResult(req, mustJSON(session.SingleTabSession))}, nil
}

func (s *Server) executeRelease(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	var args sessionArgs
	if err := json.Unmarshal([]byte(req.GetToolCall().GetArgsJson()), &args); err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "args_json must decode into single_tab.release args")}, nil
	}
	if strings.TrimSpace(args.SessionID) == "" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "session_id is required")}, nil
	}
	session, changed, err := s.client.ReleaseSession(ctx, args.SessionID)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, classifyAPIError(err), err.Error())}, nil
	}
	payload := map[string]any{
		"released":           changed,
		"single_tab_session": session.SingleTabSession,
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
	sessionID := strings.TrimSpace(stringValue(rawArgs["session_id"]))
	if sessionID == "" {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR, "session_id is required")}, nil
	}

	session, err := s.client.GetSession(ctx, sessionID)
	if err != nil {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, classifyAPIError(err), err.Error())}, nil
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
		s.handleDispatchFailure(ctx, sessionID, err)
		return &runtimev1.ExecuteResponse{Result: failedResult(req, classifyDispatchError(err), err.Error())}, nil
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
	call := req.GetToolCall()
	result := &toolbrokerv1.ToolResult{
		Status:     "failed",
		FinishedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Error:      &toolbrokerv1.ToolError{ErrorClass: class, Message: message, Retryable: false, DetailsJson: "{}"},
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
