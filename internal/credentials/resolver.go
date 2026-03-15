package credentials

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

type SecretResolver interface {
	ResolveSecretRef(context.Context, string) (string, error)
}

type EnvSecretResolver struct{}

func (EnvSecretResolver) ResolveSecretRef(_ context.Context, secretRef string) (string, error) {
	secretRef = strings.TrimSpace(secretRef)
	if secretRef == "" {
		return "", errors.New("secret_ref is required")
	}
	name, ok := strings.CutPrefix(secretRef, "env://")
	if !ok {
		return "", fmt.Errorf("unsupported secret_ref scheme in %q", secretRef)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("secret_ref %q must include an environment variable name", secretRef)
	}
	value, exists := os.LookupEnv(name)
	if !exists || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("environment secret %q is not set", name)
	}
	return value, nil
}

type ResolvedSecret struct {
	Alias string
	Field string
	Value string
}
