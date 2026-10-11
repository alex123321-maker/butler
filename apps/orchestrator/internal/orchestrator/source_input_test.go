package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func TestOpaqueSourceInputRequiresTrustedHookAndKeepsIdentity(t *testing.T) {
	event := ingress.InputEvent{EventID: "voice-reviewed:fixture", SessionKey: "web:owner", Source: "web", PayloadJSON: `{"voice_review_binding":"fixture"}`}
	service := &Service{}
	if _, e := service.prepareSourceInput(context.Background(), event); e == nil {
		t.Fatal("opaque data released without composition")
	}
	service.config.InputSourcePreparation = func(_ context.Context, e ingress.InputEvent) (ingress.InputEvent, error) {
		e.SessionKey = "foreign"
		return e, nil
	}
	if _, e := service.prepareSourceInput(context.Background(), event); e == nil {
		t.Fatal("hook changed session authority")
	}
	service.config.InputSourcePreparation = func(_ context.Context, e ingress.InputEvent) (ingress.InputEvent, error) {
		e.PayloadJSON = `{"text":"PRIVATE REVIEWED BODY"}`
		return e, nil
	}
	prepared := preparedRun{SourceInputEvent: &event, UserMessage: "PRIVATE REVIEWED BODY", InputPayload: map[string]any{"text": "PRIVATE REVIEWED BODY"}, InputItems: []transport.InputItem{content.CurrentOwnerMessage("PRIVATE REVIEWED BODY", "input:"+event.EventID)}}
	durable := durableSourcePreparation(prepared)
	if strings.Contains(mustMarshalJSON(durable), "PRIVATE REVIEWED BODY") {
		t.Fatal("reviewed speech copied to durable preparation")
	}
	if e := service.rehydrateSourcePreparation(context.Background(), &durable); e != nil || durable.UserMessage != "PRIVATE REVIEWED BODY" || durable.InputItems[0].Content != "PRIVATE REVIEWED BODY" {
		t.Fatal("current source did not resolve", e)
	}
	if prepared.UserMessage != "PRIVATE REVIEWED BODY" || prepared.InputItems[0].Content != "PRIVATE REVIEWED BODY" {
		t.Fatal("durable projection mutated live input")
	}
}

func TestOpaqueSourceInputRecoveryRejectsMissingOrReplacedBinding(t *testing.T) {
	event := ingress.InputEvent{EventID: "voice-reviewed:fixture", SessionKey: "web:owner", Source: "web", PayloadJSON: `{"voice_review_binding":"fixture"}`}
	if e := validateSourceCheckpoint(event, preparedRun{UserMessage: "cached speech"}); e == nil {
		t.Fatal("recovery accepted cached speech without binding")
	}
	replaced := event
	replaced.PayloadJSON = `{"voice_review_binding":"foreign"}`
	if e := validateSourceCheckpoint(event, preparedRun{SourceInputEvent: &replaced}); e == nil {
		t.Fatal("recovery accepted a replaced binding")
	}
	if e := validateSourceCheckpoint(event, preparedRun{SourceInputEvent: &event}); e != nil {
		t.Fatal(e)
	}
}

func TestOpaqueSourceDataProjectionRetainsInstructionsAndEffectMetadata(t *testing.T) {
	const private = "PRIVATE SELECTED SOURCE BODY"
	source := content.Origin{Kind: content.SubagentOutput, SourceRefs: []string{"source:exact"}, Sensitivity: content.Private}
	tool := content.Data(map[string]any{"action_id": "permanent-effect", "effect_state": "unknown"}, content.Origin{Kind: content.ToolOutput, SourceRefs: source.SourceRefs, Sensitivity: content.Private})
	instruction := content.Instruction("fixed kernel label", content.Origin{Kind: content.KernelInstruction, SourceRefs: source.SourceRefs, Sensitivity: content.Private})
	live := preparedRun{SourceDataRefs: []string{"source:exact"}, InputItems: []transport.InputItem{content.Data(private, source), instruction, tool, content.Data("other current data", content.Origin{Kind: content.Document, SourceRefs: []string{"source:other"}, Sensitivity: content.Private})}}
	durable := durableSourcePreparation(live)
	if strings.Contains(mustMarshalJSON(durable), private) || !strings.Contains(mustMarshalJSON(live), private) {
		t.Fatal("selected DATA body was persisted or live input mutated")
	}
	if !strings.Contains(durable.InputItems[0].Content, "reload_current_data") || content.ItemOrigin(durable.InputItems[0]).IsInstruction() {
		t.Fatal("selected reference became authority")
	}
	for _, i := range []int{1, 2, 3} {
		if mustMarshalJSON(live.InputItems[i]) != mustMarshalJSON(durable.InputItems[i]) {
			t.Fatal("instruction/effect/unselected DATA changed", i)
		}
	}
}
