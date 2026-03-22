package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/butler/butler/apps/tool-browser-local/internal/client"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type fakeSessionClient struct{}
type fakeBridgeClient struct{}

func (f *fakeSessionClient) GetSession(_ context.Context, sessionID string) (client.SessionEnvelope, error) {
	return client.SessionEnvelope{SingleTabSession: map[string]any{
		"single_tab_session_id": sessionID,
		"session_key":           "telegram:chat:1",
		"status":                "ACTIVE",
		"current_url":           "https://example.com",
		"current_title":         "Example",
		"bound_tab_ref":         "17",
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
