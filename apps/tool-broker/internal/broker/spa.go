package broker

import (
	"context"

	"github.com/butler/butler/internal/browserspa"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resourceexec"
)

type spaExecutor struct {
	guarded RuntimeExecutor
	next    RuntimeExecutor
}

func NewSPAExecutor(store *resourceexec.Store, next RuntimeExecutor) (RuntimeExecutor, error) {
	guarded, err := NewResourceExecutor(store, next)
	if err != nil {
		return nil, err
	}
	return &spaExecutor{guarded: guarded, next: next}, nil
}
func (e *spaExecutor) Execute(ctx context.Context, c *toolbrokerv1.ToolCall, k *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if c.GetToolName() != browserspa.Tool {
		return e.next.Execute(ctx, c, k, secrets)
	}
	if !browserspa.ValidContract(k) || len(secrets) != 0 {
		return nil, browserspa.ErrDenied
	}
	return e.guarded.Execute(ctx, c, k, secrets)
}
