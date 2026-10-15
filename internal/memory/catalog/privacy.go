package catalog

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Preparation can persist before the first message exists. Source pins are
// authoritative for those runs too; deletion must erase their cached bodies
// without waiting for a message edge. Identity and external-effect rows stay.
func redactPinnedRunsTx(ctx context.Context, tx pgx.Tx, sources []string, deletionID string) error {
	if err := redactRunToolPayloadsTx(ctx, tx, sources, deletionID); err != nil {
		return err
	}
	queries := []string{
		`UPDATE accepted_inputs SET event_json=jsonb_build_object('privacy_redacted',TRUE,'deletion_id',$2::TEXT,'event_id',event_id,'task_id',task_id),checkpoint_json=jsonb_build_object('privacy_redacted',TRUE),status=CASE WHEN status IN ('completed','failed','cancelled','expired') THEN status ELSE 'cancelled' END,claim_id='',claim_expires_at=NULL,error_code='privacy_redacted',error_message='',updated_at=clock_timestamp() WHERE run_id IN(SELECT run_id FROM memory_run_sources WHERE source_id=ANY($1))`,
		`UPDATE runs SET metadata_json=jsonb_build_object('privacy_redacted',TRUE,'deletion_id',$2::TEXT) WHERE run_id IN(SELECT run_id FROM memory_run_sources WHERE source_id=ANY($1))`,
	}
	for _, q := range queries {
		if _, e := tx.Exec(ctx, q, sources, deletionID); e != nil {
			return e
		}
	}
	return nil
}

// Redaction is one-way content removal. IDs, original hashes, effect evidence,
// verdicts and sent/unknown receipts remain truthful; no external undo is claimed.
func redactMessagesTx(ctx context.Context, tx pgx.Tx, sources []string, deletionID string) error {
	var messageIDs []string
	for _, id := range sources {
		if strings.HasPrefix(id, "message:") {
			messageIDs = append(messageIDs, strings.TrimPrefix(id, "message:"))
		}
	}
	if len(messageIDs) == 0 {
		return nil
	}
	queries := []string{
		`UPDATE run_final_results SET content='',privacy_redacted=TRUE,redaction_id=$2 WHERE message_id=ANY($1) AND NOT privacy_redacted`,
		`UPDATE delivery_outbox SET content='',privacy_redacted=TRUE,redaction_id=$2,status=CASE WHEN status='sending' THEN 'delivery_unknown' WHEN status IN ('pending','result_ready') THEN 'failed' ELSE status END,error_code='privacy_redacted',claim_id='',claim_until=NULL,updated_at=clock_timestamp() WHERE result_id IN(SELECT result_id FROM run_final_results WHERE message_id=ANY($1)) AND NOT privacy_redacted`,
		`UPDATE artifacts SET title='',summary='',content_text='',content_json='{}',privacy_redacted=TRUE,redaction_id=$2,updated_at=clock_timestamp() WHERE run_id IN(SELECT run_id FROM messages WHERE message_id=ANY($1)) AND NOT privacy_redacted`,
		`UPDATE task_results SET summary='',privacy_redacted=TRUE,redaction_id=$2 WHERE run_id IN(SELECT run_id FROM messages WHERE message_id=ANY($1)) AND NOT privacy_redacted`,
		`UPDATE accepted_inputs SET event_json=jsonb_build_object('privacy_redacted',TRUE,'deletion_id',$2::TEXT,'event_id',event_id,'task_id',task_id),checkpoint_json=jsonb_build_object('privacy_redacted',TRUE),status=CASE WHEN status IN ('completed','failed','cancelled','expired') THEN status ELSE 'cancelled' END,claim_id='',claim_expires_at=NULL,error_code='privacy_redacted',error_message='',updated_at=clock_timestamp() WHERE run_id IN(SELECT run_id FROM messages WHERE message_id=ANY($1))`,
		`UPDATE messages SET content='',metadata=jsonb_build_object('privacy_redacted',TRUE,'deletion_id',$2::TEXT,'butler_provenance',jsonb_build_object('kind','memory_derived','sensitivity','private','source_refs',jsonb_build_array('message:'||message_id))),privacy_redacted=TRUE,redaction_id=$2 WHERE message_id=ANY($1) AND NOT privacy_redacted`,
	}
	for _, q := range queries {
		if _, e := tx.Exec(ctx, q, messageIDs, deletionID); e != nil {
			return e
		}
	}
	return nil
}
