package initiative

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type ClaimedTick struct {
	ScheduleID       string
	ScheduleRevision int64
	SourceID         string
	SourceRevision   int64
	SlotKey          string
	ScheduledAt      time.Time
	ClaimID          string
}

func scanSchedule(row sourceRow) (Schedule, error) {
	var schedule Schedule
	var encoded []byte
	err := row.Scan(&encoded, &schedule.NextAt, &schedule.Revision, &schedule.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return schedule, ErrNotFound
	}
	if err != nil {
		return schedule, err
	}
	next, revision, enabled := schedule.NextAt, schedule.Revision, schedule.Enabled
	if err = json.Unmarshal(encoded, &schedule); err != nil {
		return schedule, err
	}
	schedule.NextAt, schedule.Revision, schedule.Enabled = next, revision, enabled
	return schedule, schedule.Validate()
}

// AdvanceDue persists a single latest tick per due schedule before advancing it.
// Pending work prevents another tick for that schedule until it is acknowledged.
// Source-before-schedule locking agrees with owner configuration updates.
func (s *Store) AdvanceDue(ctx context.Context, at time.Time, limit int) (int, error) {
	if at.IsZero() {
		return 0, ErrInvalid
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT schedule_id,source_id FROM initiative_schedules q WHERE enabled AND next_at<=$1 AND NOT EXISTS(SELECT 1 FROM initiative_ticks t WHERE t.schedule_id=q.schedule_id AND t.status='pending') ORDER BY next_at,schedule_id LIMIT $2`, at, limit)
	if err != nil {
		return 0, err
	}
	type key struct{ id, source string }
	keys := []key{}
	for rows.Next() {
		var k key
		if err = rows.Scan(&k.id, &k.source); err != nil {
			rows.Close()
			return 0, err
		}
		keys = append(keys, k)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	count := 0
	for _, k := range keys {
		advanced, err := s.advanceOne(ctx, k.id, k.source, at)
		if err != nil {
			return count, err
		}
		if advanced {
			count++
		}
	}
	return count, nil
}

func (s *Store) advanceOne(ctx context.Context, id, sourceID string, at time.Time) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	source, err := scanSource(tx.QueryRow(ctx, `SELECT source_json,revision,enabled FROM initiative_sources WHERE source_id=$1 FOR SHARE`, sourceID))
	if err != nil {
		return false, err
	}
	q, err := scanSchedule(tx.QueryRow(ctx, `SELECT schedule_json,next_at,revision,enabled FROM initiative_schedules WHERE schedule_id=$1 AND source_id=$2 FOR UPDATE SKIP LOCKED`, id, sourceID))
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var pending bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM initiative_ticks WHERE schedule_id=$1 AND status='pending')`, id).Scan(&pending); err != nil {
		return false, err
	}
	if pending {
		return false, nil
	}
	tick, due := q.Due(at)
	if !due {
		return false, nil
	}
	status, reason := "pending", "due"
	if !tick.ShouldObserve {
		status, reason = "skipped", "missed_window"
	}
	if !source.Enabled {
		status, reason = "skipped", "source_disabled"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO initiative_ticks(schedule_id,schedule_revision,slot_key,scheduled_at,observed_at,missed_count,status,source_revision,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT DO NOTHING`, id, q.Revision, tick.SlotKey, tick.ScheduledAt, at, tick.MissedCount, status, source.Revision, reason); err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE initiative_schedules SET next_at=$2,updated_at=clock_timestamp() WHERE schedule_id=$1`, id, tick.NextAt); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

// ClaimTick recovers an unacknowledged observation after the lease expires.
// Its deterministic event identity makes a lost acknowledgement harmless.
func (s *Store) ClaimTick(ctx context.Context, ttl time.Duration) (ClaimedTick, error) {
	if ttl < time.Second || ttl > 5*time.Minute {
		return ClaimedTick{}, ErrInvalid
	}
	claim, err := newID("tickclaim_")
	if err != nil {
		return ClaimedTick{}, err
	}
	var t ClaimedTick
	err = s.pool.QueryRow(ctx, `WITH head AS (
 SELECT schedule_id,schedule_revision,slot_key FROM initiative_ticks WHERE status='pending' AND (claim_expires_at IS NULL OR claim_expires_at<=clock_timestamp()) ORDER BY scheduled_at,schedule_id LIMIT 1 FOR UPDATE SKIP LOCKED
) UPDATE initiative_ticks t SET claim_id=$1,claim_expires_at=clock_timestamp()+$2*interval '1 millisecond' FROM head h,initiative_schedules q WHERE t.schedule_id=h.schedule_id AND t.schedule_revision=h.schedule_revision AND t.slot_key=h.slot_key AND q.schedule_id=t.schedule_id RETURNING t.schedule_id,t.schedule_revision,q.source_id,t.source_revision,t.slot_key,t.scheduled_at,t.claim_id`, claim, ttl.Milliseconds()).Scan(&t.ScheduleID, &t.ScheduleRevision, &t.SourceID, &t.SourceRevision, &t.SlotKey, &t.ScheduledAt, &t.ClaimID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return t, err
}

func (s *Store) FinishTick(ctx context.Context, t ClaimedTick, status, reason string) error {
	if status != "checked" && status != "skipped" && status != "failed" {
		return ErrInvalid
	}
	if !identifier.MatchString(reason) {
		return ErrInvalid
	}
	result, err := s.pool.Exec(ctx, `UPDATE initiative_ticks SET status=$5,reason=$6,claim_id='',claim_expires_at=NULL WHERE schedule_id=$1 AND schedule_revision=$2 AND slot_key=$3 AND claim_id=$4 AND status='pending' AND claim_expires_at>clock_timestamp()`, t.ScheduleID, t.ScheduleRevision, t.SlotKey, t.ClaimID, status, reason)
	if err == nil && result.RowsAffected() != 1 {
		return ErrConflict
	}
	return err
}
