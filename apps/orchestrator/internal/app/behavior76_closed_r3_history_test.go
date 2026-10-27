package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Closed r3 is evidence, never authority to replay its controls or retry its
// stopped arms. This additive consumer preserves all old raw/window validators.
func behavior76ClosedR3History(ctx context.Context, tx pgx.Tx, root string) ([]byte, error) {
	directory := filepath.Join(root, "native-local-response-v10-r3")
	if err := behavior76CheckRawFiles(directory, "/src20/experiments/behavior76/2026-10-26/native-local-response-v10-r3/r3-raw-inventory.json", "98447bb97b8f43b7932f6a25baa8fee0835dd4035f4bd693e3cb222d54c4dc4b", 3761); err != nil {
		return nil, err
	}
	manifest, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil || behavior76Hash(manifest) != "6d8e43f55a8abd42ea0033d66638f9857ee3b0402dbbdff751909f5c42171bb6" {
		return nil, errors.New("closed r3 manifest changed")
	}
	var exact bool
	if err = tx.QueryRow(ctx, `SELECT closed AND admitted=197 AND manifest=$1::jsonb FROM public.behavior76_windows WHERE id='issue76-fixture-day-20261026T195902'`, string(manifest)).Scan(&exact); err != nil || !exact {
		return nil, errors.New("closed r3 window changed")
	}
	if err = tx.QueryRow(ctx, `SELECT count(*)=197 AND min(n)=1 AND max(n)=197 AND count(total_tokens)=197 AND sum(total_tokens)=1751497 FROM public.behavior76_attempts WHERE window_id='issue76-fixture-day-20261026T195902'`).Scan(&exact); err != nil || !exact {
		return nil, errors.New("closed r3 expense changed")
	}
	if err = tx.QueryRow(ctx, `SELECT (SELECT count(*)=12 AND bool_and(closed) AND sum(admitted)=971 FROM public.behavior76_windows) AND count(*)=971 AND count(total_tokens)=959 AND sum(total_tokens)=7700211 AND count(*)FILTER(WHERE total_tokens IS NULL)=12 FROM public.behavior76_attempts`).Scan(&exact); err != nil || !exact {
		return nil, errors.New("closed r3 inclusive history changed")
	}
	prior, err := os.ReadFile(filepath.Join(root, "native-local-response-v10-r2", "behavior76_attempts.jsonl"))
	if err != nil {
		return nil, errors.New("original r2 history unavailable")
	}
	lines := strings.Split(strings.TrimSpace(string(prior)), "\n")
	if len(lines) != 774 {
		return nil, errors.New("original r2 history incomplete")
	}
	array := "[" + strings.Join(lines, ",") + "]"
	if err = tx.QueryRow(ctx, `SELECT count(*)=774 AND bool_and(a.window_id IS NOT NULL AND j.value=to_jsonb(a)) FROM jsonb_array_elements($1::jsonb) j(value) LEFT JOIN public.behavior76_attempts a ON a.window_id=j.value->>'window_id' AND a.n=(j.value->>'n')::integer`, array).Scan(&exact); err != nil || !exact {
		return nil, errors.New("original774 history changed")
	}
	return manifest, nil
}
