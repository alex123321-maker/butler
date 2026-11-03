package runtime

import (
	"context"

	"github.com/butler/butler/internal/browserspa"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
)

// SPA has its own resource ticket and live authority, while legacy contexts
// retain the existing independent owner-control/epoch composition.
type SPADispatch struct {
	runtimev1.UnimplementedToolRuntimeServiceServer
	Legacy  runtimev1.ToolRuntimeServiceServer
	Guarded runtimev1.ToolRuntimeServiceServer
}

func (d *SPADispatch) Execute(ctx context.Context, r *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	if r.GetToolCall().GetToolName() == browserspa.Tool {
		return d.Guarded.Execute(ctx, r)
	}
	return d.Legacy.Execute(ctx, r)
}
