package credentials

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestStoreIntegrationCRUD(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 4, MinConns: 1, MaxConnLifetime: time.Minute}, nil)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	migrationsDir := filepath.Clean(filepath.Join("..", "..", "migrations"))
	if err := store.RunMigrations(ctx, migrationsDir); err != nil {
		t.Fatalf("RunMigrations returned error: %v", err)
	}

	alias := "integration-github"
	_, _ = store.Pool().Exec(ctx, `DELETE FROM credentials WHERE alias = $1`, alias)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM credentials WHERE alias = $1`, alias)
	}()

	credentialStore := NewStore(store.Pool())
	created, err := credentialStore.Create(ctx, Record{Alias: alias, SecretType: "api_token", TargetType: "http", AllowedDomains: []string{"api.github.com"}, AllowedTools: []string{"http.request"}, ApprovalPolicy: ApprovalPolicyAlwaysConfirm, SecretRef: "secret://github/token", Status: StatusActive, MetadataJSON: `{"owner":"integration"}`})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("expected persisted record id")
	}

	loaded, err := credentialStore.GetByAlias(ctx, alias)
	if err != nil {
		t.Fatalf("GetByAlias returned error: %v", err)
	}
	if loaded.SecretRef != "secret://github/token" {
		t.Fatalf("unexpected secret ref %q", loaded.SecretRef)
	}

	updated, err := credentialStore.Update(ctx, Record{Alias: alias, SecretType: "api_token", TargetType: "http", AllowedDomains: []string{"api.github.com", "uploads.github.com"}, AllowedTools: []string{"http.request"}, ApprovalPolicy: ApprovalPolicyConfirmOnMutation, SecretRef: "secret://github/token", Status: StatusActive, MetadataJSON: `{"owner":"integration","rotated":true}`})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if len(updated.AllowedDomains) != 2 {
		t.Fatalf("unexpected updated domains: %+v", updated.AllowedDomains)
	}

	listed, err := credentialStore.List(ctx, false)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("expected active credentials in list")
	}

	revoked, err := credentialStore.Revoke(ctx, alias)
	if err != nil {
		t.Fatalf("Revoke returned error: %v", err)
	}
	if revoked.Status != StatusRevoked {
		t.Fatalf("expected revoked status, got %q", revoked.Status)
	}

	activeOnly, err := credentialStore.List(ctx, false)
	if err != nil {
		t.Fatalf("List active returned error: %v", err)
	}
	for _, item := range activeOnly {
		if item.Alias == alias {
			t.Fatal("expected revoked alias to be hidden from active-only listing")
		}
	}
}
