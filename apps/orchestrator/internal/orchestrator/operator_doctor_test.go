package orchestrator

import (
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestOperatorDoctorCommandIdentitySurvivesReplayWithoutCrossRunCollision(t *testing.T) {
	command := func(run string) *transport.ToolCallRequest {
		stream := doctorCommandStream(run)
		event, ok := <-stream
		if !ok || event.RunID != run || event.EventType != transport.EventTypeToolCallRequested || event.ToolCall == nil || event.ToolCall.ToolCallRef == "" || event.ToolCall.ToolName != "doctor.check_system" || event.ToolCall.ArgsJSON != "{}" || event.Usage != nil {
			t.Fatal("operator command changed its fixed read-only non-model contract")
		}
		if _, ok = <-stream; ok {
			t.Fatal("operator command emitted another event")
		}
		return event.ToolCall
	}
	first, replay, other := command("run-a"), command("run-a"), command("run-b")
	if first.ToolCallRef != replay.ToolCallRef || first.ToolCallRef == other.ToolCallRef {
		t.Fatal("Doctor tool identity is not stable within a run and distinct between runs")
	}
}
