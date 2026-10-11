package providerauth

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAntigravityImportEncryptedPostgresAndReconnect(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_PROVIDER_AUTH_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated provider-auth PostgreSQL not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	schema := fmt.Sprintf("provider_auth_%d", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	privatePool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer privatePool.Close()
	if _, err := privatePool.Exec(ctx, `CREATE TABLE system_settings(key text PRIMARY KEY,value text NOT NULL,component text NOT NULL,is_secret boolean NOT NULL,updated_at timestamptz NOT NULL,updated_by text NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	codec := config.StaticSecretValueCodec{Key: strings.Repeat("a", 64)}
	store := config.NewPostgresSettingsStore(privatePool, config.WithSettingsCodec(codec))
	m := NewManager(store)
	if err := m.ImportAntigravity(ctx, strings.NewReader(importFixture(time.Now().Add(time.Hour), "project")), "operator"); err != nil {
		t.Fatal(err)
	}
	var stored string
	var isSecret bool
	if err := privatePool.QueryRow(ctx, `SELECT value,is_secret FROM system_settings WHERE key=$1`, settingKeyAntigravity).Scan(&stored, &isSecret); err != nil {
		t.Fatal(err)
	}
	if !isSecret || strings.Contains(stored, "access-old") || strings.Contains(stored, "refresh-private") || strings.Contains(stored, "client-private") {
		t.Fatal("raw OAuth persisted at rest")
	}
	restarted := NewManager(config.NewPostgresSettingsStore(privatePool, config.WithSettingsCodec(codec)))
	audit, err := restarted.AuditAntigravityOrigin(ctx, strings.NewReader(importFixture(time.Now().Add(-time.Hour), "ignored-reference-project")))
	if err != nil || !audit.StoredCredentials || !audit.OAuthApplicationMatches || !audit.OAuthCredentialMatches || !audit.RefreshCredentialMatches {
		t.Fatal("encrypted origin audit failed", err)
	}
	var unchanged string
	if privatePool.QueryRow(ctx, `SELECT value FROM system_settings WHERE key=$1`, settingKeyAntigravity).Scan(&unchanged) != nil || unchanged != stored {
		t.Fatal("origin audit rewrote encrypted auth state")
	}
	auth, err := restarted.ResolveAntigravity(ctx)
	if err != nil || auth.AccessToken != "access-old" || auth.ProjectID != "" {
		t.Fatal("encrypted credentials did not survive restart")
	}
	if err := restarted.Delete(ctx, modelprovider.ProviderAntigravity); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ResolveAntigravity(ctx); err != ErrNotConnected {
		t.Fatal("revoked database credentials were cached")
	}
	wrongKey := NewManager(config.NewPostgresSettingsStore(privatePool, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: strings.Repeat("b", 64)})))
	if err := m.ImportAntigravity(ctx, strings.NewReader(importFixture(time.Now().Add(time.Hour), "project")), "operator"); err != nil {
		t.Fatal(err)
	}
	if _, err := wrongKey.ResolveAntigravity(ctx); err == nil {
		t.Fatal("wrong encryption key accepted")
	}
	if _, err := wrongKey.AuditAntigravityOrigin(ctx, strings.NewReader(importFixture(time.Now().Add(time.Hour), ""))); err == nil || strings.Contains(err.Error(), "private") || strings.Contains(err.Error(), "access-old") {
		t.Fatal("origin audit accepted wrong encryption or leaked material")
	}
}
