// Record immutable research evidence in the independent experiment database.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if run() != nil {
		fmt.Fprintln(os.Stderr, "record_artifacts_failed")
		os.Exit(1)
	}
}
func run() error {
	ref := flag.String("database-file", "", "private isolated database URL file")
	id := flag.String("run", "", "existing inventory run ID")
	flag.Parse()
	if *ref == "" || *id == "" || len(flag.Args()) == 0 {
		return fmt.Errorf("configuration")
	}
	raw, e := os.ReadFile(*ref)
	if e != nil {
		return e
	}
	cfg, e := pgx.ParseConfig(strings.TrimSpace(string(raw)))
	if e != nil || cfg.Database != "butler_emoji140_test" {
		return fmt.Errorf("database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, e := pgx.ConnectConfig(ctx, cfg)
	if e != nil {
		return e
	}
	defer db.Close(ctx)
	_, e = db.Exec(ctx, "CREATE TABLE IF NOT EXISTS emoji140_artifacts(run_id text NOT NULL REFERENCES emoji140_runs(id),name text NOT NULL,sha256 text NOT NULL,payload jsonb NOT NULL,recorded_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(run_id,name,sha256))")
	if e != nil {
		return e
	}
	tx, e := db.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	for _, path := range flag.Args() {
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		b, e := io.ReadAll(io.LimitReader(f, 32*1024*1024+1))
		f.Close()
		if e != nil || len(b) > 32*1024*1024 || !json.Valid(b) {
			return fmt.Errorf("artifact")
		}
		h := sha256.Sum256(b)
		hash := hex.EncodeToString(h[:])
		_, e = tx.Exec(ctx, "INSERT INTO emoji140_artifacts(run_id,name,sha256,payload) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", *id, filepath.Base(path), hash, b)
		if e != nil {
			return e
		}
		fmt.Printf("artifact=%s sha256=%s\n", filepath.Base(path), hash)
	}
	return tx.Commit(ctx)
}
