package transport

import "context"

const DefaultInputByteLimit int64 = 8 * 1024 * 1024
const ErrorTypeInputBytesExceeded ErrorType = "provider_input_bytes_exceeded"

type inputByteLimitKey struct{}

// WithInputByteLimit carries a trusted kernel allowance. Providers check the
// complete serialized request, including retained history/tool declarations.
// It is deliberately absent from model-visible args and transportOptions.
func WithInputByteLimit(ctx context.Context, limit int64) context.Context {
	return context.WithValue(ctx, inputByteLimitKey{}, limit)
}
func CheckSerializedInput(ctx context.Context, length int) error {
	limit, ok := ctx.Value(inputByteLimitKey{}).(int64)
	if !ok {
		limit = DefaultInputByteLimit
	}
	if limit <= 0 || int64(length) > limit {
		return &Error{Type: ErrorTypeInputBytesExceeded, Message: "serialized provider input exceeds the trusted byte allowance", Retryable: false}
	}
	return nil
}
