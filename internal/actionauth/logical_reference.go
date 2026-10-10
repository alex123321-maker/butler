package actionauth

import "context"

type logicalReferenceKey struct{}

// Logical references are trusted checkpoint metadata. Never resolve this key
// from model arguments, provider call IDs or identical argument fingerprints.
func WithLogicalReference(ctx context.Context, ref string) context.Context {
	return context.WithValue(ctx, logicalReferenceKey{}, ref)
}
func LogicalReference(ctx context.Context) string {
	ref, _ := ctx.Value(logicalReferenceKey{}).(string)
	return ref
}
