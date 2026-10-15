package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

const schemaDialectJSON = "json_schema_response_json_schema_v1"
const schemaDialectGoogle = "native_openapi_response_schema_v1"

// Dialect is frozen runner configuration, never a model transport option. This
// factory has no admission, retry, fallback, corpus, signing or scoring capability.
func newSchemaDialectNativeFactory(ctx context.Context, pool *pgxpool.Pool, importPath, keyPath, dialect string) (func(json.RawMessage) (transport.ModelProvider, error), error) {
	return newSchemaDialectNativeFactoryWithTimeout(ctx, pool, importPath, keyPath, dialect, 120*time.Second)
}

// Caller-selected timeouts are prospective runner settings. Existing diagnostic
// callers retain their original two-minute setting through the wrapper above.
func newSchemaDialectNativeFactoryWithTimeout(ctx context.Context, pool *pgxpool.Pool, importPath, keyPath, dialect string, timeout time.Duration) (func(json.RawMessage) (transport.ModelProvider, error), error) {
	if (dialect != schemaDialectJSON && dialect != schemaDialectGoogle) || pool == nil || timeout <= 0 {
		return nil, errNativeFixtureDenied
	}
	key, err := privateNativeFixtureKey(keyPath)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	store := config.NewPostgresSettingsStore(pool, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: key}))
	manager := providerauth.NewManager(store)
	state, err := manager.State(ctx, modelprovider.ProviderAntigravity)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	if !state.Connected {
		file, err := privateNativeFixtureFile(importPath, 64*1024)
		if err != nil {
			return nil, errNativeFixtureDenied
		}
		err = manager.ImportAntigravity(ctx, file, "isolated-issue33-schema-dialect-diagnostic")
		_ = file.Close()
		if err != nil {
			return nil, errNativeFixtureDenied
		}
	}
	return func(schema json.RawMessage) (transport.ModelProvider, error) {
		if len(schema) == 0 {
			return nil, errNativeFixtureDenied
		}
		cfg := antigravity.Config{Model: antigravity.FlashHighModel, Timeout: timeout, AuthSource: manager, MaximumOutputTokens: 32768}
		if dialect == schemaDialectGoogle {
			cfg.GoogleResponseSchema = schema
		} else {
			cfg.JSONResponseSchema = schema
		}
		return antigravity.NewProvider(cfg)
	}, nil
}

func TestSchemaDialectFactoryUnknownSelectionDeniesBeforePrivateState(t *testing.T) {
	key := filepath.Join(t.TempDir(), "must-not-create-key")
	// An untouched sentinel pool must never be called for an unsupported dialect.
	if _, err := newSchemaDialectNativeFactory(context.Background(), &pgxpool.Pool{}, "missing-import", key, "model-selected-fallback"); err == nil {
		t.Fatal("unsupported dialect reached private state")
	}
	if _, err := os.Stat(key); !os.IsNotExist(err) {
		t.Fatal("unsupported dialect created private key material")
	}
}

func TestSchemaDialectFactoryInvalidTimeoutDeniesBeforePrivateState(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		key := filepath.Join(t.TempDir(), "must-not-create-key")
		if _, err := newSchemaDialectNativeFactoryWithTimeout(context.Background(), &pgxpool.Pool{}, "missing-import", key, schemaDialectGoogle, timeout); err == nil {
			t.Fatal("invalid timeout reached private state")
		}
		if _, err := os.Stat(key); !os.IsNotExist(err) {
			t.Fatal("invalid timeout created private key material")
		}
	}
}
