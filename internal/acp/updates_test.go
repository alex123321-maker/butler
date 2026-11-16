package acp

import (
	"errors"
	"testing"
)

func TestACPUserEchoCannotBecomeAssistantOutput(t *testing.T) {
	wire := &exchange{sessionID: "session", prompting: true, limits: Limits{TextBytes: 1024}, modelID: "gemini-fixture", modelOptionID: "model"}
	if err := wire.update([]byte(`{"sessionId":"session","update":{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":"echo must not be an answer"}}}`)); err != nil || wire.text.Len() != 0 {
		t.Fatal("user echo failed or was released as assistant output", err)
	}
	if err := wire.update([]byte(`{"sessionId":"session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"answer"}}}`)); err != nil || wire.text.String() != "answer" {
		t.Fatal("assistant output lost", err)
	}
	if err := wire.update([]byte(`{"sessionId":"foreign","update":{"sessionUpdate":"user_message_chunk","content":{"type":"text","text":"foreign"}}}`)); !errors.Is(err, ErrProtocol) {
		t.Fatal("foreign-session echo accepted", err)
	}
}

func TestACPConfigNotificationsCannotChangeModelOrPermissions(t *testing.T) {
	wire := &exchange{sessionID: "session", prompting: true, modelID: "gemini-fixture", modelOptionID: "model"}
	if err := wire.update([]byte(`{"sessionId":"session","update":{"sessionUpdate":"config_option_update","configOptions":[{"id":"model","category":"model","currentValue":"gemini-fixture"}]}}`)); err != nil {
		t.Fatal(err)
	}
	if err := wire.update([]byte(`{"sessionId":"session","update":{"sessionUpdate":"config_option_update","configOptions":[{"id":"model","category":"model","currentValue":"other"}]}}`)); !errors.Is(err, ErrModel) {
		t.Fatal("model change accepted", err)
	}
	if err := wire.update([]byte(`{"sessionId":"session","update":{"sessionUpdate":"config_option_update","configOptions":[{"id":"model","currentValue":"other"}]}}`)); !errors.Is(err, ErrModel) {
		t.Fatal("model change without category accepted", err)
	}
	if err := wire.update([]byte(`{"sessionId":"session","update":{"sessionUpdate":"current_mode_update","currentModeId":"yolo"}}`)); !errors.Is(err, ErrTools) {
		t.Fatal("permission escalation accepted", err)
	}
}
