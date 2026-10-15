package orchestrator

import (
	"context"

	"github.com/butler/butler/internal/transport"
)

type operatorDoctorKey struct{}

// WithOperatorDoctor is used only by the durable input worker after reading its
// private admission marker. Neither input payloads nor models select this path.
func WithOperatorDoctor(ctx context.Context) context.Context {
	return context.WithValue(ctx, operatorDoctorKey{}, true)
}
func operatorDoctor(ctx context.Context) bool {
	v, _ := ctx.Value(operatorDoctorKey{}).(bool)
	return v
}

// The fixed operator command uses the existing run/tool/approval state machine.
// These locally generated events never enter a model transport or model budget.
func doctorCommandStream(runID string) transport.EventStream {
	ch := make(chan transport.TransportEvent, 1)
	ch <- transport.TransportEvent{RunID: runID, EventType: transport.EventTypeToolCallRequested, ToolCall: &transport.ToolCallRequest{ToolCallRef: "doctor-check-system", ToolName: "doctor.check_system", ArgsJSON: "{}"}}
	close(ch)
	return ch
}
func doctorResultStream(runID, result string) transport.EventStream {
	ch := make(chan transport.TransportEvent, 1)
	ch <- transport.NewAssistantFinalEvent(runID, "operator", transport.AssistantFinal{Content: result, FinishReason: "stop"})
	close(ch)
	return ch
}
