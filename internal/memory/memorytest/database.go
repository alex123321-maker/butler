// Package memorytest contains isolated PostgreSQL acceptance fixtures.
package memorytest

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Database(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated memory PostgreSQL required")
	}
	ctx := context.Background()
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("separate test database required")
	}
	cfg.MaxConns = 2
	admin, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = admin.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public`); e != nil {
		t.Fatal(e)
	}
	schema := "memory_" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	_, file, _, _ := runtime.Caller(0)
	paths, e := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations", "*.up.sql"))
	if e != nil {
		t.Fatal(e)
	}
	sort.Strings(paths)
	for _, path := range paths {
		data, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(data)); e != nil {
			t.Fatalf("%s: %v", filepath.Base(path), e)
		}
	}
	return pool
}
