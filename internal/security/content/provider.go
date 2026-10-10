package content

import (
	"context"
	"github.com/butler/butler/internal/transport"
)

// Protect wraps the orchestrator's provider, including restored checkpoints.
// Provider-native live function responses remain data handled by the adapter.
func Protect(provider transport.ModelProvider) transport.ModelProvider {
	return protectedProvider{provider}
}

type protectedProvider struct{ transport.ModelProvider }

func (p protectedProvider) StartRun(ctx context.Context, request transport.StartRunRequest) (transport.EventStream, error) {
	items, err := Normalize(request.InputItems)
	if err != nil {
		return nil, err
	}
	request.InputItems = items
	return p.ModelProvider.StartRun(ctx, request)
}

func (p protectedProvider) ContinueRun(ctx context.Context, request transport.ContinueRunRequest) (transport.EventStream, error) {
	items, err := Normalize(request.InputItems)
	if err != nil {
		return nil, err
	}
	request.InputItems = items
	return p.ModelProvider.ContinueRun(ctx, request)
}
