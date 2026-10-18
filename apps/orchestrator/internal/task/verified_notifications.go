package task

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// NotifyRunResultTx reads only the task's immutable owner-configured delivery
// mode on the existing result transaction. Missing task/mode never grants quiet
// verified delivery; explicit069 creation already requires the actual column.
func (s *Store) NotifyRunResultTx(ctx context.Context, tx pgx.Tx, id string) (bool, error) {
	if id == "" {
		return true, nil
	}
	var mode string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(to_jsonb(tasks)->>'result_delivery','attempt') FROM tasks WHERE task_id=$1`, id).Scan(&mode); err != nil {
		return false, err
	}
	switch mode {
	case "attempt":
		return true, nil
	case "verified_task":
		return false, nil
	default:
		return false, ErrInvalid
	}
}

// AttemptDeliveryAllowed is a projection of actual run/session/task state for
// the external channel adapter. Model events cannot select a delivery mode.
func (s *Store) AttemptDeliveryAllowed(ctx context.Context, runID, sessionKey string) (bool, error) {
	var mode string
	err := s.pool.QueryRow(ctx, `SELECT CASE WHEN r.task_id IS NULL THEN 'attempt' ELSE COALESCE(to_jsonb(t)->>'result_delivery','attempt') END
 FROM runs r JOIN sessions s ON s.session_key=r.session_key LEFT JOIN tasks t ON t.task_id=r.task_id
 WHERE r.run_id=$1 AND r.session_key=$2 AND (r.task_id IS NULL OR (t.owner_id=s.user_id AND t.session_key=s.session_key))`, runID, sessionKey).Scan(&mode)
	if err != nil {
		return false, err
	}
	switch mode {
	case "attempt":
		return true, nil
	case "verified_task":
		return false, nil
	default:
		return false, ErrInvalid
	}
}

func verifiedDeliveryReady(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) (bool, error) {
	var ready bool
	err := q.QueryRow(ctx, `WITH owned AS(SELECT to_regclass(format('%I.tasks',current_schema())) tasks,to_regclass(format('%I.task_verified_notifications',current_schema())) notices)
 SELECT tasks IS NOT NULL AND notices IS NOT NULL
 AND EXISTS(SELECT 1 FROM pg_attribute,owned WHERE attrelid=tasks AND attname='result_delivery' AND atttypid='text'::regtype AND attnotnull AND NOT attisdropped)
 AND EXISTS(SELECT 1 FROM pg_trigger,owned WHERE tgrelid=tasks AND tgname='immutable_task_result_delivery' AND tgenabled='O' AND NOT tgisinternal)
 AND EXISTS(SELECT 1 FROM pg_trigger,owned WHERE tgrelid=notices AND tgname='immutable_verified_task_notification' AND tgenabled='O' AND NOT tgisinternal)
 FROM owned`).Scan(&ready)
	return ready, err
}

func (s *Store) VerifiedDeliveryReady(ctx context.Context) (bool, error) {
	return verifiedDeliveryReady(ctx, s.pool)
}

func (s *Store) queueVerifiedNotificationTx(ctx context.Context, tx pgx.Tx, before, after Record) error {
	if before.ResultDelivery != "verified_task" || before.ParentTaskID != "" || after.Status != "completed" {
		return nil
	}
	a, ok := owneridentity.FromContext(ctx)
	_, worker := ownership.TokenFromContext(ctx)
	if !ok || worker || a.OwnerID != before.OwnerID || (a.SessionKey != "" && a.SessionKey != before.SessionKey) {
		return ErrUnauthorized
	}
	return s.queueVerifiedNotificationEvidenceTx(ctx, tx, before, after, "owner")
}

// Called only by the owner wrapper or the fixed code-owned mobile verifier
// after its independent durable evidence checks. No synthetic owner actor.
func (s *Store) queueVerifiedNotificationEvidenceTx(ctx context.Context, tx pgx.Tx, before, after Record, expectedVerifier string) error {
	if before.ResultDelivery != "verified_task" || before.ParentTaskID != "" || after.Status != "completed" {
		return nil
	}
	if expectedVerifier != "owner" && expectedVerifier != "mobile_batch_v1" {
		return ErrUnauthorized
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if _, err := actionauth.CheckTaskAuthorityTx(ctx, tx, before.TaskID, before.OwnerID, nil, now); err != nil {
		return err
	}
	// A semantic owner result is distinct from a model final. This mode requires
	// the exact current unredacted complete attempt as its privacy/lifecycle pin;
	// a partial, deleted, unrelated or old attempt cannot generate this notice.
	var runID, channel, verifier string
	err := tx.QueryRow(ctx, `SELECT r.run_id,s.channel,r.verifier FROM task_results r
 JOIN tasks t ON t.task_id=r.task_id JOIN sessions s ON s.session_key=t.session_key AND s.user_id=t.owner_id
 JOIN runs x ON x.run_id=r.run_id AND x.task_id=t.task_id
 JOIN run_final_results f ON f.run_id=x.run_id AND f.task_id=t.task_id
 WHERE r.task_id=$1 AND r.result_id=$2 AND r.task_revision=$3 AND r.verdict='verified' AND r.verifier=$5
 AND NOT COALESCE((to_jsonb(r)->>'privacy_redacted')::boolean,false)
 AND f.task_revision=$3 AND f.constraints_revision=$4 AND f.answer_state='final' AND NOT f.privacy_redacted
 AND x.current_state='completed' AND x.status='completed'
 AND NOT COALESCE((t.working_json #>> '{scratch,privacy_redacted}')::boolean,false)
 FOR SHARE OF r,s,x,f`, before.TaskID, after.CompletionResultID, before.Revision, before.ConstraintsRevision, expectedVerifier).Scan(&runID, &channel, &verifier)
	if err != nil || (channel != "web" && channel != "telegram") {
		return ErrInvalid
	}
	if s.verifiedResultSourceGuard == nil {
		return ErrInvalid
	}
	if err = s.verifiedResultSourceGuard(ctx, tx, runID); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, before.TaskID, before.OwnerID, nil, now); err != nil {
		return err
	}
	state := "pending"
	if channel == "web" {
		state = "result_ready"
	}
	id := "verified-task:" + before.TaskID
	content := fmt.Sprintf("Butler: владелец подтвердил завершение поручения. Результат доступен в интерфейсе: /tasks/%s.", before.TaskID)
	if verifier == "mobile_batch_v1" {
		content = fmt.Sprintf("Butler: разбор уведомлений завершён, полезные заметки сохранены. Результат: /tasks/%s.", before.TaskID)
	}
	_, err = tx.Exec(ctx, `INSERT INTO task_verified_notifications(notification_id,task_id,result_id,task_revision,constraints_revision,owner_id,run_id,session_key,channel,content,status,deadline_at)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, id, before.TaskID, after.CompletionResultID, after.Revision, after.ConstraintsRevision, before.OwnerID, runID, before.SessionKey, channel, content, state, now.Add(24*time.Hour))
	return err
}

// VerifiedNotificationStore owns only the task result outbox. The common worker
// has channel capability and no model, tool, approval or task-execution API.
type VerifiedNotificationStore struct{ Tasks *Store }

const verifiedNotificationColumns = `notification_id,result_id,run_id,session_key,channel,0,content,status,attempt_count,claim_id,channel_ack,error_code`

func scanVerifiedMessage(row scanner) (deliveryoutbox.Message, error) {
	var m deliveryoutbox.Message
	err := row.Scan(&m.NotificationID, &m.ResultID, &m.RunID, &m.SessionKey, &m.Channel, &m.ChunkIndex, &m.Content, &m.Status, &m.AttemptCount, &m.ClaimID, &m.ChannelACK, &m.ErrorCode)
	return m, err
}

func (s *Store) VerifiedNotifications(ctx context.Context, id string) ([]deliveryoutbox.Message, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok {
		return nil, ErrUnauthorized
	}
	r, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if a.OwnerID != r.OwnerID || (a.SessionKey != "" && a.SessionKey != r.SessionKey) {
		return nil, ErrUnauthorized
	}
	rows, err := s.pool.Query(ctx, `SELECT `+verifiedNotificationColumns+` FROM task_verified_notifications WHERE task_id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []deliveryoutbox.Message{}
	for rows.Next() {
		m, err := scanVerifiedMessage(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (s VerifiedNotificationStore) Claim(ctx context.Context, ttl time.Duration) (*deliveryoutbox.Message, error) {
	if s.Tasks == nil || ttl <= 0 || ttl > 30*time.Second {
		return nil, ErrInvalid
	}
	tx, err := s.Tasks.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `UPDATE task_verified_notifications SET status='delivery_unknown',error_code='sending_claim_expired',claim_id='',claim_until=NULL,updated_at=clock_timestamp() WHERE status='sending' AND claim_until<=clock_timestamp()`); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE task_verified_notifications SET status='failed',error_code='delivery_deadline_exceeded',updated_at=clock_timestamp() WHERE status='pending' AND (deadline_at<=clock_timestamp() OR attempt_count>=6)`); err != nil {
		return nil, err
	}
	claimID, err := newID("notice-claim")
	if err != nil {
		return nil, err
	}
	m, err := scanVerifiedMessage(tx.QueryRow(ctx, `UPDATE task_verified_notifications SET status='sending',claim_id=$1,claim_until=clock_timestamp()+$2::interval,attempt_count=attempt_count+1,updated_at=clock_timestamp()
 WHERE notification_id=(SELECT notification_id FROM task_verified_notifications WHERE status='pending' AND next_attempt_at<=clock_timestamp() AND deadline_at>clock_timestamp() AND attempt_count<6 ORDER BY created_at,notification_id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING `+verifiedNotificationColumns, claimID, ttl.String()))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s VerifiedNotificationStore) AuthorizeSend(ctx context.Context, m deliveryoutbox.Message) error {
	if s.Tasks == nil {
		return deliveryoutbox.ErrClaimLost
	}
	tx, err := s.Tasks.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var taskID, owner string
	err = tx.QueryRow(ctx, `SELECT d.task_id,d.owner_id FROM task_verified_notifications d
 JOIN tasks t ON t.task_id=d.task_id JOIN task_results r ON r.task_id=t.task_id AND r.result_id=d.result_id
 JOIN sessions s ON s.session_key=d.session_key AND s.user_id=d.owner_id
 JOIN run_final_results f ON f.run_id=d.run_id
 WHERE d.notification_id=$1 AND d.claim_id=$2 AND d.status='sending' AND d.claim_until>clock_timestamp() AND d.deadline_at>clock_timestamp()
 AND d.content=$3 AND d.result_id=$4 AND d.run_id=$5 AND d.session_key=$6 AND d.channel=$7
 AND t.owner_id=d.owner_id AND t.status='completed' AND t.revision=d.task_revision AND t.constraints_revision=d.constraints_revision AND t.completion_result_id=d.result_id
 AND r.verdict='verified' AND r.verifier IN ('owner','mobile_batch_v1') AND NOT COALESCE((to_jsonb(r)->>'privacy_redacted')::boolean,false)
 AND NOT f.privacy_redacted AND f.answer_state='final'
 AND NOT COALESCE((t.working_json #>> '{scratch,privacy_redacted}')::boolean,false)
 FOR SHARE OF d,t,r,s,f`, m.NotificationID, m.ClaimID, m.Content, m.ResultID, m.RunID, m.SessionKey, m.Channel).Scan(&taskID, &owner)
	if err != nil {
		return deliveryoutbox.ErrClaimLost
	}
	if s.Tasks.verifiedResultSourceGuard == nil {
		return ErrInvalid
	}
	if err = s.Tasks.verifiedResultSourceGuard(ctx, tx, m.RunID); err != nil {
		return err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, owner, nil, now); err != nil {
		return err
	}
	// Authority locks may wait past expiry. Current source/final/task rows are
	// already held, so recheck the exact claim and goal policy on fresh DB time.
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if _, err = actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, owner, nil, now); err != nil {
		return err
	}
	var live bool
	if err = tx.QueryRow(ctx, `SELECT TRUE FROM task_verified_notifications WHERE notification_id=$1 AND claim_id=$2 AND status='sending' AND claim_until>clock_timestamp() AND deadline_at>clock_timestamp()`, m.NotificationID, m.ClaimID).Scan(&live); err != nil {
		return deliveryoutbox.ErrClaimLost
	}
	return tx.Commit(ctx)
}

func (s VerifiedNotificationStore) Finish(ctx context.Context, m deliveryoutbox.Message, o deliveryoutbox.Outcome) error {
	if s.Tasks == nil {
		return deliveryoutbox.ErrClaimLost
	}
	state, code, delay := deliveryoutbox.DeliveryOutcome(m, o)
	tag, err := s.Tasks.pool.Exec(ctx, `UPDATE task_verified_notifications SET status=$3,channel_ack=$4,error_code=$5,next_attempt_at=clock_timestamp()+$6::interval,claim_id='',claim_until=NULL,updated_at=clock_timestamp() WHERE notification_id=$1 AND claim_id=$2 AND status='sending' AND claim_until>clock_timestamp()`, m.NotificationID, m.ClaimID, state, o.ACK, code, delay.String())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return deliveryoutbox.ErrClaimLost
	}
	return nil
}
