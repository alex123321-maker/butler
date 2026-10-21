package communication

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/jackc/pgx/v5"
	"time"
)

// DeliveryStore reuses the existing channel-only Worker and outcome policy.
// Bootstrap does not construct this store/worker or a real sender in this phase.
type DeliveryStore struct{ Core *Store }

func (d *DeliveryStore) Claim(ctx context.Context, ttl time.Duration) (*deliveryoutbox.Message, error) {
	if d == nil || d.Core == nil || d.Core.SourceGuard == nil || d.Core.ChannelGuard == nil || ttl <= 0 || ttl > time.Minute {
		return nil, ErrDenied
	}
	tx, e := d.Core.Pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(context.Background())
	_, e = tx.Exec(ctx, `UPDATE delivery_outbox SET status='delivery_unknown',claim_id='',claim_until=NULL,error_code='sending_claim_expired' WHERE publication_id IS NOT NULL AND status='sending' AND claim_until<=clock_timestamp()`)
	if e != nil {
		return nil, e
	}
	_, e = tx.Exec(ctx, `UPDATE delivery_outbox SET status='failed',error_code='delivery_deadline_exceeded' WHERE publication_id IS NOT NULL AND status='pending' AND (deadline_at<=clock_timestamp() OR attempt_count>=6)`)
	if e != nil {
		return nil, e
	}
	var raw [16]byte
	if _, e = rand.Read(raw[:]); e != nil {
		return nil, e
	}
	claim := hex.EncodeToString(raw[:])
	var m deliveryoutbox.Message
	e = tx.QueryRow(ctx, `UPDATE delivery_outbox SET status='sending',claim_id=$1,claim_until=clock_timestamp()+$2::interval,attempt_count=attempt_count+1 WHERE notification_id=(SELECT d.notification_id FROM delivery_outbox d JOIN communication_publications c USING(publication_id) WHERE d.publication_id IS NOT NULL AND d.status='pending' AND d.next_attempt_at<=clock_timestamp() AND d.deadline_at>clock_timestamp() AND d.attempt_count<6 AND NOT EXISTS(SELECT 1 FROM delivery_outbox p WHERE p.publication_id=d.publication_id AND p.binding_id=d.binding_id AND p.chunk_index<d.chunk_index AND p.status<>'sent') AND NOT EXISTS(SELECT 1 FROM delivery_outbox p JOIN communication_publications pc ON pc.publication_id=p.publication_id WHERE p.binding_id=d.binding_id AND pc.session_key=c.session_key AND pc.publication_sequence<c.publication_sequence AND p.status IN ('pending','sending')) ORDER BY c.publication_sequence,d.chunk_index FOR UPDATE OF d SKIP LOCKED LIMIT 1) RETURNING notification_id,run_id,session_key,channel,chunk_index,content,status,attempt_count,claim_id,publication_id,binding_id,binding_version,recipient_snapshot,representation_json::text`, claim, ttl.String()).Scan(&m.NotificationID, &m.RunID, &m.SessionKey, &m.Channel, &m.ChunkIndex, &m.Content, &m.Status, &m.AttemptCount, &m.ClaimID, &m.PublicationID, &m.BindingID, &m.BindingVersion, &m.Recipient, &m.RepresentationJSON)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, tx.Commit(ctx)
	}
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return &m, nil
}
func (d *DeliveryStore) AuthorizeSend(ctx context.Context, m deliveryoutbox.Message) error {
	if d.Core.SourceGuard == nil || d.Core.ChannelGuard == nil {
		return ErrDenied
	}
	tx, e := d.Core.Pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(context.Background())
	deny := func() error {
		_ = tx.Rollback(context.Background())
		finish, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		_, err := d.Core.Pool.Exec(finish, `UPDATE delivery_outbox SET status='failed',error_code='policy_denied',claim_id='',claim_until=NULL WHERE notification_id=$1 AND claim_id=$2 AND status='sending' AND claim_until>clock_timestamp()`, m.NotificationID, m.ClaimID)
		if err != nil {
			return err
		}
		return ErrDenied
	}
	var kind, taskID, owner string
	e = tx.QueryRow(ctx, `SELECT p.kind,COALESCE(p.task_id,''),p.owner_id FROM delivery_outbox d JOIN communication_publications p USING(publication_id) JOIN communication_bindings b ON b.binding_id=d.binding_id JOIN sessions s ON s.session_key=p.session_key LEFT JOIN tasks t ON t.task_id=p.task_id WHERE d.notification_id=$1 AND d.claim_id=$2 AND d.status='sending' AND d.claim_until>clock_timestamp() AND d.content=$3 AND d.recipient_snapshot=$4 AND d.representation_json=$5::jsonb AND d.run_id=$6 AND d.publication_id=$7 AND d.binding_id=$8 AND d.binding_version=$9 AND d.channel=$10 AND b.channel=d.channel AND b.version=d.binding_version AND b.owner_id=p.owner_id AND b.enabled AND b.verified AND b.recipient=d.recipient_snapshot AND s.user_id=p.owner_id AND (p.task_id IS NULL OR t.owner_id=p.owner_id AND t.revision=p.task_revision AND t.constraints_revision=p.constraints_revision AND t.status NOT IN ('cancelled','paused','blocked')) AND NOT COALESCE((to_jsonb(d)->>'privacy_redacted')::boolean,FALSE)`, m.NotificationID, m.ClaimID, m.Content, m.Recipient, m.RepresentationJSON, m.RunID, m.PublicationID, m.BindingID, m.BindingVersion, m.Channel).Scan(&kind, &taskID, &owner)
	if e != nil {
		return deny()
	}
	if kind == "widget" {
		var current bool
		if tx.QueryRow(ctx, `SELECT TRUE FROM communication_interactions WHERE publication_id=$1 AND state='open' AND NOT privacy_redacted AND (expires_at IS NULL OR expires_at>clock_timestamp())`, m.PublicationID).Scan(&current) != nil {
			return deny()
		}
	}
	if taskID != "" {
		if _, e = actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, owner, nil, time.Now().UTC()); e != nil {
			return deny()
		}
	}
	if e = d.Core.SourceGuard(ctx, tx, m.RunID); e != nil {
		return deny()
	}
	a, e := communicationcontrol.DecodeArgs("communication."+kind, m.RepresentationJSON)
	if e != nil {
		return deny()
	}
	if e = d.Core.ChannelGuard(ctx, tx, m.RunID, Binding{ID: m.BindingID, Channel: m.Channel, Recipient: m.Recipient, Version: m.BindingVersion}, a); e != nil {
		return deny()
	}
	// Policy/lock waits do not extend a sending claim or freeze wall-clock
	// authority. Revalidate the current target and deadlines after those waits.
	if e = d.Core.SourceGuard(ctx, tx, m.RunID); e != nil {
		return deny()
	}
	if taskID != "" {
		var now time.Time
		if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now) != nil {
			return deny()
		}
		if _, e = actionauth.CheckTaskAuthorityTx(ctx, tx, taskID, owner, nil, now); e != nil {
			return deny()
		}
	}
	// Keep this DB-clock check last: even a successful source read may have
	// waited beyond the claim or question deadline. The authority rows read
	// above remain locked through this transaction.
	var current bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM delivery_outbox d JOIN communication_publications p USING(publication_id) JOIN communication_bindings b ON b.binding_id=d.binding_id LEFT JOIN tasks t ON t.task_id=p.task_id WHERE d.notification_id=$1 AND d.claim_id=$2 AND d.status='sending' AND d.claim_until>clock_timestamp() AND NOT d.privacy_redacted AND NOT p.privacy_redacted AND b.enabled AND b.verified AND b.owner_id=p.owner_id AND b.version=d.binding_version AND b.recipient=d.recipient_snapshot AND (p.task_id IS NULL OR t.revision=p.task_revision AND t.constraints_revision=p.constraints_revision AND t.status NOT IN ('cancelled','paused','blocked')) AND (p.kind<>'widget' OR EXISTS(SELECT 1 FROM communication_interactions x WHERE x.publication_id=p.publication_id AND x.state='open' AND NOT x.privacy_redacted AND (x.expires_at IS NULL OR x.expires_at>clock_timestamp())))`, m.NotificationID, m.ClaimID).Scan(&current) != nil {
		return deny()
	}
	return tx.Commit(ctx)
}

func (d *DeliveryStore) Finish(ctx context.Context, m deliveryoutbox.Message, o deliveryoutbox.Outcome) error {
	return deliveryoutbox.NewStore(d.Core.Pool).Finish(ctx, m, o)
}

var _ deliveryoutbox.ClaimStore = (*DeliveryStore)(nil)
