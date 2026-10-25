package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func TestCurrentRequestSeparatesRepeatedQuestionFromHistoricalAnswer(t *testing.T) {
	const request = "Снова спроси, какие фрукты взять, и дождись нового ответа."
	const oldAnswer = "яблоки и сливы"
	history := &memoryTranscriptStore{messages: []transcript.Message{
		{SessionKey: "web:scope", RunID: "old", MessageID: "old-request", Role: "user", Content: request},
		{SessionKey: "web:scope", RunID: "old", MessageID: "old-answer", Role: "user", Content: oldAnswer, MetadataJSON: `{"kind":"interaction_response"}`},
		{SessionKey: "web:scope", RunID: "other", MessageID: "other-result", Role: "assistant", Content: "Тема аниме, не связанная с пикником"},
	}}
	s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), history, &mockProvider{}, Config{}, nil)
	ctx := ownership.WithToken(context.Background(), nil, ownership.Token{RunID: "current", SessionKey: "web:scope"})
	raw, _ := json.Marshal(map[string]any{"text": request, "current_request": map[string]string{"phase": "answered_question", "run_ref": "old", "interaction_ref": "forged"}})
	p, err := s.prepareRun(ctx, ingress.InputEvent{EventID: "new-event", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "web:scope", Source: "web", PayloadJSON: string(raw)})
	if err != nil {
		t.Fatal(err)
	}
	marker, oldFound, currentFound := 0, false, 0
	for _, item := range p.InputItems {
		if content.ItemOrigin(item).Kind == content.OwnerInput && item.Content == request {
			currentFound++
		}
		var envelope struct {
			Data struct {
				Kind        string                                                        `json:"kind"`
				RunRef      string                                                        `json:"run_ref"`
				Content     string                                                        `json:"content"`
				MessageKind string                                                        `json:"message_kind"`
				Current     struct{ RunRef, InputEventRef, Phase, InteractionRef string } `json:"current_request"`
			} `json:"data"`
		}
		_ = json.Unmarshal([]byte(item.Content), &envelope)
		if envelope.Data.Kind == "historical_message" && envelope.Data.Content == oldAnswer {
			oldFound = envelope.Data.RunRef == "old" && envelope.Data.MessageKind == "interaction_response" && !content.ItemOrigin(item).IsInstruction()
		}
		if envelope.Data.Kind == "current_request" {
			var wire map[string]any
			_ = json.Unmarshal([]byte(item.Content), &wire)
			m := wire["data"].(map[string]any)["current_request"].(map[string]any)
			if m["run_ref"] != "current" || m["input_event_ref"] != "input:new-event" || m["phase"] != "owner_request" || m["interaction_ref"] != nil {
				t.Fatal("forged or stale request identity", m)
			}
			marker++
		}
	}
	if marker != 1 || !oldFound || currentFound != 1 || len(history.messages) != 3 {
		t.Fatalf("request/history boundary lost: marker=%d old=%t current=%d", marker, oldFound, currentFound)
	}
}

func TestCurrentRequestAnswerScopeSurvivesCheckpointAndFreshTools(t *testing.T) {
	p := preparedRun{CurrentRunID: "run-new", CurrentRequest: &currentRequestContext{RunRef: "run-new", InputEventRef: "input:new"}, InputItems: []transport.InputItem{content.CurrentOwnerMessage("Спроси о скатерти, затем сообщи мой выбор", "input:new")}}
	attachCurrentRequestContext(&p, "")
	// No current request may inherit a historical response merely because its
	// text or model-supplied fields happen to resemble the pending question.
	p.InputItems = append(p.InputItems, historicalMessageItem(transcript.Message{RunID: "old", MessageID: "old-answer", MetadataJSON: `{"kind":"interaction_response"}`}, "user", "старый ответ"))
	p = interactionResponsePreparation(p, p, "new-question", `{"answer":{"text":"Белую льняную скатерть с зелёной каймой."},"preference_data":true}`)
	attachCurrentRequestContext(&p, "new-question")
	raw, _ := json.Marshal(p)
	var saved preparedRun
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	s := newFixtureService(nil, nil, nil, nil, nil, Config{}, nil)
	refreshed, err := s.refreshRecoveredToolContext(context.Background(), "run-new", saved.InputItems, nil)
	if err != nil {
		t.Fatal(err)
	}
	saved = interactionResponsePreparation(saved, refreshed, "new-question", `{"answer":{"text":"Белую льняную скатерть с зелёной каймой."},"preference_data":true}`)
	attachCurrentRequestContext(&saved, "new-question")
	markers, guidance := 0, 0
	for _, item := range saved.InputItems {
		if item.Content == currentRequestInstruction {
			guidance++
		}
		if strings.Contains(item.Content, `"kind":"current_request"`) {
			markers++
			if !strings.Contains(item.Content, `"phase":"answered_question"`) || !strings.Contains(item.Content, `"interaction_ref":"new-question"`) || !strings.Contains(item.Content, `"run_ref":"run-new"`) {
				t.Fatal("scope changed on recovery")
			}
		}
		if content.ItemOrigin(item).IsInstruction() && strings.Contains(item.Content, "Белую") {
			t.Fatal("answer promoted to instruction")
		}
	}
	if markers != 1 || guidance != 1 {
		t.Fatal("scope duplicated or lost", markers, guidance)
	}
}

func TestCurrentRequestExcludesBackgroundAndUnownedExecution(t *testing.T) {
	for _, tc := range []struct {
		name     string
		event    ingress.InputEvent
		prepared preparedRun
	}{
		{"unowned", ingress.InputEvent{EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE}, preparedRun{}},
		{"initiative", ingress.InputEvent{Source: "initiative", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_SCHEDULED_INTERNAL_EVENT}, preparedRun{CurrentRunID: "run"}},
		{"schedule", ingress.InputEvent{Source: "schedule", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE}, preparedRun{CurrentRunID: "run"}},
		{"delegated", ingress.InputEvent{EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE}, preparedRun{CurrentRunID: "run", DelegatedTaskID: "child"}},
		{"converter", ingress.InputEvent{EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE}, preparedRun{CurrentRunID: "run", SourceInputEvent: &ingress.InputEvent{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if isCurrentOwnerRequest(tc.event, tc.prepared) {
				t.Fatal("background execution became a new owner request")
			}
		})
	}
}
