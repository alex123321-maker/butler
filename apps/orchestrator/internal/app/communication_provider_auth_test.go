package app

import (
	"context"
	"strings"
	"testing"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestCommunicationNativeProviderRefreshStoreIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	store := config.NewPostgresSettingsStore(pool, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: "synthetic-native-refresh-test-key"}))
	old := config.Setting{Key: "BUTLER_PROVIDER_AUTH_ANTIGRAVITY", Value: `{"access_token":"synthetic-old","expires_at":"2000-01-01T00:00:00Z"}`, Component: "orchestrator", IsSecret: true, UpdatedBy: "fixture"}
	if _, err := store.Set(ctx, old); err != nil {
		t.Fatal("encrypted fixture unavailable")
	}
	guard := communicationProviderAuth{store: store}
	if got, err := guard.Get(ctx, old.Key); err != nil || got.Value != old.Value {
		t.Fatal("expired row blocked before normal Manager renewal")
	}
	renewed := old
	renewed.Value = `{"access_token":"synthetic-renewed","expires_at":"2100-01-01T00:00:00Z"}`
	renewed.UpdatedBy = "provider-auth-refresh"
	if _, err := guard.Set(ctx, renewed); err != nil {
		t.Fatal("normal Manager persistence denied")
	}
	for _, attack := range []string{"key", "writer", "component", "plaintext"} {
		v := renewed
		switch attack {
		case "key":
			v.Key = "BUTLER_OTHER_SECRET"
		case "writer":
			v.UpdatedBy = "operator-import"
		case "component":
			v.Component = "runtime"
		case "plaintext":
			v.IsSecret = false
		}
		if _, err := guard.Set(ctx, v); err == nil {
			t.Fatal("non-renewal write allowed", attack)
		}
	}
	if _, err := guard.Get(ctx, "BUTLER_OTHER_SECRET"); err == nil {
		t.Fatal("other credential read allowed")
	}
	if err := guard.Delete(ctx, old.Key); err == nil {
		t.Fatal("credential deletion allowed")
	}
	if got, err := store.Get(ctx, old.Key); err != nil || got.Value != renewed.Value {
		t.Fatal("renewed row changed after denied write")
	}
	var ciphertext string
	if err := pool.QueryRow(ctx, `SELECT value FROM system_settings WHERE key=$1`, old.Key).Scan(&ciphertext); err != nil || ciphertext == renewed.Value || strings.Contains(ciphertext, "synthetic-renewed") {
		t.Fatal("renewal was not encrypted")
	}
}
