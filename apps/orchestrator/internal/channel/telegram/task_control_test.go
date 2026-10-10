package telegram

import "testing"

func TestTelegramTaskCommandsCarryExplicitTargetWithoutModelParsing(t *testing.T) {
	payload := map[string]any{}
	ownerControlPayload("/amend task_current только подготовь письмо", payload)
	if payload["task_id"] != "task_current" || payload["intent"] != "amend" || payload["execution_policy"] != "prepare_only" || payload["text"] != "только подготовь письмо" {
		t.Fatalf("command lost target/restriction: %#v", payload)
	}
	if !isOwnerControlText("стоп") || !isOwnerControlText("/stop task_current") || isOwnerControlText("explain draft only") {
		t.Fatal("control classification changed ordinary input")
	}
}
