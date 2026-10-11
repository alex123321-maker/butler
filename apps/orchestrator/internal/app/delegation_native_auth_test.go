package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Operator paths are opaque runtime metadata. This helper never prints their
// content/path, exposes credentials to input items or reads main PG settings.
// The existing control-plane Manager alone owns native import/refresh.
func nativeFixtureProvider(ctx context.Context, pool *pgxpool.Pool, importPath, keyPath string, limit *nativeDelegationLimiter) (*limitedNativeDelegationProvider, error) {
	if pool == nil || limit == nil || limit.pool != pool {
		return nil, errNativeFixtureDenied
	}
	key, err := privateNativeFixtureKey(keyPath)
	if err != nil {
		return nil, err
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
			return nil, err
		}
		err = manager.ImportAntigravity(ctx, file, "isolated-native-delegation-fixture")
		_ = file.Close()
		if err != nil {
			return nil, errNativeFixtureDenied
		}
	}
	// On restart, preserve the current encrypted rotation. Never reimport an old
	// read-only operator copy over credentials refreshed by the first process.
	provider, err := antigravity.NewProvider(antigravity.Config{Model: antigravity.DefaultModel, AuthSource: manager})
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	return &limitedNativeDelegationProvider{Provider: provider, limit: limit}, nil
}

func privateNativeFixtureFile(path string, maxBytes int64) (*os.File, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, errNativeFixtureDenied
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm() != 0600 || before.Size() <= 0 || before.Size() > maxBytes {
		return nil, errNativeFixtureDenied
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	after, err := file.Stat()
	current, checkErr := os.Lstat(path)
	if err != nil || checkErr != nil || !after.Mode().IsRegular() || after.Mode().Perm() != 0600 || !current.Mode().IsRegular() || !os.SameFile(before, after) || !os.SameFile(current, after) || after.Size() <= 0 || after.Size() > maxBytes {
		_ = file.Close()
		return nil, errNativeFixtureDenied
	}
	return file, nil
}

func privateNativeFixtureKey(path string) (string, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", errNativeFixtureDenied
	}
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		var raw [32]byte
		if _, err = rand.Read(raw[:]); err != nil {
			return "", errNativeFixtureDenied
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return "", errNativeFixtureDenied
		}
		_, writeErr := file.WriteString(hex.EncodeToString(raw[:]))
		syncErr := file.Sync()
		closeErr := file.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			return "", errNativeFixtureDenied
		}
	}
	file, err := privateNativeFixtureFile(path, 64)
	if err != nil {
		return "", err
	}
	defer file.Close()
	var raw [64]byte
	n, err := file.Read(raw[:])
	if err != nil || n != len(raw) {
		return "", errNativeFixtureDenied
	}
	if _, err := hex.DecodeString(string(raw[:])); err != nil {
		return "", errNativeFixtureDenied
	}
	return string(raw[:]), nil
}

func TestNativeDelegationPrivateRuntimeFiles(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "key")
	one, err := privateNativeFixtureKey(keyPath)
	if err != nil {
		t.Fatal("private key initialization failed")
	}
	two, err := privateNativeFixtureKey(keyPath)
	if err != nil || one != two {
		t.Fatal("restart changed the encryption key")
	}
	if err = os.Chmod(keyPath, 0644); err != nil {
		t.Fatal("fixture mode change failed")
	}
	if _, err = privateNativeFixtureKey(keyPath); err == nil {
		t.Fatal("public runtime material accepted")
	}
	if err = os.Chmod(keyPath, 0600); err != nil {
		t.Fatal("fixture mode restore failed")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err = os.Symlink(keyPath, link); err != nil {
		t.Fatal("lab fixture symlink unavailable")
	}
	if _, err = privateNativeFixtureKey(link); err == nil {
		t.Fatal("symlink runtime material accepted")
	}
}

func TestNativeDelegationEncryptedFixtureImportWithoutInferenceIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	limit, err := newNativeDelegationLimiter(ctx, pool)
	if err != nil {
		t.Fatal("private counter initialization failed")
	}
	privateDir := t.TempDir()
	keyPath, importPath := filepath.Join(privateDir, "key"), filepath.Join(privateDir, "import.json")
	// These are deliberately nonworking synthetic values. No Resolve or native
	// provider method is invoked by this storage/rotation-boundary fixture.
	raw, _ := json.Marshal(map[string]any{"access_token": "synthetic-access-not-working", "refresh_token": "synthetic-refresh-not-working", "client_id": "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com", "client_secret": "synthetic-client-not-working", "expires_at": time.Now().Add(time.Hour).UTC()})
	if os.WriteFile(importPath, raw, 0600) != nil {
		t.Fatal("private synthetic import creation failed")
	}
	if _, err = nativeFixtureProvider(ctx, pool, importPath, keyPath, limit); err != nil {
		t.Fatal("isolated encrypted import denied")
	}
	var stored string
	if pool.QueryRow(ctx, `SELECT value FROM system_settings WHERE key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY'`).Scan(&stored) != nil || stored == "" || strings.Contains(stored, "synthetic-") {
		t.Fatal("isolated credential persistence was not encrypted")
	}
	// Removal stands in for a readonly stale import becoming unavailable. A
	// restart must use its current encrypted row, not reread/reimport old OAuth.
	if os.Remove(importPath) != nil {
		t.Fatal("fixture import removal failed")
	}
	if _, err = nativeFixtureProvider(ctx, pool, importPath, keyPath, limit); err != nil {
		t.Fatal("restart depended on the old import")
	}
	var attempts int
	if pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&attempts) != nil || attempts != 0 {
		t.Fatal("credential preparation spent a native attempt")
	}
}
