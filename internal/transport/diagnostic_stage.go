package transport

import "context"

// ProviderStage is a fixed content-free operation phase. It neither records
// credentials/network bodies nor establishes that inference was dispatched.
type ProviderStage string

const (
	ProviderStageAuthentication ProviderStage = "authentication"
	ProviderStageSerialization  ProviderStage = "request_serialization"
	ProviderStageHTTP           ProviderStage = "http_round_trip"
	ProviderStageStream         ProviderStage = "response_stream"
)

type providerStageObserverKey struct{}

// WithProviderStageObserver installs trusted, request-local diagnostics. Model
// options cannot create an observer; callers own synchronization and storage.
func WithProviderStageObserver(ctx context.Context, observer func(ProviderStage)) context.Context {
	if observer == nil {
		return ctx
	}
	return context.WithValue(ctx, providerStageObserverKey{}, observer)
}

// ObserveProviderStage accepts only the fixed vocabulary. A phase transition
// carries no user/model content, endpoint, token, project, or raw error.
func ObserveProviderStage(ctx context.Context, stage ProviderStage) {
	if ctx == nil {
		return
	}
	switch stage {
	case ProviderStageAuthentication, ProviderStageSerialization, ProviderStageHTTP, ProviderStageStream:
	default:
		return
	}
	if observer, ok := ctx.Value(providerStageObserverKey{}).(func(ProviderStage)); ok && observer != nil {
		observer(stage)
	}
}
