package credentials

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
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

func TestToolCallBrokerWritesAuditLog(t *testing.T) {
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
	if err := store.RunMigrations(ctx, filepath.Clean(filepath.Join("..", "..", "migrations"))); err != nil {
		t.Fatalf("RunMigrations returned error: %v", err)
	}
	alias := "integration-audit"
	_, _ = store.Pool().Exec(ctx, `DELETE FROM credential_audit_logs WHERE alias = $1`, alias)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM credentials WHERE alias = $1`, alias)
	defer func() {
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM credential_audit_logs WHERE alias = $1`, alias)
		_, _ = store.Pool().Exec(context.Background(), `DELETE FROM credentials WHERE alias = $1`, alias)
	}()
	if _, err := NewStore(store.Pool()).Create(ctx, Record{Alias: alias, SecretType: "api_token", TargetType: "http", AllowedDomains: []string{"api.github.com"}, AllowedTools: []string{"http.request"}, ApprovalPolicy: ApprovalPolicyAutoReadOnly, SecretRef: "env://BUTLER_TEST_CREDENTIAL_SECRET", Status: StatusActive}); err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	t.Setenv("BUTLER_TEST_CREDENTIAL_SECRET", "secret-token")
	broker := NewToolCallBroker(NewBroker(NewStore(store.Pool()), NewAuditStore(store.Pool())), EnvSecretResolver{})
	resolved, err := broker.ResolveToolCall(ctx, &toolbrokerv1.ToolCall{ToolCallId: "tool-1", RunId: "run-1", ToolName: "http.request", ArgsJson: `{"method":"GET","url":"https://api.github.com/repos"}`, CredentialRefs: []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: alias, Field: "token"}}, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2})
	if err != nil {
		t.Fatalf("ResolveToolCall returned error: %v", err)
	}
	if len(resolved) != 1 || resolved[0].Value != "secret-token" {
		t.Fatalf("unexpected resolved credentials: %+v", resolved)
	}
	var runID, toolCallID, decision string
	if err := store.Pool().QueryRow(ctx, `SELECT run_id, tool_call_id, decision FROM credential_audit_logs WHERE alias = $1 ORDER BY created_at DESC LIMIT 1`, alias).Scan(&runID, &toolCallID, &decision); err != nil {
		t.Fatalf("QueryRow returned error: %v", err)
	}
	if runID != "run-1" || toolCallID != "tool-1" || decision != AuditDecisionAllowed {
		t.Fatalf("unexpected audit log row run_id=%q tool_call_id=%q decision=%q", runID, toolCallID, decision)
	}
}
