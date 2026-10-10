package budget

import "context"

type egressKey struct{}

// WithToolEgressAllowance accepts only a trusted Broker privacy guard's fixed
// enforced cap. It is context metadata, never a model-visible tool argument.
func WithToolEgressAllowance(ctx context.Context, bytes int64) context.Context {
	return context.WithValue(ctx, egressKey{}, bytes)
}
func ToolEgressAllowance(ctx context.Context) (int64, bool) {
	bytes, ok := ctx.Value(egressKey{}).(int64)
	return bytes, ok
}
