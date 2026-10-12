package main

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresAdmissionSurvivesDuplicateConcurrencyExpiryAndRestart(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_COMPARE_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("fixture pool failed")
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `CREATE SCHEMA pair_fixture`); err != nil {
		t.Fatal("fixture schema failed")
	}
	pool.Close()
	pool, err = pgxpool.New(ctx, dsn+"&search_path=pair_fixture")
	if err != nil {
		t.Fatal("fixture schema pool failed")
	}
	defer pool.Close()
	p := pgAdmission{pool: pool, root: "/src"}
	if p.init(ctx) != nil {
		t.Fatal("fixture init failed")
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	admitted := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if id, err := p.Reserve(ctx, "cli"); err == nil {
				if id != 19 {
					t.Error("wrong carried attempt")
				}
				mu.Lock()
				admitted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if admitted != 1 {
		t.Fatal("duplicate admission")
	}
	if p.init(ctx) == nil {
		t.Fatal("restart reset deadline or counter")
	}
	if id, err := p.Reserve(ctx, "go"); err != nil || id != 20 {
		t.Fatal("second distinct client denied")
	}
	if _, err := p.Reserve(ctx, "go"); err == nil {
		t.Fatal("third attempt admitted")
	}
	if p.Observe(ctx, 19, observation{Attempt: 19, Client: "cli", Reason: "UNKNOWN", Outcome: "HTTP_REJECTED"}) != nil {
		t.Fatal("observation save failed")
	}
	var data []byte
	if pool.QueryRow(ctx, `SELECT observation FROM diagnostic_attempt WHERE id=19`).Scan(&data) != nil {
		t.Fatal("observation read failed")
	}
	var saved map[string]any
	if json.Unmarshal(data, &saved) != nil || saved["input_tokens"] != nil || saved["output_tokens"] != nil {
		t.Fatal("unknown usage turned into zero")
	}
	if _, err = pool.Exec(ctx, `UPDATE diagnostic_window SET deadline=clock_timestamp()-interval '1 second'`); err != nil {
		t.Fatal("fixture expiry failed")
	}
	if _, err = p.Reserve(ctx, "cli"); err == nil {
		t.Fatal("expired replay admitted")
	}
	var spent int
	if pool.QueryRow(ctx, `SELECT spent FROM diagnostic_window`).Scan(&spent) != nil || spent != 2 {
		t.Fatal("spent allowance refunded")
	}
}

func TestNative8192CarriesComparisonAndAdmitsOnlyAttempt21(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_COMPARE_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("fixture pool failed")
	}
	defer pool.Close()
	p := pgAdmission{pool: pool, root: "/src", native8192: true}
	if p.init(ctx) != nil {
		t.Fatal("carried fixture init failed")
	}
	if _, err := p.Reserve(ctx, "cli"); err == nil {
		t.Fatal("CLI admitted to native-only profile")
	}
	if id, err := p.Reserve(ctx, "go"); err != nil || id != 21 {
		t.Fatal("attempt20 or allowance was reset")
	}
	if _, err := p.Reserve(ctx, "go"); err == nil {
		t.Fatal("second native call admitted")
	}
	if p.init(ctx) == nil {
		t.Fatal("restart reopened native profile")
	}
	var base, spent, reserved int
	if pool.QueryRow(ctx, `SELECT historical,spent FROM diagnostic_window`).Scan(&base, &spent) != nil || base != 20 || spent != 1 {
		t.Fatal("carried spending changed")
	}
	if pool.QueryRow(ctx, `SELECT reserved_output_tokens FROM diagnostic_attempt WHERE id=21`).Scan(&reserved) != nil || reserved != 8192 {
		t.Fatal("output reservation lost")
	}
}
