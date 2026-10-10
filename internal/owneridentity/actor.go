// Package owneridentity carries proof verified by a trusted channel adapter.
// Model arguments, request body owner IDs and arbitrary headers are not proof.
package owneridentity

import "context"

type Actor struct {
	OwnerID    string
	Channel    string
	SessionKey string
	DeliveryID string
}

type actorKey struct{}

func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}

func FromContext(ctx context.Context) (Actor, bool) {
	a, ok := ctx.Value(actorKey{}).(Actor)
	return a, ok && a.OwnerID != "" && a.DeliveryID != "" && (a.Channel == "web" || a.Channel == "telegram")
}
