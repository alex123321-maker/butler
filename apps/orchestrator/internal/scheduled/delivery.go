package scheduled

import (
	"context"
	"errors"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/jackc/pgx/v5"
	"time"
)

type ReminderStore struct{ Jobs *Store }

const noticeColumns = `notification_id,occurrence_id,session_key,channel,chunk_index,content,status,attempt_count,claim_id,channel_ack,error_code`

func scanNotice(row interface{ Scan(...any) error }) (deliveryoutbox.Message, error) {
	var m deliveryoutbox.Message
	err := row.Scan(&m.NotificationID, &m.ResultID, &m.SessionKey, &m.Channel, &m.ChunkIndex, &m.Content, &m.Status, &m.AttemptCount, &m.ClaimID, &m.ChannelACK, &m.ErrorCode)
	return m, err
}
func (s ReminderStore) Claim(ctx context.Context, ttl time.Duration) (*deliveryoutbox.Message, error) {
	if s.Jobs == nil || ttl <= 0 || ttl > 30*time.Second {
		return nil, ErrDenied
	}
	tx, err := s.Jobs.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `UPDATE schedule_reminder_outbox SET status='delivery_unknown',error_code='sending_claim_expired',claim_id='',claim_until=NULL,updated_at=clock_timestamp() WHERE status='sending' AND claim_until<=clock_timestamp(); UPDATE schedule_reminder_outbox SET status='failed',error_code='delivery_deadline_exceeded',updated_at=clock_timestamp() WHERE status='pending' AND (deadline_at<=clock_timestamp() OR attempt_count>=6)`); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE schedule_reminder_outbox n SET status='failed',error_code='schedule_changed',updated_at=clock_timestamp() FROM scheduled_jobs q WHERE q.schedule_id=n.schedule_id AND n.status='pending' AND (q.revision<>n.schedule_revision OR q.state IN ('paused','blocked','cancelled') OR q.expires_at<=clock_timestamp())`); err != nil {
		return nil, err
	}
	claim, err := newID("notice_")
	if err != nil {
		return nil, err
	}
	m, err := scanNotice(tx.QueryRow(ctx, `UPDATE schedule_reminder_outbox SET status='sending',claim_id=$1,claim_until=clock_timestamp()+$2::interval,attempt_count=attempt_count+1,updated_at=clock_timestamp() WHERE notification_id=(SELECT n.notification_id FROM schedule_reminder_outbox n WHERE n.status='pending' AND n.next_attempt_at<=clock_timestamp() AND n.deadline_at>clock_timestamp() AND n.attempt_count<6 AND NOT EXISTS(SELECT 1 FROM schedule_reminder_outbox p WHERE p.occurrence_id=n.occurrence_id AND p.chunk_index<n.chunk_index AND p.status<>'sent') ORDER BY created_at,notification_id FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING `+noticeColumns, claim, ttl.String()))
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
func (s ReminderStore) AuthorizeSend(ctx context.Context, m deliveryoutbox.Message) error {
	tx, err := s.Jobs.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var id string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT n.schedule_id,n.schedule_revision FROM schedule_reminder_outbox n JOIN sessions s ON s.session_key=n.session_key AND s.user_id=n.owner_id WHERE notification_id=$1 AND claim_id=$2 AND status='sending' AND claim_until>clock_timestamp() AND deadline_at>clock_timestamp() AND content=$3 AND n.session_key=$4 AND n.channel=$5 AND s.channel=n.channel`, m.NotificationID, m.ClaimID, m.Content, m.SessionKey, m.Channel).Scan(&id, &revision)
	if err != nil {
		return deliveryoutbox.ErrClaimLost
	}
	j, err := scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM scheduled_jobs WHERE schedule_id=$1 FOR SHARE`, id))
	if err != nil {
		return err
	}
	if j.Revision != revision || j.State == "cancelled" || j.State == "paused" || j.State == "blocked" {
		return ErrDenied
	}
	if err = s.Jobs.sourceTx(ctx, tx, j); err != nil {
		return err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	if _, err = s.Jobs.authorityTx(ctx, tx, j, now); err != nil {
		return err
	}
	// Read again after policy waits. Never repeat an uncertain send.
	if err = tx.QueryRow(ctx, `SELECT TRUE FROM schedule_reminder_outbox WHERE notification_id=$1 AND claim_id=$2 AND status='sending' AND claim_until>clock_timestamp() AND deadline_at>clock_timestamp()`, m.NotificationID, m.ClaimID).Scan(new(bool)); err != nil {
		return deliveryoutbox.ErrClaimLost
	}
	return tx.Commit(ctx)
}
func (s ReminderStore) Finish(ctx context.Context, m deliveryoutbox.Message, o deliveryoutbox.Outcome) error {
	state, code, delay := deliveryoutbox.DeliveryOutcome(m, o)
	tag, err := s.Jobs.Pool.Exec(ctx, `UPDATE schedule_reminder_outbox SET status=$3,channel_ack=$4,error_code=$5,next_attempt_at=clock_timestamp()+$6::interval,claim_id='',claim_until=NULL,updated_at=clock_timestamp() WHERE notification_id=$1 AND claim_id=$2 AND status='sending' AND claim_until>clock_timestamp()`, m.NotificationID, m.ClaimID, state, o.ACK, code, delay.String())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return deliveryoutbox.ErrClaimLost
	}
	return nil
}
