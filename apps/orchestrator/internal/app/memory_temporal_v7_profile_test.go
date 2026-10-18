package app

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const temporalV7WholeDuration = 16 * time.Hour
const temporalV7OuterDuration = 975 * time.Minute
const temporalV7ForegroundDuration = 40 * time.Minute
const temporalV7BrokerDuration = temporalV7OuterDuration

func temporalNativeVersion(h *temporalV5History) int {
	if h != nil && h.SuccessorV7 != nil {
		return 7
	}
	if h != nil && h.SuccessorV6 != nil {
		return 6
	}
	return 0
}

func temporalClaimNativeWindow(ctx context.Context, pool *pgxpool.Pool, version int) error {
	statement := ""
	switch version {
	case 6:
		statement = `CREATE TABLE IF NOT EXISTS public.issue88_native_windows(window_id TEXT PRIMARY KEY);INSERT INTO public.issue88_native_windows VALUES('temporal-explicit-v6');CREATE SCHEMA memory_temporal_native_v6`
	case 7:
		statement = `CREATE TABLE IF NOT EXISTS public.issue88_native_windows(window_id TEXT PRIMARY KEY);INSERT INTO public.issue88_native_windows VALUES('temporal-explicit-v7');CREATE SCHEMA memory_temporal_native_v7`
	default:
		return errors.New("unsupported fixed temporal native window")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, statement); err != nil {
		return errors.New("one-use native schema/window unavailable")
	}
	return tx.Commit(ctx)
}
