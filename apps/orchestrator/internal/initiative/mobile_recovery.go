package initiative

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/butler/butler/internal/actionauth"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
)

// RecoverMobileInputs continues an incomplete, normally completed attempt using
// the existing task/input queue and the same root/mandate budget. It does not
// reopen paused/blocked tasks, infer from final text, or replay tool effects.
func (p *Promoter) RecoverMobileInputs(ctx context.Context) error {
	rows, err := p.Store.pool.Query(ctx, `SELECT t.task_id,t.session_key,t.owner_id,b.batch_id FROM tasks t JOIN initiatives i ON i.task_id=t.task_id JOIN mobile_attention_batches b ON b.batch_id=t.source_json->>'mobile_batch_id' JOIN initiative_sources s ON s.source_id=b.source_id JOIN runs r ON r.run_id=t.last_run_id WHERE t.status='active' AND r.status='completed' AND r.finished_at<clock_timestamp()-interval '30 seconds' AND b.finished_at IS NULL AND s.enabled AND b.source_revision=s.revision AND EXISTS(SELECT 1 FROM mobile_messages m WHERE m.batch_id=b.batch_id AND m.state='pending') ORDER BY r.finished_at LIMIT 10`)
	if err != nil {
		return err
	}
	type item struct{ task, session, owner, batch string }
	items := []item{}
	for rows.Next() {
		var v item
		if err = rows.Scan(&v.task, &v.session, &v.owner, &v.batch); err != nil {
			rows.Close()
			return err
		}
		items = append(items, v)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}
	for _, v := range items {
		tx, err := p.Store.pool.Begin(ctx)
		if err != nil {
			return err
		}
		work := func() error {
			var owner string
			if err := tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR UPDATE`, v.session).Scan(&owner); err != nil {
				return err
			}
			if owner != v.owner {
				return ErrInvalid
			}
			var current bool
			if err := tx.QueryRow(ctx, `SELECT t.status='active' AND r.status='completed' FROM tasks t JOIN runs r ON r.run_id=t.last_run_id WHERE t.task_id=$1 FOR UPDATE OF t`, v.task).Scan(&current); err != nil {
				return err
			}
			if !current {
				return nil
			}
			var now time.Time
			if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
				return err
			}
			if _, err := actionauth.CheckTaskAuthorityTx(ctx, tx, v.task, v.owner, nil, now); err != nil {
				return nil
			}
			var source Source
			source, err = scanSource(tx.QueryRow(ctx, `SELECT s.source_json,s.revision,s.enabled FROM initiative_sources s JOIN mobile_attention_batches b ON b.source_id=s.source_id WHERE b.batch_id=$1 AND b.owner_id=$2 AND b.finished_at IS NULL AND b.source_revision=s.revision FOR SHARE OF s,b`, v.batch, v.owner))
			if err != nil {
				return nil
			}
			if !source.Enabled || source.ObserverKind != "mobile_inbox" {
				return nil
			}
			var count, active int
			if err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status IN ('queued','running')) FROM accepted_inputs WHERE task_id=$1`, v.task).Scan(&count, &active); err != nil {
				return err
			}
			if count >= 4 || active > 0 {
				return nil
			}
			id := fmt.Sprintf("mobile-recovery:%s:%d", v.batch, count)
			payload, _ := json.Marshal(map[string]any{"user_id": v.owner, "task_id": v.task, "text": MobileTaskGoal + " Продолжи незавершённую порцию; уже сохранённые решения не повторяй. Часовой пояс: " + source.TimeZone})
			_, _, err = p.Inputs.AcceptTx(ctx, tx, ingress.InputEvent{EventID: id, IdempotencyKey: id, EventType: runv1.InputEventType_INPUT_EVENT_TYPE_SCHEDULED_INTERNAL_EVENT, SessionKey: v.session, Source: "initiative", PayloadJSON: string(payload), CreatedAt: now}, v.owner, v.task)
			return err
		}
		err = work()
		if err == nil {
			err = tx.Commit(ctx)
		} else {
			_ = tx.Rollback(ctx)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
