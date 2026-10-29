// Operator-only journal review. No model, provider, runtime or delivery client.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func run() error {
	inputs := flag.String("inputs", "", "comma-separated exact stopped Input refs")
	apply := flag.Bool("apply", false, "commit verified review; default rolls back")
	flag.Parse()
	ids := strings.Split(*inputs, ",")
	if len(ids) == 0 || len(ids) > 8 || flag.NArg() != 0 {
		return errors.New("bounded exact input list required")
	}
	for _, id := range ids {
		if !regexp.MustCompile(`^input-[a-f0-9]{32}$`).MatchString(id) {
			return errors.New("invalid input reference")
		}
	}
	dsn := os.Getenv("BUTLER_POSTGRES_URL")
	if dsn == "" {
		return errors.New("private operator database configuration required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return errors.New("private database configuration unavailable")
	}
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return errors.New("database unavailable")
	}
	defer pool.Close()
	store := input.NewStore(pool)
	store.CommunicationEpoch = 1
	store.StoppedFinalizer = &communication.Store{Pool: pool, Inputs: store}
	for _, id := range ids {
		bounded, stop := context.WithTimeout(ctx, 2*time.Second)
		preview, reviewErr := store.ReviewStoppedInput(bounded, id, *apply)
		stop()
		if reviewErr != nil {
			return fmt.Errorf("review unavailable; original uncertainty retained: %w", reviewErr)
		}
		var classification, status string
		if err = pool.QueryRow(ctx, `SELECT f.classification,i.status FROM input_stopped_attempts f JOIN accepted_inputs i USING(input_id) WHERE f.input_id=$1`, id).Scan(&classification, &status); err != nil {
			return errors.New("review observation unavailable")
		}
		if err = json.NewEncoder(os.Stdout).Encode(map[string]any{"input": id, "applied": *apply, "classification": classification, "reviewed_classification": preview.Classification, "reviewed_reason": preview.Reason, "input_status": status, "old_work_replayed": false}); err != nil {
			return err
		}
		if preview.Classification != "known" || (*apply && classification != "known") {
			return errors.New("unresolved evidence remains; effects stay blocked")
		}
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		code := "unavailable"
		var pgerr *pgconn.PgError
		if errors.As(err, &pgerr) && regexp.MustCompile(`^[A-Z0-9]{5}$`).MatchString(pgerr.Code) {
			code = pgerr.Code
		}
		fmt.Fprintln(os.Stderr, "Stopped review did not complete; storage_code="+code+"; no raw storage diagnostics exported.")
		os.Exit(1)
	}
}
