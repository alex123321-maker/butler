package transport

import "context"

type EventStream <-chan TransportEvent

type ModelProvider interface {
	Name() string
	Capabilities(context.Context, TransportRunContext) (CapabilitySnapshot, error)
	StartRun(context.Context, StartRunRequest) (EventStream, error)
	ContinueRun(context.Context, ContinueRunRequest) (EventStream, error)
	SubmitToolResult(context.Context, SubmitToolResultRequest) (EventStream, error)
	CancelRun(context.Context, CancelRunRequest) (*TransportEvent, error)
}
