package transcript

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ToolPin identifies the exact stored tool observation that was copied. It is
// metadata only; a future unrelated call is not part of this immutable set.
type ToolPin struct {
	ID       string `json:"tool_call_id"`
	RunID    string `json:"run_id"`
	Hash     string `json:"row_hash"`
	Revision int64  `json:"row_revision"`
}

type ToolSnapshot struct {
	SessionKey     string
	SourceRevision int64
	Calls          []ToolCall
	Pins           []ToolPin
}

// CaptureRunTools belongs to Transcript Store. Capture row data and its server
// fingerprint in one repeatable snapshot; Seal subsequently checks those exact
// rows again under locks. The historical head is audit data, not a global veto.
func (s *Store) CaptureRunTools(ctx context.Context, run string, limit int) (ToolSnapshot, error) {
	var out ToolSnapshot
	if s == nil || run == "" || limit < 1 || limit > 64 {
		return out, fmt.Errorf("bounded tool observation required")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, `SELECT r.session_key,h.source_revision FROM runs r JOIN transcript_heads h USING(session_key) WHERE r.run_id=$1`, run).Scan(&out.SessionKey, &out.SourceRevision); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT t.tool_call_id,t.run_id,t.tool_name,t.args::text,t.status,t.runtime_target,t.started_at,t.finished_at,COALESCE(t.result::text,'{}'),COALESCE(t.error::text,'{}'),CASE WHEN h.run_id=t.run_id AND h.alive THEN h.row_hash END,h.revision FROM tool_calls t LEFT JOIN memory_experience_tool_heads h USING(tool_call_id) WHERE t.run_id=$1 ORDER BY t.tool_call_id LIMIT $2`, run, limit+1)
	if err != nil {
		return out, err
	}
	out.Calls = []ToolCall{}
	out.Pins = []ToolPin{}
	for rows.Next() {
		var c ToolCall
		var digest string
		var revision int64
		if err = rows.Scan(&c.ToolCallID, &c.RunID, &c.ToolName, &c.ArgsJSON, &c.Status, &c.RuntimeTarget, &c.StartedAt, &c.FinishedAt, &c.ResultJSON, &c.ErrorJSON, &digest, &revision); err != nil {
			rows.Close()
			return out, err
		}
		out.Calls = append(out.Calls, c)
		out.Pins = append(out.Pins, ToolPin{ID: c.ToolCallID, RunID: c.RunID, Hash: digest, Revision: revision})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Calls) > limit {
		return ToolSnapshot{}, fmt.Errorf("tool observation exceeds bounded manifest")
	}
	return out, tx.Commit(ctx)
}
