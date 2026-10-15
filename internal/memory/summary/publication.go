package summary

import (
	"context"
	"github.com/jackc/pgx/v5"
)

func saveCoveredSummaryTx(ctx context.Context, tx pgx.Tx, s Snapshot, result string) (bool, error) {
	tag, err := tx.Exec(ctx, `WITH boundary AS MATERIALIZED (
 SELECT m.session_key,m.message_id,m.message_sequence FROM messages m WHERE m.session_key=$1 AND m.message_id=$3 AND m.message_sequence=$4 FOR KEY SHARE
),head AS (SELECT h.source_revision FROM transcript_heads h JOIN boundary b USING(session_key) FOR UPDATE OF h)
INSERT INTO session_summaries(session_key,summary,through_message_id,through_sequence,source_revision,result_source_id)
SELECT b.session_key,$2,b.message_id,b.message_sequence,h.source_revision,$6 FROM boundary b CROSS JOIN head h WHERE h.source_revision=$5
ON CONFLICT(session_key) DO UPDATE SET summary=EXCLUDED.summary,through_message_id=EXCLUDED.through_message_id,through_sequence=EXCLUDED.through_sequence,source_revision=EXCLUDED.source_revision,result_source_id=EXCLUDED.result_source_id,updated_at=clock_timestamp()
WHERE session_summaries.source_revision<EXCLUDED.source_revision OR (session_summaries.source_revision=EXCLUDED.source_revision AND session_summaries.through_sequence<EXCLUDED.through_sequence)`, s.SessionKey, s.Summary, s.ThroughMessageID, s.ThroughSequence, s.SourceRevision, result)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}
