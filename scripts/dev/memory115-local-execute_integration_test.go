//go:build ignore

package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

func TestLocal115ActualPostgresRecoveryAndCAS(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_OPERATOR_POSTGRES_URL")
	if dsn == "" {
		t.Skip("explicit isolated operator database required")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil || cfg.Database != "memory115_operator_test" || cfg.Host != "127.0.0.1" || cfg.Port != 55451 {
		t.Fatal("dedicated local operator database required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal("isolated operator PostgreSQL unavailable")
	}
	defer conn.Close(context.Background())
	transaction := func(t *testing.T) pgx.Tx {
		t.Helper()
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
		return tx
	}
	exec := func(t *testing.T, tx pgx.Tx, sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal("synthetic operator fixture failed", err)
		}
	}
	for _, name := range []string{"closed_CAS", "live_window", "live_lease", "live_claim", "stale_source", "not_secret"} {
		t.Run(name, func(t *testing.T) {
			tx := transaction(t)
			exec(t, tx, `CREATE TABLE public.behavior76_windows(closed boolean);CREATE TABLE public.session_ownership(expires_at timestamptz);CREATE TABLE public.accepted_inputs(claim_expires_at timestamptz);CREATE TABLE public.system_settings(key text PRIMARY KEY,value text,is_secret boolean,updated_by text,updated_at timestamptz)`)
			exec(t, tx, `INSERT INTO public.system_settings(key,value,is_secret) VALUES($1,'synthetic-original',true)`, local115AuthKey)
			switch name {
			case "live_window":
				exec(t, tx, `INSERT INTO public.behavior76_windows VALUES(false)`)
			case "live_lease":
				exec(t, tx, `INSERT INTO public.session_ownership VALUES(clock_timestamp()+interval '1 hour')`)
			case "live_claim":
				exec(t, tx, `INSERT INTO public.accepted_inputs VALUES(clock_timestamp()+interval '1 hour')`)
			case "stale_source":
				exec(t, tx, `UPDATE public.system_settings SET value='synthetic-concurrent-refresh'`)
			case "not_secret":
				exec(t, tx, `UPDATE public.system_settings SET is_secret=false`)
			}
			err := local115Return(ctx, tx, "synthetic-original", "synthetic-new-durable-refresh")
			var actual string
			if tx.QueryRow(ctx, `SELECT value FROM public.system_settings WHERE key=$1`, local115AuthKey).Scan(&actual) != nil {
				t.Fatal("actual source readback failed")
			}
			if name == "closed_CAS" {
				if err != nil || actual != "synthetic-new-durable-refresh" {
					t.Fatal("actual closed-source CAS failed")
				}
			} else if err == nil || actual == "synthetic-new-durable-refresh" {
				t.Fatal("source ownership/CAS fence bypassed", name)
			}
		})
	}
	for _, name := range []string{"namespaced_idle", "namespaced_live_lease", "namespaced_live_claim", "incomplete_namespace"} {
		t.Run(name, func(t *testing.T) {
			tx := transaction(t)
			exec(t, tx, `CREATE TABLE public.behavior76_windows(closed boolean);CREATE TABLE public.system_settings(key text PRIMARY KEY,value text,is_secret boolean,updated_by text,updated_at timestamptz);CREATE SCHEMA retained_fixture;CREATE TABLE retained_fixture.session_ownership(expires_at timestamptz)`)
			exec(t, tx, `INSERT INTO public.system_settings(key,value,is_secret) VALUES($1,'synthetic-original',true)`, local115AuthKey)
			if name != "incomplete_namespace" {
				exec(t, tx, `CREATE TABLE retained_fixture.accepted_inputs(claim_expires_at timestamptz)`)
			}
			if name == "namespaced_live_lease" {
				exec(t, tx, `INSERT INTO retained_fixture.session_ownership VALUES(clock_timestamp()+interval '1 hour')`)
			}
			if name == "namespaced_live_claim" {
				exec(t, tx, `INSERT INTO retained_fixture.accepted_inputs VALUES(clock_timestamp()+interval '1 hour')`)
			}
			err := local115Return(ctx, tx, "synthetic-original", "synthetic-new-durable-refresh")
			var actual string
			if tx.QueryRow(ctx, `SELECT value FROM public.system_settings WHERE key=$1`, local115AuthKey).Scan(&actual) != nil {
				t.Fatal("namespaced source readback failed")
			}
			if name == "namespaced_idle" {
				if err != nil || actual != "synthetic-new-durable-refresh" {
					t.Fatal("actual retained-schema layout was rejected", err)
				}
			} else if err == nil || actual == "synthetic-new-durable-refresh" {
				t.Fatal("namespaced source fence bypassed", name)
			}
		})
	}
	for _, name := range []string{"no_import", "durable_refresh", "import_without_setting", "empty_allocated_domain", "entries_without_auth", "schema_without_claim", "wrong_claim"} {
		t.Run(name, func(t *testing.T) {
			tx := transaction(t)
			if name != "no_import" {
				exec(t, tx, `CREATE SCHEMA experience_native_v1`)
			}
			if name != "no_import" && name != "schema_without_claim" {
				exec(t, tx, `CREATE TABLE public.issue90_native_windows(window_id text,window_hash text,schema_name text,owner_id text)`)
				plan := "synthetic-plan"
				if name == "wrong_claim" {
					plan = "foreign-plan"
				}
				exec(t, tx, `INSERT INTO public.issue90_native_windows VALUES('experience-explicit-v1',$1,'experience_native_v1','owner')`, plan)
			}
			if name == "durable_refresh" {
				exec(t, tx, `CREATE TABLE experience_native_v1.system_settings(key text,value text,is_secret boolean)`)
				exec(t, tx, `INSERT INTO experience_native_v1.system_settings VALUES($1,'synthetic-latest-refresh',true)`, local115AuthKey)
			}
			if name == "entries_without_auth" {
				exec(t, tx, `CREATE TABLE experience_native_v1.experience_native_entries(n int);INSERT INTO experience_native_v1.experience_native_entries VALUES(1)`)
			}
			latest, source, err := local115Recover(ctx, tx, "synthetic-plan", "synthetic-original", name == "import_without_setting" || name == "durable_refresh")
			switch name {
			case "no_import", "empty_allocated_domain":
				if err != nil || latest != "synthetic-original" || source != "unused_original_before_provider_import" {
					t.Fatal("unused-original boundary failed", name, err)
				}
			case "durable_refresh":
				if err != nil || latest != "synthetic-latest-refresh" || source != "latest_durable_local_setting" {
					t.Fatal("durable recovery lost latest refresh", err)
				}
			default:
				if err == nil || latest != "" || source != "" {
					t.Fatal("uncertain domain/auth recovery was accepted", name)
				}
			}
		})
	}
	t.Run("pending_backend_must_exit_before_final_read", func(t *testing.T) {
		if _, err := conn.Exec(ctx, `CREATE TABLE public.operator_backend_refresh(value text);INSERT INTO public.operator_backend_refresh VALUES('synthetic-original')`); err != nil {
			t.Fatal("new synthetic backend fixture unavailable")
		}
		t.Cleanup(func() { _, _ = conn.Exec(context.Background(), `DROP TABLE public.operator_backend_refresh`) })
		other, err := pgx.ConnectConfig(ctx, cfg)
		if err != nil {
			t.Fatal("synthetic credential-writer backend unavailable")
		}
		defer other.Close(context.Background())
		if _, err = other.Exec(ctx, `BEGIN;UPDATE public.operator_backend_refresh SET value='synthetic-latest-durable'`); err != nil {
			t.Fatal("synthetic pending refresh unavailable")
		}
		blocked, stop := context.WithTimeout(ctx, 10*time.Millisecond)
		if local115WaitDatabaseExit(blocked, conn) == nil {
			stop()
			t.Fatal("live backend allowed premature final snapshot")
		}
		stop()
		if _, err = other.Exec(ctx, `COMMIT`); err != nil {
			t.Fatal("synthetic refresh commit failed")
		}
		if other.Close(ctx) != nil || local115WaitDatabaseExit(ctx, conn) != nil {
			t.Fatal("exited backend did not release final read")
		}
		var final string
		if conn.QueryRow(ctx, `SELECT value FROM public.operator_backend_refresh`).Scan(&final) != nil || final != "synthetic-latest-durable" {
			t.Fatal("final read did not observe latest durable refresh")
		}
	})
}
