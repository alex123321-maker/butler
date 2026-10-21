// Package memorytest contains isolated PostgreSQL acceptance fixtures.
package memorytest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Database(t *testing.T) *pgxpool.Pool {
	return database(t, 0)
}

// DatabaseAtVersion is for ordered migration rollback tests. It never drops an
// old migration underneath newer dependent schemas.
func DatabaseAtVersion(t *testing.T, version int) *pgxpool.Pool {
	t.Helper()
	if version < 1 || version > 999 {
		t.Fatal("invalid test schema version")
	}
	return database(t, version)
}

func database(t *testing.T, version int) *pgxpool.Pool {
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
	t.Cleanup(admin.Close)
	if _, e = admin.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public`); e != nil {
		t.Fatal(e)
	}
	// PostgreSQL fixtures run concurrently in separate package processes. A
	// wall-clock timestamp does not guarantee distinct schema names across them.
	var schemaID [16]byte
	if _, e = rand.Read(schemaID[:]); e != nil {
		t.Fatal("isolated memory schema identity unavailable")
	}
	schema := "memory_" + hex.EncodeToString(schemaID[:])
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
	})
	_, file, _, _ := runtime.Caller(0)
	paths, e := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations", "*.up.sql"))
	if e != nil {
		t.Fatal(e)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if version > 0 {
			number, e := strconv.Atoi(filepath.Base(path)[:3])
			if e != nil {
				t.Fatal(e)
			}
			if number > version {
				continue
			}
		}
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
