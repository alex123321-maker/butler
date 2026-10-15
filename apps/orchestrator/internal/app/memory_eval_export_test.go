package app

import (
	"context"
	"encoding/json"
	"errors"
)

// Explicit safe metadata allowlist excludes settings/provider credential stores.
// Export is performed from PostgreSQL after spend retention, never from guesses.
func (c *memoryEvalCollector) exportLedger(ctx context.Context) error {
	ledger := map[string]json.RawMessage{}
	for _, table := range []string{"fixture_memory_eval_window", "fixture_memory_eval_expenses", "fixture_memory_eval_attempts", "fixture_memory_eval_aux", "fixture_memory_eval_guards", "fixture_memory_eval_auth", "fixture_memory_eval_results", "execution_model_attempts", "execution_budget_reservations", "memory_aux_jobs", "memory_index_jobs", "memory_sources", "memory_source_revisions", "memory_object_sources", "memory_chunks", "semantic_index_generations", "tasks", "runs", "sessions", "session_ownership"} {
		var raw []byte
		if err := c.pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]'::jsonb) FROM `+table+` t`).Scan(&raw); err != nil {
			return errors.New("safe ledger export failed")
		}
		ledger[table] = raw
	}
	return c.writeEvidence("ledger.json", ledger)
}
