package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browserpage"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/transport"
)

func TestPageUnknownProbePinsRecordedPostEffectPageAndRunsOnce(t *testing.T) {
	for _, mode := range []string{"observed", "wrong-page", "wrong-document", "permission", "read-ack", "source-page", "source-empty"} {
		t.Run(mode, func(t *testing.T) {
			f := &browserOutcomeFixture{managed: true, fail: mode}
			history := &memoryTranscriptStore{}
			s := &Service{config: Config{Tools: f, ActionAuthorizer: f, Checkpoints: f}, transcript: history}
			ref := browserpage.Ref{ID: strings.Repeat("b", 32), DocumentID: strings.Repeat("c", 32), Version: 1}
			args := mustMarshalJSON(browserpage.Args{SessionID: strings.Repeat("a", 32), Page: &ref, Selector: "#submit"})
			hash, _ := actionauth.ArgumentsHash(args)
			b := actionauth.ActionBinding{OwnerID: "owner", RunID: "run", ToolCallID: "click", ToolName: browserpage.Click, ContractRevision: "1", Effect: actionauth.EffectUnknown, ArgsHash: hash, Resource: "browser:" + strings.Repeat("a", 32) + ":", BrowserPage: &browserpage.Binding{Ref: ref, URL: "https://example.com/form", RuntimeVersion: 1}}
			original := actionauth.PreparedAction{Binding: b, EffectID: "original"}
			ref.Version = 2
			if mode == "source-page" {
				ref.ID = strings.Repeat("d", 32)
			}
			body := mustMarshalJSON(browserpage.Observation{OK: true, SessionID: strings.Repeat("a", 32), Page: &ref, URL: "https://example.com/form"})
			if mode == "source-empty" {
				body = "{}"
			}
			result := &toolbrokerv1.ToolResult{RunId: "run", ToolCallId: "click", ToolName: browserpage.Click, ActionId: "original", Status: "effect_unknown", ResultJson: body}
			call := &transport.ToolCallRequest{ToolCallRef: "click", ToolName: browserpage.Click, ArgsJSON: args}
			if f.SaveCheckpoint(t.Context(), "run", "tool_result_recorded", `{"tool_call_id":"click","effect_id":"original","status":"effect_unknown"}`) != nil {
				t.Fatal("checkpoint")
			}
			_ = s.observeUnknownBrowser(context.Background(), &sessionv1.RunRecord{RunId: "run"}, call, original, result)
			count := len(f.calls)
			if mode == "observed" || mode == "wrong-document" {
				if count != 1 {
					t.Fatal("exact read not dispatched")
				}
				var probe browserpage.Args
				if json.Unmarshal([]byte(f.calls[0].ArgsJson), &probe) != nil || probe.Page == nil || *probe.Page != ref || f.calls[0].ToolName != browserpage.Freeze {
					t.Fatal("probe did not use canonical post-effect reference")
				}
			}
			if (mode == "wrong-page" || mode == "permission" || mode == "source-page" || mode == "source-empty") && count != 0 {
				t.Fatal("unrelated page or permission dispatched")
			}
			if mode == "observed" && !strings.Contains(f.raw, `"observation_status":"observed"`) {
				t.Fatal("frozen page not retained")
			}
			if mode == "wrong-document" && strings.Contains(f.raw, `"observation_status":"observed"`) {
				t.Fatal("another document satisfied observation")
			}
			_ = s.observeUnknownBrowser(context.Background(), &sessionv1.RunRecord{RunId: "run"}, call, original, result)
			if len(f.calls) != count {
				t.Fatal("probe replayed after saved marker")
			}
		})
	}
}
