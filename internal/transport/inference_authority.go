package transport

import "context"

type inferenceAuthorityKey struct{}
type inferenceDispatchKey struct{}

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

// WithInferenceDispatch installs an optional one-attempt durable dispatch marker.
// Providers call it once immediately before their actual inference HTTP request,
// after all repeatable authority/auth/serialization checks. It contains no data
// and is never serialized. Unlike authority checks, it is not a reusable probe.
func WithInferenceDispatch(ctx context.Context, mark func(context.Context) error) context.Context {
	return context.WithValue(ctx, inferenceDispatchKey{}, mark)
}
func CheckInferenceDispatch(ctx context.Context) error {
	if mark, ok := ctx.Value(inferenceDispatchKey{}).(func(context.Context) error); ok && mark != nil {
		return mark(ctx)
	}
	return nil
}
