package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/transport"
)

type browserOutcomeFixture struct {
	stage, raw, fail string
	calls            []*toolbrokerv1.ToolCall
	readDeadline     time.Time
}

func (f *browserOutcomeFixture) SaveCheckpoint(_ context.Context, _ string, stage, raw string) error {
	if f.fail == stage {
		return errors.New("unavailable")
	}
	f.stage, f.raw = stage, `{"checkpoint_data":`+raw+`}`
	return nil
}
func (f *browserOutcomeFixture) LoadCheckpoint(context.Context, string) (string, string, error) {
	return f.stage, f.raw, nil
}
func (f *browserOutcomeFixture) PrepareAction(ctx context.Context, call *toolbrokerv1.ToolCall) (actionauth.PreparedAction, error) {
	h, _ := actionauth.ArgumentsHash(call.ArgsJson)
	p := actionauth.PreparedAction{Binding: actionauth.ActionBinding{OwnerID: "owner", ActionID: "action-" + call.ToolCallId, ActionHash: "hash", RunID: call.RunId, ToolCallID: call.ToolCallId, ToolName: call.ToolName, Effect: actionauth.EffectUnknown, ArgsHash: h}, EffectID: "effect-" + call.ToolCallId}
	if call.ToolName == frozenBrowserObservation {
		f.readDeadline, _ = ctx.Deadline()
		p.Binding.Effect, p.Binding.ContractRevision, p.Binding.ContractHash = actionauth.EffectRead, "1", strings.Repeat("a", 64)
		p.Binding.Resource = "browser:" + strings.Repeat("a", 32) + ":"
		switch f.fail {
		case "permission":
			p.RequiresApproval = true
		case "revision":
			p.Binding.ContractRevision = "0"
		case "foreign-owner":
			p.Binding.OwnerID = "foreign"
		case "prepare":
			return p, errors.New("unavailable")
		}
	}
	return p, nil
}
func (f *browserOutcomeFixture) AuthorizeAction(_ context.Context, call *toolbrokerv1.ToolCall, _ string, approval, _ string) (string, error) {
	if call.ToolName == frozenBrowserObservation && (approval != "" || f.fail == "authorize") {
		return "", errors.New("denied")
	}
	return "grant-" + call.ToolCallId, nil
}
func (f *browserOutcomeFixture) ExecuteToolCall(context.Context, *toolbrokerv1.ToolCall) (*toolbrokerv1.ToolResult, error) {
	return nil, errors.New("unmediated dispatch forbidden")
}
func (f *browserOutcomeFixture) ExecuteAuthorizedToolCall(_ context.Context, call *toolbrokerv1.ToolCall, grant string) (*toolbrokerv1.ToolResult, error) {
	if grant != "grant-"+call.ToolCallId {
		return nil, errors.New("wrong grant")
	}
	f.calls = append(f.calls, call)
	r := &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, ActionId: "effect-" + call.ToolCallId, Status: "effect_unknown", EffectState: "effect_unknown", ResultJson: `{"ok":true}`}
	if call.ToolName == frozenBrowserObservation {
		if f.fail == "read-ack" {
			return nil, errors.New("lost ACK")
		}
		r.Status, r.EffectState = "completed", "effect_confirmed"
		r.ResultJson = `{"ok":true,"browser_session_id":"` + strings.Repeat("a", 32) + `","network_state":"frozen","observation_version":3,"text":"Current visible state only"}`
		if f.fail == "read-identity" {
			r.RunId = "foreign"
		}
	} else if f.fail == "original-ack" {
		return nil, errors.New("lost ACK")
	}
	return r, nil
}

func TestUnknownBrowserObservesOnceThenBlocksWithoutModelResume(t *testing.T) {
	for _, mode := range []string{"success", "permission", "revision", "foreign-owner", "prepare", "authorize", "read-ack", "read-identity", "original-ack", "browser_outcome_observing"} {
		t.Run(mode, func(t *testing.T) {
			f := &browserOutcomeFixture{fail: mode}
			history := &memoryTranscriptStore{}
			provider := &mockProvider{startEvents: []transport.TransportEvent{transport.NewToolCallRequestedEvent("", "test", transport.ToolCallRequest{ToolCallRef: "click", ToolName: "browser.click", ArgsJSON: `{"browser_session_id":"` + strings.Repeat("a", 32) + `","selector":"#submit","expected_url":"https://example.com/form","observation_version":1}`})}}
			s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), history, provider, Config{ProviderName: "test", ModelName: "test", Tools: f, ActionAuthorizer: f, Checkpoints: f}, nil)
			result, err := s.Execute(context.Background(), ingress.InputEvent{EventID: "event", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "session", Source: "web", PayloadJSON: `{"text":"submit once"}`, CreatedAt: time.Now(), IdempotencyKey: "event"})
			if err != nil || result.ReceiptStatus != "blocked" || result.ErrorCode != "effect_unknown" {
				t.Fatal("unknown did not stay blocked", result, err)
			}
			if len(provider.submittedResults) != 0 || len(history.messages) != 1 {
				t.Fatal("observation resumed model or published final")
			}
			want := 1
			if mode == "success" || mode == "read-ack" || mode == "read-identity" {
				want = 2
			}
			if len(f.calls) != want {
				t.Fatalf("got %d dispatches, want %d", len(f.calls), want)
			}
			if mode != "original-ack" && (len(history.toolCalls) == 0 || history.toolCalls[0].Status != "effect_unknown") {
				t.Fatal("original unknown overwritten")
			}
			if want == 2 {
				if f.calls[1].ToolName != frozenBrowserObservation || f.calls[1].ArgsJson != `{"browser_session_id":"`+strings.Repeat("a", 32)+`"}` {
					t.Fatal("read copied original input or changed context")
				}
				if f.readDeadline.IsZero() || time.Until(f.readDeadline) > 5*time.Second {
					t.Fatal("observation deadline is unbounded")
				}
			}
			if mode == "success" && (len(history.toolCalls) != 2 || f.stage != "browser_outcome_observed") {
				t.Fatal("observation not durably retained")
			}
		})
	}
}

func TestUnknownBrowserObservationCannotRetryOrExtendDeadline(t *testing.T) {
	f := &browserOutcomeFixture{}
	history := &memoryTranscriptStore{}
	s := &Service{config: Config{Tools: f, ActionAuthorizer: f, Checkpoints: f}, transcript: history}
	current := &sessionv1.RunRecord{RunId: "run"}
	call := &transport.ToolCallRequest{ToolCallRef: "fill", ToolName: "browser.fill", ArgsJSON: `{"browser_session_id":"` + strings.Repeat("a", 32) + `","value":{"type":"credential_ref","alias":"private","field":"password"}}`}
	original := actionauth.PreparedAction{EffectID: "original", Binding: actionauth.ActionBinding{OwnerID: "owner", Effect: actionauth.EffectUnknown}}
	result := &toolbrokerv1.ToolResult{RunId: "run", ToolCallId: "fill", ToolName: call.ToolName, ActionId: "original", Status: "effect_unknown"}
	f.SaveCheckpoint(context.Background(), "run", "tool_result_recorded", `{"tool_call_id":"fill","effect_id":"original","status":"effect_unknown"}`)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	deadline, _ := ctx.Deadline()
	if err := s.observeUnknownBrowser(ctx, current, call, original, result); err != nil {
		t.Fatal(err)
	}
	if err := s.observeUnknownBrowser(ctx, current, call, original, result); err != nil || len(f.calls) != 1 {
		t.Fatal("same unknown scheduled another read", err)
	}
	if !f.readDeadline.Equal(deadline) {
		t.Fatal("child extended parent deadline")
	}
	if strings.Contains(f.calls[0].ArgsJson, "private") {
		t.Fatal("credential ref copied to observation")
	}
	var checkpoint struct {
		Data struct {
			Original browserOutcomeCheckpoint `json:"uncertain_browser"`
		} `json:"checkpoint_data"`
	}
	if json.Unmarshal([]byte(f.raw), &checkpoint) != nil || checkpoint.Data.Original.EffectID != "original" {
		t.Fatal("lost original uncertainty marker")
	}
}
