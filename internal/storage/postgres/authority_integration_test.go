package postgres

import (
	"context"
	"net/url"
	"os"
	"testing"
	"time"
)

func TestCommunicationAuthorityPoolAllocationIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	for _, c := range []struct{ max, min int32 }{{3, 0}, {3, 3}, {4, 0}, {4, 4}, {6, 5}} {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		db, err := OpenWithAuthorityReserve(ctx, Config{URL: dsn, MaxConns: c.max, MinConns: c.min}, nil)
		if err != nil {
			cancel()
			t.Fatal("capacity configuration rejected", c, err)
		}
		max, min := db.Pool().Config().MaxConns+db.AuthorityReadPool().Config().MaxConns, db.Pool().Config().MinConns+db.AuthorityReadPool().Config().MinConns
		if c.max >= 4 {
			if db.SourceReadPool() == db.Pool() || db.SourceReadPool() == db.AuthorityReadPool() {
				t.Fatal("source work can consume lease reserve")
			}
			max += db.SourceReadPool().Config().MaxConns
			min += db.SourceReadPool().Config().MinConns
		} else if db.SourceReadPool() != db.Pool() {
			t.Fatal("minimum capacity must keep sources on business pool")
		}
		if max != c.max || min != c.min || db.Pool().Config().MaxConns < 2 {
			t.Fatal("connection allowance changed", c, max, min)
		}
		if err := db.HealthCheck(ctx); err != nil {
			t.Fatal(err)
		}
		db.Close()
		cancel()
	}
	// DSN-only limits participate in the same allocation; a zero Config field
	// must not let the business pool inherit an oversized DSN minimum.
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("pool_max_conns", "4")
	q.Set("pool_min_conns", "4")
	u.RawQuery = q.Encode()
	db, err := OpenWithAuthorityReserve(t.Context(), Config{URL: u.String()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if db.Pool().Config().MaxConns != 2 || db.Pool().Config().MinConns != 2 || db.AuthorityReadPool().Config().MinConns != 1 || db.SourceReadPool().Config().MinConns != 1 {
		t.Fatal("DSN-only limits bypassed total allowance")
	}
}
