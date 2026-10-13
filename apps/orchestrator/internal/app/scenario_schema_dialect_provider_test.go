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
	if (dialect != schemaDialectJSON && dialect != schemaDialectGoogle) || pool == nil {
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
		cfg := antigravity.Config{Model: antigravity.FlashHighModel, Timeout: 120 * time.Second, AuthSource: manager, MaximumOutputTokens: 32768}
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
