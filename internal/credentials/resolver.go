package credentials

import (
	"context"
)

type SecretResolver interface {
	ResolveSecretRef(context.Context, string) (string, error)
}

type ResolvedSecret struct {
	Alias string
	Field string
	Value string
}
