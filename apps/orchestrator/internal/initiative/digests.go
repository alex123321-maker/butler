package initiative

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/jackc/pgx/v5"
)

type DigestStore struct{ Store *Store }
type digestSource struct {
	ID       string `json:"source_id"`
	Revision int64  `json:"revision"`
}

// QueueDigests groups only admission/verified-result changes. Content consists
// of counts, never source text, error logs, model output, or credentials. The
// owner explicitly selects the session channel in source configuration.
func (s *Store) QueueDigests(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT s.session_key,s.owner_id FROM initiative_sources s JOIN initiatives i ON i.source_id=s.source_id
	 WHERE s.enabled AND s.source_json->>'digest_channel' IN ('web','telegram') AND i.status IN ('promoted','completed')
	 AND NOT EXISTS(SELECT 1 FROM initiative_digest_events e WHERE e.initiative_id=i.initiative_id AND e.state=i.status)
	 ORDER BY s.session_key,s.owner_id LIMIT 50`)
	if err != nil {
		return err
	}
	type group struct{ key, owner string }
	groups := []group{}
	for rows.Next() {
		var g group
		if err = rows.Scan(&g.key, &g.owner); err != nil {
			rows.Close()
			return err
		}
		groups = append(groups, g)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, g := range groups {
		if err = s.queueDigest(ctx, g.key, g.owner); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) queueDigest(ctx context.Context, key, owner string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var storedOwner, channel string
	if err = tx.QueryRow(ctx, `SELECT user_id,channel FROM sessions WHERE session_key=$1 FOR SHARE`, key).Scan(&storedOwner, &channel); err != nil {
		return err
	}
	if storedOwner != owner || (channel != "web" && channel != "telegram") {
		return ErrInvalid
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "initiative_digest:"+owner); err != nil {
		return err
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return err
	}
	bucket := now.UTC().Truncate(6 * time.Hour)
	var blocked bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM initiative_digest_outbox WHERE session_key=$1 AND bucket=$2)
	 OR (SELECT count(*) FROM initiative_digest_outbox WHERE owner_id=$3 AND created_at>=(date_trunc('day',$4::timestamptz AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'))>=4`, key, bucket, owner, now).Scan(&blocked); err != nil {
		return err
	}
	if blocked {
		return tx.Commit(ctx)
	}
	sourceRows, err := tx.Query(ctx, `SELECT s.source_json,s.revision,s.enabled FROM initiative_sources s
	 JOIN owner_goals g ON g.goal_id=s.goal_id AND g.owner_id=s.owner_id
	 JOIN action_mandates m ON m.mandate_id=s.mandate_id AND m.owner_id=s.owner_id AND m.goal_id=g.goal_id
	 WHERE s.session_key=$1 AND s.owner_id=$2 AND s.enabled AND s.source_json->>'digest_channel'=$3
	 AND g.status='active' AND g.expires_at>clock_timestamp() AND m.status='active' AND m.revoked_at IS NULL AND m.expires_at>clock_timestamp()
	 ORDER BY s.source_id LIMIT 200 FOR SHARE OF s`, key, owner, channel)
	if err != nil {
		return err
	}
	eligible := map[string]Source{}
	for sourceRows.Next() {
		source, e := scanSource(sourceRows)
		if e != nil {
			sourceRows.Close()
			return e
		}
		if active(source, now) {
			eligible[source.ID] = source
		}
	}
	sourceRows.Close()
	if err = sourceRows.Err(); err != nil {
		return err
	}
	type change struct{ id, state, source string }
	changes := []change{}
	admitted, completed := 0, 0
	stamps := []digestSource{}
	seen := map[string]bool{}
	changeRows, err := tx.Query(ctx, `SELECT i.initiative_id,i.status,i.source_id,i.source_revision FROM initiatives i JOIN initiative_sources s ON s.source_id=i.source_id
	 WHERE s.session_key=$1 AND i.owner_id=$2 AND i.status IN ('promoted','completed')
	 AND NOT EXISTS(SELECT 1 FROM initiative_digest_events e WHERE e.initiative_id=i.initiative_id AND e.state=i.status)
	 ORDER BY i.updated_at,i.initiative_id LIMIT 100 FOR UPDATE OF i`, key, owner)
	if err != nil {
		return err
	}
	for changeRows.Next() {
		var c change
		var rev int64
		if err = changeRows.Scan(&c.id, &c.state, &c.source, &rev); err != nil {
			changeRows.Close()
			return err
		}
		source, ok := eligible[c.source]
		if !ok || source.Revision != rev {
			continue
		}
		changes = append(changes, c)
		if c.state == "completed" {
			completed++
		} else {
			admitted++
		}
		if !seen[c.source] {
			stamps = append(stamps, digestSource{source.ID, source.Revision})
			seen[c.source] = true
		}
	}
	changeRows.Close()
	if err = changeRows.Err(); err != nil {
		return err
	}
	if len(changes) == 0 {
		return tx.Commit(ctx)
	}
	id := "initiative_digest:" + hash([]any{owner, key, bucket})
	content := fmt.Sprintf("Инициативы Butler: начато поручений — %d; проверено результатов — %d. Подробности доступны в интерфейсе.", admitted, completed)
	encoded, _ := json.Marshal(stamps)
	status := "pending"
	if channel == "web" {
		status = "result_ready"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO initiative_digest_outbox(notification_id,owner_id,session_key,bucket,channel,content,source_ids_json,status,deadline_at) VALUES($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9)`, id, owner, key, bucket, channel, content, encoded, status, now.Add(24*time.Hour)); err != nil {
		return err
	}
	for _, c := range changes {
		if _, err = tx.Exec(ctx, `INSERT INTO initiative_digest_events(initiative_id,state,notification_id) VALUES($1,$2,$3)`, c.id, c.state, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (d DigestStore) Claim(ctx context.Context, ttl time.Duration) (*deliveryoutbox.Message, error) {
	if d.Store == nil || ttl <= 0 || ttl > time.Minute {
		return nil, ErrInvalid
	}
	tx, err := d.Store.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE initiative_digest_outbox SET status='delivery_unknown',error_code='sending_claim_expired',claim_id='',claim_until=NULL,updated_at=clock_timestamp() WHERE status='sending' AND claim_until<=clock_timestamp();
	 UPDATE initiative_digest_outbox SET status='failed',error_code='delivery_deadline_exceeded',updated_at=clock_timestamp() WHERE status='pending' AND (deadline_at<=clock_timestamp() OR attempt_count>=6)`); err != nil {
		return nil, err
	}
	var m deliveryoutbox.Message
	var encoded []byte
	var owner string
	err = tx.QueryRow(ctx, `SELECT notification_id,session_key,channel,content,attempt_count,source_ids_json,owner_id FROM initiative_digest_outbox
	 WHERE status='pending' AND next_attempt_at<=clock_timestamp() AND deadline_at>clock_timestamp() AND attempt_count<6
	 ORDER BY created_at,notification_id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&m.NotificationID, &m.SessionKey, &m.Channel, &m.Content, &m.AttemptCount, &encoded, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	var stamps []digestSource
	if json.Unmarshal(encoded, &stamps) != nil || len(stamps) == 0 || len(stamps) > 100 {
		return nil, ErrInvalid
	}
	var now time.Time
	if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return nil, err
	}
	next := now
	for _, stamp := range stamps {
		source, e := scanSource(tx.QueryRow(ctx, `SELECT s.source_json,s.revision,s.enabled FROM initiative_sources s
	 JOIN sessions q ON q.session_key=s.session_key AND q.user_id=s.owner_id
	 JOIN owner_goals g ON g.goal_id=s.goal_id AND g.owner_id=s.owner_id
	 JOIN action_mandates a ON a.mandate_id=s.mandate_id AND a.owner_id=s.owner_id AND a.goal_id=g.goal_id
	 WHERE s.source_id=$1 AND s.owner_id=$2 AND s.session_key=$3 AND s.enabled AND q.channel=$4 AND s.source_json->>'digest_channel'=$4
	 AND g.status='active' AND g.expires_at>clock_timestamp() AND a.status='active' AND a.revoked_at IS NULL AND a.expires_at>clock_timestamp()`, stamp.ID, owner, m.SessionKey, m.Channel))
		if errors.Is(e, ErrNotFound) || (e == nil && source.Revision != stamp.Revision) {
			_, e = tx.Exec(ctx, `UPDATE initiative_digest_outbox SET status='failed',error_code='source_authority_changed',updated_at=clock_timestamp() WHERE notification_id=$1`, m.NotificationID)
			if e != nil {
				return nil, e
			}
			return nil, tx.Commit(ctx)
		}
		if e != nil {
			return nil, e
		}
		if !active(source, now) {
			resume := nextActive(source, now)
			if resume.After(next) {
				next = resume
			}
		}
	}
	if next.After(now) {
		if _, err = tx.Exec(ctx, `UPDATE initiative_digest_outbox SET next_attempt_at=$2 WHERE notification_id=$1`, m.NotificationID, next); err != nil {
			return nil, err
		}
		return nil, tx.Commit(ctx)
	}
	m.ClaimID, err = newID("claim_")
	if err != nil {
		return nil, err
	}
	m.AttemptCount++
	m.ResultID = m.NotificationID
	m.Status = "sending"
	if _, err = tx.Exec(ctx, `UPDATE initiative_digest_outbox SET status='sending',claim_id=$2,claim_until=clock_timestamp()+$3::interval,attempt_count=attempt_count+1,updated_at=clock_timestamp() WHERE notification_id=$1`, m.NotificationID, m.ClaimID, ttl.String()); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &m, nil
}

func (d DigestStore) Finish(ctx context.Context, m deliveryoutbox.Message, o deliveryoutbox.Outcome) error {
	status, code, delay := deliveryoutbox.DeliveryOutcome(m, o)
	tag, err := d.Store.pool.Exec(ctx, `UPDATE initiative_digest_outbox SET status=$3,channel_ack=$4,error_code=$5,next_attempt_at=clock_timestamp()+$6::interval,claim_id='',claim_until=NULL,updated_at=clock_timestamp() WHERE notification_id=$1 AND claim_id=$2 AND status='sending' AND claim_until>clock_timestamp()`, m.NotificationID, m.ClaimID, status, o.ACK, code, delay.String())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return deliveryoutbox.ErrClaimLost
	}
	return nil
}
