package bridge

import (
	"context"
	"github.com/butler/butler/internal/singletabcontrol"
)

type GuardedMetadata struct{ dispatcher *Dispatcher }

func NewGuardedMetadata(d *Dispatcher) *GuardedMetadata { return &GuardedMetadata{d} }
func (m *GuardedMetadata) List(ctx context.Context) ([]singletabcontrol.Tab, error) {
	var response struct {
		Tabs []singletabcontrol.Tab `json:"tabs"`
	}
	if m.dispatcher.request(ctx, "tabs.guarded.list", struct{}{}, &response) != nil {
		return nil, singletabcontrol.ErrDenied
	}
	return response.Tabs, nil
}
func (m *GuardedMetadata) Check(ctx context.Context, t singletabcontrol.Tab) error {
	var response struct {
		Current bool `json:"current"`
	}
	if m.dispatcher.request(ctx, "tabs.guarded.check", t, &response) != nil || !response.Current {
		return singletabcontrol.ErrDenied
	}
	return nil
}
