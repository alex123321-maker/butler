package task

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
)

// ReconcileMobileResults is a code-owned verifier for the fixed notification
// processing criterion. Neither a model final nor a caller-supplied verdict can
// complete a task. The normal goal/mandate, catalog and delivery guards apply.
func (s *Store) ReconcileMobileResults(ctx context.Context) error {
	if _, ok := owneridentity.FromContext(ctx); ok {
		return ErrUnauthorized
	}
	if _, ok := ownership.TokenFromContext(ctx); ok {
		return ErrUnauthorized
	}
	rows, err := s.pool.Query(ctx, `SELECT t.task_id FROM tasks t JOIN initiatives i ON i.task_id=t.task_id JOIN mobile_attention_batches b ON b.batch_id=t.source_json->>'mobile_batch_id' JOIN runs r ON r.run_id=t.last_run_id JOIN run_final_results f ON f.run_id=r.run_id WHERE t.status='active' AND t.parent_task_id IS NULL AND i.source_id=b.source_id AND b.finished_at IS NOT NULL AND r.status='completed' AND r.task_revision=t.revision AND EXISTS(SELECT 1 FROM accepted_inputs a WHERE a.run_id=r.run_id AND a.task_id=t.task_id AND a.owner_id=t.owner_id AND a.status='completed') AND NOT EXISTS(SELECT 1 FROM accepted_inputs a WHERE a.task_id=t.task_id AND a.status IN ('queued','running')) ORDER BY t.created_at LIMIT 20`)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	for _, id := range ids {
		if err = s.verifyMobileResult(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) verifyMobileResult(ctx context.Context, id string) error {
	if s.verifiedResultSourceGuard == nil {
		return ErrUnauthorized
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Follow the normal session -> task -> goal -> source lock order.
	var session string
	if err = tx.QueryRow(ctx, `SELECT session_key FROM tasks WHERE task_id=$1`, id).Scan(&session); err != nil {
		return err
	}
	var own string
	if err = tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR UPDATE`, session).Scan(&own); err != nil {
		return err
	}
	before, err := scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM tasks WHERE task_id=$1 FOR UPDATE`, id))
	if err != nil {
		return err
	}
	if before.Status != "active" || before.ParentTaskID != "" || before.OwnerID != own {
		return nil
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, id, own, nil, now); err != nil {
		return nil
	}
	var batch, runID string
	err = tx.QueryRow(ctx, `SELECT b.batch_id,r.run_id FROM mobile_attention_batches b JOIN initiative_sources s ON s.source_id=b.source_id JOIN initiatives i ON i.source_id=s.source_id AND i.task_id=$1 JOIN mobile_agent_access a ON a.owner_id=b.owner_id JOIN runs r ON r.task_id=i.task_id AND r.run_id=$4 JOIN run_final_results f ON f.run_id=r.run_id WHERE b.batch_id=$2 AND b.owner_id=$3 AND s.owner_id=b.owner_id AND s.source_json->>'observer_kind'='mobile_inbox' AND s.enabled AND b.source_revision=s.revision AND a.enabled AND b.finished_at IS NOT NULL AND r.status='completed' AND r.task_revision=$5 AND f.task_revision=$5 AND EXISTS(SELECT 1 FROM accepted_inputs ai WHERE ai.run_id=r.run_id AND ai.task_id=$1 AND ai.owner_id=$3 AND ai.status='completed') AND NOT EXISTS(SELECT 1 FROM accepted_inputs ai WHERE ai.task_id=$1 AND ai.status IN ('queued','running')) FOR SHARE OF b,s,a,r`, id, before.Source["mobile_batch_id"], own, before.LastRunID, before.Revision).Scan(&batch, &runID)
	if err != nil {
		return nil
	}
	if s.verifiedResultSourceGuard(ctx, tx, runID) != nil {
		return nil
	}
	var total, pending, remembered, invalid int
	err = tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE m.state='pending'),count(*) FILTER(WHERE m.state='remembered'),count(*) FILTER(WHERE (m.state='remembered' AND m.result_source_id IS NULL) OR (m.result_source_id IS NOT NULL AND (s.status<>'active' OR s.owner_id<>m.owner_id OR EXISTS(SELECT 1 FROM memory_source_dependencies d JOIN memory_sources p ON p.source_id=d.parent_source_id WHERE d.child_source_id=s.source_id AND (p.status<>'active' OR p.head_revision<>d.parent_revision))))) FROM mobile_messages m LEFT JOIN memory_sources s ON s.source_id=m.result_source_id WHERE m.batch_id=$1 AND m.owner_id=$2`, batch, own).Scan(&total, &pending, &remembered, &invalid)
	if err != nil {
		return err
	}
	if total < 1 || pending != 0 || invalid != 0 {
		return nil
	}
	resultID, err := newID("result")
	if err != nil {
		return err
	}
	refs, _ := json.Marshal([]string{"mobile_batch:" + batch})
	criteria, _ := json.Marshal(before.CompletionCriteria)
	summary := fmt.Sprintf("Разобрал %d сообщений из разрешённых приложений. Сохранил полезных заметок: %d. Оригиналы и скрытый шум доступны для поиска.", total, remembered)
	_, err = tx.Exec(ctx, `INSERT INTO task_results(result_id,task_id,task_revision,run_id,summary,evidence_refs_json,verified_criteria_json,verdict,verifier) VALUES($1,$2,$3,$4,$5,$6,$7,'verified','mobile_batch_v1')`, resultID, id, before.Revision, runID, summary, refs, criteria)
	if err != nil {
		return err
	}
	// Recheck DB time after the evidence locks, as for owner verification.
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, id, own, nil, now); err != nil {
		return nil
	}
	after, err := scan(tx.QueryRow(ctx, `UPDATE tasks SET status='completed',revision=revision+1,constraints_revision=constraints_revision+1,completion_result_id=$2,finished_at=clock_timestamp(),updated_at=clock_timestamp() WHERE task_id=$1 RETURNING `+columns, id, resultID))
	if err != nil {
		return err
	}
	if remembered > 0 {
		if err = s.queueVerifiedNotificationEvidenceTx(ctx, tx, before, after, "mobile_batch_v1"); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
