package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/tool-browser-local/internal/client"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type fakeSessionClient struct{}
type fakeBridgeClient struct{}
type fakeBindTimeoutSessionClient struct{}
type fakeUnavailableBridgeClient struct{}
type fakeRecoveringSessionClient struct {
	getActiveCalls int
}
type fakeRecoveringBridgeClient struct {
	dispatchCalls int
}

func (f *fakeSessionClient) GetSession(_ context.Context, sessionID string) (client.SessionEnvelope, error) {
	if strings.HasPrefix(sessionID, "telegram:chat:") {
		return client.SessionEnvelope{}, &client.APIError{StatusCode: 404, Message: "single tab session not found"}
	}
	return client.SessionEnvelope{SingleTabSession: map[string]any{
		"single_tab_session_id": sessionID,
		"session_key":           "telegram:chat:1",
		"status":                "ACTIVE",
		"current_url":           "https://example.com",
		"current_title":         "Example",
		"bound_tab_ref":         "17",
	}}, nil
}

func (f *fakeSessionClient) GetActiveSession(_ context.Context, sessionKey string) (client.SessionEnvelope, error) {
	if sessionKey == "telegram:chat:bind-pending" {
		return client.SessionEnvelope{}, &client.APIError{StatusCode: 404, Message: "single tab session not found"}
	}
	return client.SessionEnvelope{SingleTabSession: map[string]any{
		"single_tab_session_id": "single-tab-active-1",
		"session_key":           sessionKey,
		"status":                "ACTIVE",
		"approval_id":           "approval-bind-1",
		"bound_tab_ref":         "17",
		"current_url":           "https://example.com",
		"current_title":         "Example",
	}}, nil
}

func (f *fakeSessionClient) GetRun(_ context.Context, runID string) (client.RunEnvelope, error) {
	sessionKey := "telegram:chat:1"
	if runID == "run-bind-pending" {
		sessionKey = "telegram:chat:bind-pending"
	}
	return client.RunEnvelope{Run: map[string]any{
		"run_id":      runID,
		"session_key": sessionKey,
	}}, nil
}

func (f *fakeSessionClient) CreateBindRequest(_ context.Context, params client.CreateBindRequestParams) (client.CreateBindRequestEnvelope, error) {
	return client.CreateBindRequestEnvelope{Approval: map[string]any{
		"approval_id": "approval-bind-1",
		"run_id":      params.RunID,
		"session_key": params.SessionKey,
		"status":      "pending",
	}}, nil
}

func (f *fakeSessionClient) ReleaseSession(_ context.Context, sessionID string) (client.SessionEnvelope, bool, error) {
	return client.SessionEnvelope{SingleTabSession: map[string]any{
		"single_tab_session_id": sessionID,
		"status":                "REVOKED_BY_USER",
	}}, true, nil
}

func (f *fakeSessionClient) UpdateSessionState(_ context.Context, sessionID string, params client.UpdateSessionStateParams) (client.SessionEnvelope, error) {
	return client.SessionEnvelope{SingleTabSession: map[string]any{
		"single_tab_session_id": sessionID,
		"session_key":           "telegram:chat:1",
		"status":                params.Status,
		"current_url":           params.CurrentURL,
		"current_title":         params.CurrentTitle,
	}}, nil
}

func (f *fakeSessionClient) CreateBrowserCaptureArtifact(_ context.Context, params client.CreateBrowserCaptureArtifactParams) (client.ArtifactEnvelope, error) {
	return client.ArtifactEnvelope{Artifact: map[string]any{
		"artifact_id":   "artifact-capture-1",
		"artifact_type": "browser_capture",
		"source_ref":    params.ToolCallID,
	}}, nil
}

func (f *fakeBridgeClient) DispatchAction(_ context.Context, params client.DispatchActionParams) (client.DispatchActionEnvelope, error) {
	resultJSON := `{"ok":true,"current_url":"https://example.com/next","title":"Next"}`
	if params.ActionType == "capture_visible" {
		resultJSON = `{"image_ref":"data:image/png;base64,abc","current_url":"https://example.com/next","title":"Next"}`
	}
	return client.DispatchActionEnvelope{
		Action: map[string]any{
			"single_tab_session_id": params.SingleTabSessionID,
			"session_status":        "ACTIVE",
			"result_json":           resultJSON,
			"current_url":           "https://example.com/next",
			"current_title":         "Next",
		},
	}, nil
}

func (f *fakeUnavailableBridgeClient) DispatchAction(_ context.Context, _ client.DispatchActionParams) (client.DispatchActionEnvelope, error) {
	return client.DispatchActionEnvelope{}, &client.BrowserBridgeAPIError{
		StatusCode: 503,
		Code:       "host_unavailable",
		Message:    "extension heartbeat timed out",
	}
}

func (f *fakeRecoveringSessionClient) GetSession(_ context.Context, sessionID string) (client.SessionEnvelope, error) {
	return client.SessionEnvelope{SingleTabSession: map[string]any{
		"single_tab_session_id": sessionID,
		"session_key":           "telegram:chat:recover",
		"status":                "ACTIVE",
		"current_url":           "https://example.com",
		"current_title":         "Example",
		"bound_tab_ref":         "17",
		"browser_instance_id":   "browser-1",
	}}, nil
}

func (f *fakeRecoveringSessionClient) GetActiveSession(_ context.Context, sessionKey string) (client.SessionEnvelope, error) {
	f.getActiveCalls++
	if sessionKey != "telegram:chat:recover" {
		return client.SessionEnvelope{}, &client.APIError{StatusCode: 404, Message: "single tab session not found"}
	}
	if f.getActiveCalls < 2 {
		return client.SessionEnvelope{}, &client.APIError{StatusCode: 404, Message: "single tab session not found"}
	}
	return client.SessionEnvelope{SingleTabSession: map[string]any{
		"single_tab_session_id": "single-tab-recovered-1",
		"session_key":           sessionKey,
		"status":                "ACTIVE",
		"approval_id":           "approval-recover-1",
		"bound_tab_ref":         "31",
		"current_url":           "https://example.com/recovered",
		"current_title":         "Recovered",
		"browser_instance_id":   "browser-1",
	}}, nil
}

func (f *fakeRecoveringSessionClient) GetRun(_ context.Context, runID string) (client.RunEnvelope, error) {
	return client.RunEnvelope{Run: map[string]any{
		"run_id":      runID,
		"session_key": "telegram:chat:recover",
	}}, nil
}

func (f *fakeRecoveringSessionClient) CreateBindRequest(_ context.Context, params client.CreateBindRequestParams) (client.CreateBindRequestEnvelope, error) {
	return client.CreateBindRequestEnvelope{Approval: map[string]any{
		"approval_id": "approval-recover-1",
		"run_id":      params.RunID,
		"session_key": params.SessionKey,
		"status":      "pending",
	}}, nil
}

func (f *fakeRecoveringSessionClient) ReleaseSession(_ context.Context, sessionID string) (client.SessionEnvelope, bool, error) {
	return client.SessionEnvelope{SingleTabSession: map[string]any{
		"single_tab_session_id": sessionID,
		"status":                "REVOKED_BY_USER",
	}}, true, nil
}

func (f *fakeRecoveringSessionClient) UpdateSessionState(_ context.Context, sessionID string, params client.UpdateSessionStateParams) (client.SessionEnvelope, error) {
	return client.SessionEnvelope{SingleTabSession: map[string]any{
		"single_tab_session_id": sessionID,
		"session_key":           "telegram:chat:recover",
		"status":                params.Status,
		"current_url":           params.CurrentURL,
		"current_title":         params.CurrentTitle,
	}}, nil
}

func (f *fakeRecoveringSessionClient) CreateBrowserCaptureArtifact(_ context.Context, params client.CreateBrowserCaptureArtifactParams) (client.ArtifactEnvelope, error) {
	return client.ArtifactEnvelope{Artifact: map[string]any{
		"artifact_id":   "artifact-capture-recovered-1",
		"artifact_type": "browser_capture",
		"source_ref":    params.ToolCallID,
	}}, nil
}

func (f *fakeRecoveringBridgeClient) DispatchAction(_ context.Context, params client.DispatchActionParams) (client.DispatchActionEnvelope, error) {
	f.dispatchCalls++
	if f.dispatchCalls == 1 {
		return client.DispatchActionEnvelope{}, &client.BrowserBridgeAPIError{
			StatusCode: 503,
			Code:       "host_unavailable",
			Message:    "extension heartbeat timed out",
		}
	}
	return client.DispatchActionEnvelope{
		Action: map[string]any{
			"single_tab_session_id": params.SingleTabSessionID,
			"session_status":        "ACTIVE",
			"result_json":           `{"image_ref":"data:image/png;base64,recovered","current_url":"https://example.com/recovered","title":"Recovered"}`,
			"current_url":           "https://example.com/recovered",
			"current_title":         "Recovered",
		},
	}, nil
}

func (f *fakeBindTimeoutSessionClient) GetSession(_ context.Context, sessionID string) (client.SessionEnvelope, error) {
	return client.SessionEnvelope{SingleTabSession: map[string]any{
		"single_tab_session_id": sessionID,
		"session_key":           "telegram:chat:bind-timeout",
		"status":                "ACTIVE",
		"bound_tab_ref":         "17",
	}}, nil
}

func (f *fakeBindTimeoutSessionClient) GetActiveSession(_ context.Context, _ string) (client.SessionEnvelope, error) {
	return client.SessionEnvelope{}, &client.APIError{StatusCode: 404, Message: "single tab session not found"}
}

func (f *fakeBindTimeoutSessionClient) GetRun(_ context.Context, runID string) (client.RunEnvelope, error) {
	return client.RunEnvelope{Run: map[string]any{
		"run_id":      runID,
		"session_key": "telegram:chat:bind-timeout",
	}}, nil
}

func (f *fakeBindTimeoutSessionClient) CreateBindRequest(_ context.Context, _ client.CreateBindRequestParams) (client.CreateBindRequestEnvelope, error) {
	return client.CreateBindRequestEnvelope{}, &client.APIError{
		StatusCode: 503,
		Code:       "host_unavailable",
		Message:    "extension bind relay response timed out",
	}
}

func (f *fakeBindTimeoutSessionClient) ReleaseSession(_ context.Context, sessionID string) (client.SessionEnvelope, bool, error) {
	return client.SessionEnvelope{SingleTabSession: map[string]any{
		"single_tab_session_id": sessionID,
		"status":                "REVOKED_BY_USER",
	}}, true, nil
}

func (f *fakeBindTimeoutSessionClient) UpdateSessionState(_ context.Context, sessionID string, params client.UpdateSessionStateParams) (client.SessionEnvelope, error) {
	return client.SessionEnvelope{SingleTabSession: map[string]any{
		"single_tab_session_id": sessionID,
		"session_key":           "telegram:chat:bind-timeout",
		"status":                params.Status,
		"current_url":           params.CurrentURL,
		"current_title":         params.CurrentTitle,
	}}, nil
}

func (f *fakeBindTimeoutSessionClient) CreateBrowserCaptureArtifact(_ context.Context, params client.CreateBrowserCaptureArtifactParams) (client.ArtifactEnvelope, error) {
	return client.ArtifactEnvelope{Artifact: map[string]any{
		"artifact_id":   "artifact-capture-timeout",
		"artifact_type": "browser_capture",
		"source_ref":    params.ToolCallID,
	}}, nil
}

func TestExecuteStatusAndRelease(t *testing.T) {
	t.Parallel()

	server := NewServer(&fakeSessionClient{}, &fakeBridgeClient{}, nil)
	statusResp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "call-1", RunId: "run-1", ToolName: "single_tab.status", ArgsJson: `{"session_id":"single-tab-1"}`},
	})
	if err != nil {
		t.Fatalf("Execute status returned error: %v", err)
	}
	if statusResp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed status result, got %q", statusResp.GetResult().GetStatus())
	}

	releaseResp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "call-2", RunId: "run-1", ToolName: "single_tab.release", ArgsJson: `{"session_id":"single-tab-1"}`},
	})
	if err != nil {
		t.Fatalf("Execute release returned error: %v", err)
	}
	if releaseResp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed release result, got %q", releaseResp.GetResult().GetStatus())
	}
}

func TestExecuteNavigateDispatchesThroughBrowserBridge(t *testing.T) {
	t.Parallel()

	server := NewServer(&fakeSessionClient{}, &fakeBridgeClient{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "call-3", RunId: "run-1", ToolName: "single_tab.navigate", ArgsJson: `{"session_id":"single-tab-1","url":"https://example.com/next"}`},
	})
	if err != nil {
		t.Fatalf("Execute navigate returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed navigate result, got %q", resp.GetResult().GetStatus())
	}
}

func TestExecuteCaptureVisibleMaterializesArtifactRef(t *testing.T) {
	t.Parallel()

	server := NewServer(&fakeSessionClient{}, &fakeBridgeClient{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "call-4", RunId: "run-1", ToolName: "single_tab.capture_visible", ArgsJson: `{"session_id":"single-tab-1"}`},
	})
	if err != nil {
		t.Fatalf("Execute capture_visible returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed capture result, got %q", resp.GetResult().GetStatus())
	}
	if got := resp.GetResult().GetResultJson(); !strings.Contains(got, `"artifact-capture-1"`) {
		t.Fatalf("expected artifact-backed image ref, got %s", got)
	}
}

func TestExecuteCaptureVisibleFallsBackFromSessionKeyToActiveSession(t *testing.T) {
	t.Parallel()

	server := NewServer(&fakeSessionClient{}, &fakeBridgeClient{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "call-4b", RunId: "run-1", ToolName: "single_tab.capture_visible", ArgsJson: `{"session_id":"telegram:chat:1"}`},
	})
	if err != nil {
		t.Fatalf("Execute capture_visible returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed capture result, got %q", resp.GetResult().GetStatus())
	}
	if got := resp.GetResult().GetResultJson(); !strings.Contains(got, `"artifact-capture-1"`) {
		t.Fatalf("expected artifact-backed image ref, got %s", got)
	}
}

func TestExecuteCaptureVisibleReturnsRebindGuidanceOnBridgeFailure(t *testing.T) {
	t.Parallel()

	server := NewServer(&fakeSessionClient{}, &fakeUnavailableBridgeClient{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "call-4c", RunId: "run-1", ToolName: "single_tab.capture_visible", ArgsJson: `{"session_id":"single-tab-1"}`},
	})
	if err != nil {
		t.Fatalf("Execute capture_visible returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed capture result, got %q", resp.GetResult().GetStatus())
	}
	errorMessage := resp.GetResult().GetError().GetMessage()
	if !strings.Contains(errorMessage, "single_tab.bind") || !strings.Contains(errorMessage, "single_tab.capture_visible") {
		t.Fatalf("expected rebind guidance in error message, got %q", errorMessage)
	}
}

func TestExecuteCaptureVisibleRecoversByRebindingAndRetrying(t *testing.T) {
	t.Parallel()

	server := NewServer(&fakeRecoveringSessionClient{}, &fakeRecoveringBridgeClient{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "call-4d", RunId: "run-recover-1", ToolName: "single_tab.capture_visible", ArgsJson: `{"session_id":"single-tab-old-1"}`},
	})
	if err != nil {
		t.Fatalf("Execute capture_visible returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed capture result after recovery, got %q", resp.GetResult().GetStatus())
	}
	if got := resp.GetResult().GetResultJson(); !strings.Contains(got, `"artifact-capture-recovered-1"`) {
		t.Fatalf("expected artifact-backed recovered image ref, got %s", got)
	}
}

func TestExecuteBindReturnsExistingActiveSession(t *testing.T) {
	t.Parallel()

	server := NewServer(&fakeSessionClient{}, &fakeBridgeClient{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "call-bind-1", RunId: "run-1", ToolName: "single_tab.bind", ArgsJson: `{}`},
	})
	if err != nil {
		t.Fatalf("Execute bind returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed bind result, got %q", resp.GetResult().GetStatus())
	}
	if got := resp.GetResult().GetResultJson(); !strings.Contains(got, `"session_id":"single-tab-active-1"`) {
		t.Fatalf("expected active session payload, got %s", got)
	}
}

func TestExecuteBindReturnsPendingApprovalWhenSessionNotYetSelected(t *testing.T) {
	t.Parallel()

	server := NewServer(&fakeSessionClient{}, &fakeBridgeClient{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "call-bind-2", RunId: "run-bind-pending", ToolName: "single_tab.bind", ArgsJson: `{"wait_timeout_ms":1000}`},
	})
	if err != nil {
		t.Fatalf("Execute bind returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "completed" {
		t.Fatalf("expected completed bind result, got %q", resp.GetResult().GetStatus())
	}
	if got := resp.GetResult().GetResultJson(); !strings.Contains(got, `"approval_required":true`) {
		t.Fatalf("expected pending approval payload, got %s", got)
	}
}

func TestExecuteBindTimeoutReturnsActionableMessage(t *testing.T) {
	t.Parallel()

	server := NewServer(&fakeBindTimeoutSessionClient{}, &fakeBridgeClient{}, nil)
	resp, err := server.Execute(context.Background(), &runtimev1.ExecuteRequest{
		ToolCall: &toolbrokerv1.ToolCall{ToolCallId: "call-bind-timeout", RunId: "run-bind-timeout", ToolName: "single_tab.bind", ArgsJson: `{}`},
	})
	if err != nil {
		t.Fatalf("Execute bind returned error: %v", err)
	}
	if resp.GetResult().GetStatus() != "failed" {
		t.Fatalf("expected failed bind result, got %q", resp.GetResult().GetStatus())
	}
	errorMessage := resp.GetResult().GetError().GetMessage()
	if !strings.Contains(errorMessage, "click Connect relay") {
		t.Fatalf("expected actionable connect relay message, got %q", errorMessage)
	}
}

func TestNormalizeBindWaitTimeoutDefaultsToNinetySeconds(t *testing.T) {
	t.Parallel()

	if got := normalizeBindWaitTimeout(0); got != 90*time.Second {
		t.Fatalf("expected default bind wait timeout 90s, got %s", got)
	}
}
