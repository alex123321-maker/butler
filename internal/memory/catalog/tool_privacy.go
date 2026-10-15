package catalog

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// Exact source/run pins also own cached tool and approval payloads. The durable
// tombstone closes late inserts and rewrites; effect verdicts, identities, hashes,
// timestamps and paid usage remain in their existing authoritative journals.
func redactRunToolPayloadsTx(ctx context.Context, tx pgx.Tx, sources []string, deletionID string) error {
	_, err := tx.Exec(ctx, `INSERT INTO memory_run_payload_redactions(run_id,redaction_id) SELECT DISTINCT affected.run_id,$2 FROM (SELECT run_id FROM memory_run_sources WHERE source_id=ANY($1) UNION SELECT run_id FROM messages WHERE 'message:'||message_id=ANY($1)) affected ON CONFLICT(run_id) DO NOTHING`, sources, deletionID)
	if err != nil {
		return err
	}
	for _, query := range []string{
		`UPDATE tool_calls t SET args='{}',result=jsonb_build_object('privacy_redacted',TRUE,'deletion_id',d.redaction_id),error='{}' FROM memory_run_payload_redactions d WHERE t.run_id=d.run_id AND d.redaction_id=$1`,
		`UPDATE approvals a SET args_json='{}',summary='',details_json='{}',resolution_reason='privacy_redacted',status=CASE WHEN status='pending' THEN 'failed' ELSE status END,updated_at=clock_timestamp() FROM memory_run_payload_redactions d WHERE a.run_id=d.run_id AND d.redaction_id=$1`,
		`UPDATE approval_events e SET reason='privacy_redacted',metadata_json=jsonb_build_object('privacy_redacted',TRUE,'deletion_id',d.redaction_id) FROM memory_run_payload_redactions d WHERE e.run_id=d.run_id AND d.redaction_id=$1`,
	} {
		if _, err = tx.Exec(ctx, query, deletionID); err != nil {
			return err
		}
	}
	return nil
}
