package postgres

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/config"
)

func TestConfigFromShared(t *testing.T) {
	shared := config.PostgresConfig{
		URL:             "postgres://localhost:5432/butler",
		MaxConns:        12,
		MinConns:        3,
		MaxConnLifetime: 30,
		MigrationsDir:   "migrations",
	}

	got := ConfigFromShared(shared)
	if got.URL != shared.URL {
		t.Fatalf("expected URL %q, got %q", shared.URL, got.URL)
	}
	if got.MaxConns != shared.MaxConns || got.MinConns != shared.MinConns {
		t.Fatal("expected pool sizes to be copied")
	}
	if got.MaxConnLifetime != 30*time.Second {
		t.Fatalf("expected lifetime conversion, got %s", got.MaxConnLifetime)
	}
	if got.MigrationsDir != shared.MigrationsDir {
		t.Fatalf("expected migrations dir %q, got %q", shared.MigrationsDir, got.MigrationsDir)
	}
}

func TestMigrationFilesSortsUpMigrations(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "003_third.up.sql"), "SELECT 1;")
	writeFile(t, filepath.Join(dir, "001_first.up.sql"), "SELECT 1;")
	writeFile(t, filepath.Join(dir, "002_second.down.sql"), "SELECT 1;")
	writeFile(t, filepath.Join(dir, "002_second.up.sql"), "SELECT 1;")

	files, err := migrationFiles(dir)
	if err != nil {
		t.Fatalf("migrationFiles returned error: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 up migrations, got %d", len(files))
	}
	if !strings.HasSuffix(files[0], "001_first.up.sql") || !strings.HasSuffix(files[1], "002_second.up.sql") || !strings.HasSuffix(files[2], "003_third.up.sql") {
		t.Fatalf("unexpected order: %v", files)
	}
}

func TestOpenIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	store, err := Open(ctx, Config{
		URL:             dsn,
		MaxConns:        4,
		MinConns:        1,
		MaxConnLifetime: time.Minute,
	}, log)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	if err := store.HealthCheck(ctx); err != nil {
		t.Fatalf("HealthCheck returned error: %v", err)
	}

	if !strings.Contains(buf.String(), "postgres connected") {
		t.Fatalf("expected connection log entry, got %q", buf.String())
	}
	if strings.Contains(buf.String(), dsn) {
		t.Fatal("expected DSN to be masked in logs")
	}
}

func TestRunMigrationsIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := Open(ctx, Config{URL: dsn, MaxConns: 4, MinConns: 1, MaxConnLifetime: time.Minute}, nil)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer store.Close()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "001_create_t004_probe.up.sql"), `DROP TABLE IF EXISTS t004_probe; CREATE TABLE t004_probe (id INT PRIMARY KEY);`)
	writeFile(t, filepath.Join(dir, "002_insert_t004_probe.up.sql"), `INSERT INTO t004_probe (id) VALUES (1) ON CONFLICT DO NOTHING;`)

	if err := store.RunMigrations(ctx, dir); err != nil {
		t.Fatalf("RunMigrations returned error: %v", err)
	}
	if err := store.RunMigrations(ctx, dir); err != nil {
		t.Fatalf("RunMigrations should be idempotent, got: %v", err)
	}

	var count int
	if err := store.Pool().QueryRow(ctx, `SELECT COUNT(*) FROM t004_probe`).Scan(&count); err != nil {
		t.Fatalf("failed to query probe table: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected single migration row, got %d", count)
	}

	_, _ = store.Pool().Exec(ctx, `DROP TABLE IF EXISTS t004_probe`)
	_, _ = store.Pool().Exec(ctx, `DELETE FROM schema_migrations WHERE version IN ('001_create_t004_probe.up.sql', '002_insert_t004_probe.up.sql')`)
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}
