package budget

import "context"

type modelOperationKey struct{}

// Content-free observability only. This reference is not admission, ownership
// or dispatch permission; an observer must corroborate its durable owning row.
func WithModelOperation(ctx context.Context, operationID string) context.Context {
	return context.WithValue(ctx, modelOperationKey{}, operationID)
}

func ModelOperationFromContext(ctx context.Context) (string, bool) {
	value, ok := ctx.Value(modelOperationKey{}).(string)
	return value, ok && value != ""
}
