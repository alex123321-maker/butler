package executionnotice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/deliveryoutbox"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDenied = errors.New("execution notice unavailable")

type Binding struct {
	ID, OwnerID, Channel, Recipient string
	Version                         int64
}
type Store struct {
	pool     *pgxpool.Pool
	bindings []Binding
}
type Notice struct {
	ID            string    `json:"notice_ref"`
	InputID       string    `json:"input_ref"`
	RunID         string    `json:"run_ref,omitempty"`
	Code          string    `json:"code"`
	Reason        string    `json:"reason,omitempty"`
	Text          string    `json:"text"`
	ApprovalRef   string    `json:"approval_ref,omitempty"`
	DeliveryState string    `json:"delivery_state"`
	CreatedAt     time.Time `json:"created_at"`
}

func New(ctx context.Context, pool *pgxpool.Pool, bindings []Binding) (*Store, error) {
	if pool == nil || len(bindings) == 0 || len(bindings) > 2 {
		return nil, ErrDenied
	}
	s := &Store{pool: pool, bindings: append([]Binding(nil), bindings...)}
	channels := map[string]bool{}
	for _, b := range s.bindings {
		if b.ID == "" || b.OwnerID == "" || b.Recipient == "" || b.Version < 1 || b.Channel != "web" && b.Channel != "telegram" {
			return nil, ErrDenied
		}
		if channels[b.Channel] || b.OwnerID != bindings[0].OwnerID {
			return nil, ErrDenied
		}
		channels[b.Channel] = true
		tag, err := pool.Exec(ctx, `INSERT INTO execution_notice_bindings(binding_id,binding_version,owner_id)
 SELECT binding_id,version,owner_id FROM communication_bindings WHERE binding_id=$1 AND version=$2 AND owner_id=$3 AND channel=$4 AND recipient=$5 AND enabled AND verified ON CONFLICT DO NOTHING`, b.ID, b.Version, b.OwnerID, b.Channel, b.Recipient)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() == 0 {
			var yes bool
			if err = pool.QueryRow(ctx, `SELECT TRUE FROM execution_notice_bindings n JOIN communication_bindings b USING(binding_id) WHERE n.binding_id=$1 AND n.binding_version=$2 AND n.owner_id=$3 AND b.owner_id=n.owner_id AND b.version=n.binding_version AND b.channel=$4 AND b.recipient=$5 AND b.enabled AND b.verified`, b.ID, b.Version, b.OwnerID, b.Channel, b.Recipient).Scan(&yes); err != nil {
				return nil, ErrDenied
			}
		}
	}
	return s, nil
}

// Read only the verified owner ingress of this composition. Durable activation
// survives restart; installing a new binding/version never adopts old backlog.
const snapshotsSQL = `SELECT i.input_id,COALESCE(i.run_id,''),i.session_key,i.status,i.checkpoint,
 CASE WHEN i.error_code<>'' THEN i.error_code ELSE COALESCE(i.checkpoint_json->'checkpoint_data'->>'reason','') END,
 COALESCE((SELECT a.approval_id FROM approvals a WHERE a.run_id=i.run_id AND a.status='pending' AND a.expires_at>clock_timestamp() ORDER BY a.requested_at DESC LIMIT 1),''),
 EXISTS(SELECT 1 FROM communication_publications p WHERE p.input_id=i.input_id AND p.purpose='result' AND NOT p.privacy_redacted),
 GREATEST(i.updated_at,CASE WHEN predecessor.status IN ('blocked','waiting_for_user') THEN predecessor.updated_at ELSE i.updated_at END),COALESCE(predecessor.status,'')
 FROM accepted_inputs i JOIN sessions ss ON ss.session_key=i.session_key AND ss.user_id=i.owner_id
 JOIN execution_notice_bindings n ON n.owner_id=i.owner_id
 JOIN communication_bindings b ON b.binding_id=n.binding_id AND b.version=n.binding_version AND b.owner_id=n.owner_id
 JOIN communication_bindings origin ON origin.binding_id=i.communication_ingress_json->>'BindingID' AND origin.version::text=i.communication_ingress_json->>'BindingVersion' AND origin.owner_id=i.owner_id AND origin.enabled AND origin.verified
 LEFT JOIN LATERAL(SELECT h.status,h.updated_at FROM accepted_inputs h WHERE i.status='queued' AND h.session_key=i.session_key AND h.owner_id=i.owner_id AND h.session_sequence<i.session_sequence AND h.status NOT IN ('completed','failed','expired','cancelled') ORDER BY h.session_sequence LIMIT 1) predecessor ON TRUE
 WHERE n.binding_id=$1 AND n.binding_version=$2 AND n.owner_id=$3 AND b.channel=$4 AND b.recipient=$5 AND b.enabled AND b.verified
 AND i.communication_ingress_epoch=1 AND i.communication_ingress_json->>'OwnerID'=i.owner_id AND i.accepted_at>=n.activated_at
 AND (COALESCE(i.communication_ingress_json->>'Kind','')<>'scheduled_occurrence' OR i.status IN ('blocked','failed','expired','waiting_for_user'))
 AND origin.binding_id=ANY($6::text[]) AND ($7='' OR i.input_id=$7)
 ORDER BY i.accepted_at DESC LIMIT 100`

func (s *Store) snapshots(ctx context.Context, tx pgx.Tx, b Binding, id string) ([]snapshot, error) {
	ids := make([]string, 0, len(s.bindings))
	for _, x := range s.bindings {
		if x.OwnerID == b.OwnerID {
			ids = append(ids, x.ID)
		}
	}
	rows, err := tx.Query(ctx, snapshotsSQL, b.ID, b.Version, b.OwnerID, b.Channel, b.Recipient, ids, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []snapshot
	for rows.Next() {
		var x snapshot
		if err = rows.Scan(&x.InputID, &x.RunID, &x.SessionKey, &x.Status, &x.Checkpoint, &x.ErrorCode, &x.ApprovalRef, &x.HasResult, &x.UpdatedAt, &x.PredecessorState); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func noticeID(input string, b Binding, code, reason, approval string, updated time.Time) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%d", input, b.ID, b.Version, code, reason, approval, updated.UnixNano())))
	return "execution:" + hex.EncodeToString(h[:])
}
func (s *Store) Sweep(ctx context.Context) error {
	for _, b := range s.bindings {
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		items, err := s.snapshots(ctx, tx, b, "")
		if err != nil {
			_ = tx.Rollback(context.Background())
			return err
		}
		for _, x := range items {
			code, reason := classify(x)
			if code == "" {
				continue
			}
			approval := ""
			if code == "approval_required" {
				approval = x.ApprovalRef
			}
			status := "pending"
			if b.Channel == "web" {
				status = "available"
			}
			if b.Channel == "telegram" && code == "finished" {
				status = "superseded"
			}
			_, err = tx.Exec(ctx, `INSERT INTO execution_notices(notice_id,input_id,owner_id,session_key,run_id,binding_id,binding_version,channel,recipient_snapshot,code,reason,approval_ref,status,source_updated_at)
 SELECT $1,$2,$3,$4,NULLIF($5,''),$6,$7,$8,$9,$10,$11,$12,$13,$14 WHERE NOT EXISTS(SELECT 1 FROM (SELECT code,reason,approval_ref FROM execution_notices WHERE input_id=$2 AND binding_id=$6 AND binding_version=$7 ORDER BY sequence DESC LIMIT 1) latest WHERE latest.code=$10 AND latest.reason=$11 AND latest.approval_ref=$12) ON CONFLICT DO NOTHING`, noticeID(x.InputID, b, code, reason, approval, x.UpdatedAt), x.InputID, b.OwnerID, x.SessionKey, x.RunID, b.ID, b.Version, b.Channel, b.Recipient, code, reason, approval, status, x.UpdatedAt)
			if err != nil {
				break
			}
		}
		if err != nil {
			_ = tx.Rollback(context.Background())
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) List(ctx context.Context, bindingID string) ([]Notice, error) {
	a, ok := owneridentity.FromContext(ctx)
	if !ok || a.Channel != "web" {
		return nil, ErrDenied
	}
	var b Binding
	for _, x := range s.bindings {
		if x.ID == bindingID && x.OwnerID == a.OwnerID && x.Channel == "web" && x.Recipient == a.SessionKey {
			b = x
		}
	}
	if b.ID == "" {
		return nil, ErrDenied
	}
	rows, err := s.pool.Query(ctx, `SELECT notice_id,input_id,COALESCE(run_id,''),code,reason,approval_ref,status,created_at FROM
 (SELECT DISTINCT ON(n.input_id) n.* FROM execution_notices n JOIN communication_bindings b ON b.binding_id=n.binding_id AND b.version=n.binding_version AND b.owner_id=n.owner_id AND b.recipient=n.recipient_snapshot WHERE n.binding_id=$1 AND n.binding_version=$2 AND n.owner_id=$3 AND b.channel='web' AND b.enabled AND b.verified ORDER BY n.input_id,n.sequence DESC) latest ORDER BY sequence DESC LIMIT 50`, b.ID, b.Version, b.OwnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notice{}
	for rows.Next() {
		var n Notice
		if err = rows.Scan(&n.ID, &n.InputID, &n.RunID, &n.Code, &n.Reason, &n.ApprovalRef, &n.DeliveryState, &n.CreatedAt); err != nil {
			return nil, err
		}
		n.Text = Text(n.Code, n.Reason)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *Store) Claim(ctx context.Context, ttl time.Duration) (*deliveryoutbox.Message, error) {
	if ttl <= 0 || ttl > time.Minute {
		return nil, ErrDenied
	}
	if err := s.Sweep(ctx); err != nil {
		return nil, err
	}
	var target Binding
	for _, b := range s.bindings {
		if b.Channel == "telegram" {
			target = b
		}
	}
	if target.ID == "" {
		return nil, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, `UPDATE execution_notices SET status='delivery_unknown',claim_id='',claim_until=NULL,error_code='sending_claim_expired' WHERE status='sending' AND claim_until<=clock_timestamp() AND binding_id=$1 AND binding_version=$2`, target.ID, target.Version)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE execution_notices n SET status='superseded' WHERE n.status='pending' AND n.binding_id=$1 AND n.binding_version=$2 AND EXISTS(SELECT 1 FROM execution_notices newer WHERE newer.input_id=n.input_id AND newer.binding_id=n.binding_id AND newer.binding_version=n.binding_version AND newer.sequence>n.sequence)`, target.ID, target.Version)
	if err != nil {
		return nil, err
	}
	var m deliveryoutbox.Message
	var code, reason string
	err = tx.QueryRow(ctx, `UPDATE execution_notices SET status='sending',claim_id=gen_random_uuid()::text,claim_until=clock_timestamp()+$1::interval,attempt_count=attempt_count+1 WHERE notice_id=(SELECT n.notice_id FROM execution_notices n JOIN communication_bindings b ON b.binding_id=n.binding_id AND b.version=n.binding_version AND b.owner_id=n.owner_id AND b.recipient=n.recipient_snapshot WHERE n.binding_id=$2 AND n.binding_version=$3 AND n.channel='telegram' AND n.status='pending' AND n.next_attempt_at<=clock_timestamp() AND n.deadline_at>clock_timestamp() AND n.attempt_count<6 AND b.enabled AND b.verified ORDER BY n.sequence FOR UPDATE OF n SKIP LOCKED LIMIT 1)
 RETURNING notice_id,COALESCE(run_id,''),session_key,binding_id,binding_version,recipient_snapshot,channel,claim_id,attempt_count,code,reason`, ttl.String(), target.ID, target.Version).Scan(&m.NotificationID, &m.RunID, &m.SessionKey, &m.BindingID, &m.BindingVersion, &m.Recipient, &m.Channel, &m.ClaimID, &m.AttemptCount, &code, &reason)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	m.ResultID = m.NotificationID
	m.Content = Text(code, reason)
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &m, nil
}
func (s *Store) AuthorizeSend(ctx context.Context, m deliveryoutbox.Message) error {
	var b Binding
	for _, x := range s.bindings {
		if x.ID == m.BindingID && x.Version == m.BindingVersion && x.Channel == m.Channel && x.Recipient == m.Recipient {
			b = x
		}
	}
	if b.ID == "" || b.Channel != "telegram" {
		return ErrDenied
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var id, code, reason, approval string
	err = tx.QueryRow(ctx, `SELECT input_id,code,reason,approval_ref FROM execution_notices WHERE notice_id=$1 AND claim_id=$2 AND status='sending' AND claim_until>clock_timestamp() AND deadline_at>clock_timestamp() AND binding_id=$3 AND binding_version=$4 AND recipient_snapshot=$5 FOR UPDATE`, m.NotificationID, m.ClaimID, b.ID, b.Version, b.Recipient).Scan(&id, &code, &reason, &approval)
	if err != nil || Text(code, reason) != m.Content {
		return ErrDenied
	}
	items, err := s.snapshots(ctx, tx, b, id)
	if err != nil {
		return err
	}
	current := len(items) == 1
	if current {
		c, r := classify(items[0])
		current = c == code && r == reason && (code != "approval_required" || items[0].ApprovalRef == approval)
	}
	if !current {
		_, err = tx.Exec(ctx, `UPDATE execution_notices SET status='superseded',claim_id='',claim_until=NULL WHERE notice_id=$1 AND claim_id=$2`, m.NotificationID, m.ClaimID)
		if err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return ErrDenied
	}
	var live bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM execution_notices n JOIN communication_bindings b ON b.binding_id=n.binding_id AND b.version=n.binding_version AND b.owner_id=n.owner_id AND b.recipient=n.recipient_snapshot WHERE n.notice_id=$1 AND n.claim_id=$2 AND n.status='sending' AND n.claim_until>clock_timestamp() AND n.deadline_at>clock_timestamp() AND b.enabled AND b.verified FOR SHARE OF b`, m.NotificationID, m.ClaimID).Scan(&live) != nil {
		return ErrDenied
	}
	return tx.Commit(ctx)
}
func (s *Store) Finish(ctx context.Context, m deliveryoutbox.Message, o deliveryoutbox.Outcome) error {
	status, code := o.Status, o.ErrorCode
	if status != "sent" && status != "delivery_unknown" && status != "absent" {
		status = "delivery_unknown"
		code = "unrecognized_outcome"
	}
	if status == "sent" && o.ACK == "" {
		status = "delivery_unknown"
		code = "missing_ack"
	}
	if status == "absent" {
		status = "failed"
		if o.Retryable && m.AttemptCount < 6 {
			status = "pending"
		}
	}
	delay := o.RetryAfter
	if delay < time.Second {
		delay = time.Duration(1<<min(m.AttemptCount, 6)) * time.Second
	}
	tag, err := s.pool.Exec(ctx, `UPDATE execution_notices SET status=$3,channel_ack=$4,error_code=$5,claim_id='',claim_until=NULL,next_attempt_at=clock_timestamp()+$6::interval WHERE notice_id=$1 AND claim_id=$2 AND status='sending' AND claim_until>clock_timestamp()`, m.NotificationID, m.ClaimID, status, o.ACK, code, delay.String())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return deliveryoutbox.ErrClaimLost
	}
	return nil
}
