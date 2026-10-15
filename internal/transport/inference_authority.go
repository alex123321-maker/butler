package transport

import "context"

type inferenceAuthorityKey struct{}

// Trusted Butler composition can recheck current authority at actual dispatch.
// The callback is never serialized, model-selected, or a substitute for budgets.
func WithInferenceAuthority(ctx context.Context, check func(context.Context) error) context.Context {
	return context.WithValue(ctx, inferenceAuthorityKey{}, check)
}
func CheckInferenceAuthority(ctx context.Context) error {
	if check, ok := ctx.Value(inferenceAuthorityKey{}).(func(context.Context) error); ok && check != nil {
		return check(ctx)
	}
	return nil
}
