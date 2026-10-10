// Package input owns durable ingress receipts and the session-ordered work queue.
package input

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("input receipt not found")
var ErrClaimLost = errors.New("input claim expired or superseded")

type Record struct {
	InputID        string             `json:"input_id"`
	SessionKey     string             `json:"session_key"`
	Sequence       int64              `json:"session_sequence"`
	OwnerID        string             `json:"owner_id"`
	Source         string             `json:"source"`
	TaskID         string             `json:"task_id,omitempty"`
	RunID          string             `json:"run_id,omitempty"`
	Status         string             `json:"status"`
	Checkpoint     string             `json:"checkpoint"`
	CheckpointJSON string             `json:"-"`
	Event          ingress.InputEvent `json:"-"`
	Attempts       int                `json:"attempts"`
	MaxAttempts    int                `json:"max_attempts"`
	ClaimID        string             `json:"-"`
	ClaimExpiresAt *time.Time         `json:"-"`
	ExpiresAt      time.Time          `json:"expires_at"`
	AcceptedAt     time.Time          `json:"accepted_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	ErrorCode      string             `json:"error_code,omitempty"`
	ErrorMessage   string             `json:"error_message,omitempty"`
}

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const columns = `input_id,session_key,session_sequence,owner_id,source,COALESCE(task_id,''),COALESCE(run_id,''),status,checkpoint,checkpoint_json::text,event_json,attempts,max_attempts,claim_id,claim_expires_at,expires_at,accepted_at,updated_at,error_code,error_message`

func scan(row interface{ Scan(...any) error }) (Record, error) {
	var r Record
	var event []byte
	err := row.Scan(&r.InputID, &r.SessionKey, &r.Sequence, &r.OwnerID, &r.Source, &r.TaskID, &r.RunID, &r.Status, &r.Checkpoint, &r.CheckpointJSON, &event, &r.Attempts, &r.MaxAttempts, &r.ClaimID, &r.ClaimExpiresAt, &r.ExpiresAt, &r.AcceptedAt, &r.UpdatedAt, &r.ErrorCode, &r.ErrorMessage)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(event, &r.Event)
	return r, err
}
func randomID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw[:]), nil
}

func (s *Store) Accept(ctx context.Context, event ingress.InputEvent, ownerID, taskID string) (Record, bool, error) {
	return s.accept(ctx, event, ownerID, taskID, "queued", "")
}
func (s *Store) AcceptWaiting(ctx context.Context, event ingress.InputEvent, ownerID string) (Record, bool, error) {
	return s.accept(ctx, event, ownerID, "", "waiting_for_user", "task_target_required")
}
func (s *Store) accept(ctx context.Context, event ingress.InputEvent, ownerID, taskID, status, code string) (Record, bool, error) {
	// Serialize admission before assigning sequence numbers. Otherwise an
	// uncommitted earlier insert can become visible after a later head was claimed.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, false, err
	}
	defer tx.Rollback(context.Background())
	var storedOwner string
	if err = tx.QueryRow(ctx, `SELECT user_id FROM sessions WHERE session_key=$1 FOR UPDATE`, event.SessionKey).Scan(&storedOwner); err != nil {
		return Record{}, false, err
	}
	if storedOwner != ownerID {
		return Record{}, false, fmt.Errorf("session owner mismatch")
	}
	id, err := randomID("input-")
	if err != nil {
		return Record{}, false, err
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		return Record{}, false, err
	}
	key := event.IdempotencyKey
	if key == "" {
		key = event.EventID
	}
	if key == "" {
		return Record{}, false, fmt.Errorf("event_id or idempotency_key required")
	}
	// Same delivery identity with a changed payload is a conflict, never a second job.
	r, err := scan(tx.QueryRow(ctx, `INSERT INTO accepted_inputs(input_id,session_key,event_id,dedup_key,source,owner_id,event_json,task_id,status,error_code)
 SELECT $1,$2,$3,$4,$5,$6,$7::jsonb,NULLIF($8,''),$9,$10
 WHERE $8='' OR EXISTS(SELECT 1 FROM tasks WHERE task_id=$8 AND session_key=$2 AND owner_id=$6 AND status='active')
 ON CONFLICT(session_key,source,dedup_key) DO NOTHING RETURNING `+columns, id, event.SessionKey, event.EventID, key, event.Source, ownerID, encoded, taskID, status, code))
	if err == nil {
		return r, true, tx.Commit(ctx)
	}
	if !errors.Is(err, ErrNotFound) {
		return Record{}, false, err
	}
	r, err = scan(tx.QueryRow(ctx, `SELECT `+columns+` FROM accepted_inputs WHERE session_key=$1 AND source=$2 AND dedup_key=$3`, event.SessionKey, event.Source, key))
	if err != nil {
		return Record{}, false, fmt.Errorf("input task or identity invalid: %w", err)
	}
	if r.OwnerID != ownerID || r.TaskID != taskID || r.Event.PayloadJSON != event.PayloadJSON || r.Event.EventType != event.EventType {
		return Record{}, false, fmt.Errorf("delivery identity conflicts with accepted payload")
	}
	return r, false, tx.Commit(ctx)
}
func (s *Store) Get(ctx context.Context, id string) (Record, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM accepted_inputs WHERE input_id=$1`, id))
}
func (s *Store) ExpireQueued(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `UPDATE accepted_inputs SET status='expired',error_code='input_expired',error_message='accepted input deadline expired',updated_at=clock_timestamp() WHERE status='queued' AND expires_at<=clock_timestamp()`)
	return err
}

func (s *Store) Claim(ctx context.Context, ttl time.Duration) (Record, error) {
	id, err := randomID("claim-")
	if err != nil {
		return Record{}, err
	}
	// A waiting/blocked head intentionally blocks later work in that session.
	// Across sessions, the oldest ready head wins, preventing a busy chat from
	// monopolizing the worker. SKIP LOCKED permits independent session workers.
	return scan(s.pool.QueryRow(ctx, `WITH candidate AS (
 SELECT i.input_id FROM accepted_inputs i
 WHERE i.status IN ('queued','running') AND i.not_before<=clock_timestamp()
 AND (i.claim_expires_at IS NULL OR i.claim_expires_at<=clock_timestamp())
 AND NOT EXISTS(SELECT 1 FROM accepted_inputs h WHERE h.session_key=i.session_key AND h.session_sequence<i.session_sequence AND h.status NOT IN ('completed','failed','expired','cancelled'))
 ORDER BY i.accepted_at,i.session_sequence FOR UPDATE OF i SKIP LOCKED LIMIT 1
 ) UPDATE accepted_inputs i SET status='running',claim_id=$1,claim_expires_at=clock_timestamp()+$2*INTERVAL '1 millisecond',attempts=attempts+CASE WHEN checkpoint='approval_ready' AND status='queued' THEN 0 ELSE 1 END,updated_at=clock_timestamp()
 FROM candidate WHERE i.input_id=candidate.input_id RETURNING `+qualifiedColumns("i"), id, ttl.Milliseconds()))
}
func qualifiedColumns(alias string) string {
	// Keep the RETURNING shape explicit: expressions cannot be prefix-concatenated.
	return alias + `.input_id,` + alias + `.session_key,` + alias + `.session_sequence,` + alias + `.owner_id,` + alias + `.source,COALESCE(` + alias + `.task_id,''),COALESCE(` + alias + `.run_id,''),` + alias + `.status,` + alias + `.checkpoint,` + alias + `.checkpoint_json::text,` + alias + `.event_json,` + alias + `.attempts,` + alias + `.max_attempts,` + alias + `.claim_id,` + alias + `.claim_expires_at,` + alias + `.expires_at,` + alias + `.accepted_at,` + alias + `.updated_at,` + alias + `.error_code,` + alias + `.error_message`
}
func (s *Store) Renew(ctx context.Context, r Record, ttl time.Duration) error {
	tag, err := s.pool.Exec(ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()+$3*INTERVAL '1 millisecond' WHERE input_id=$1 AND claim_id=$2 AND status='running' AND claim_expires_at>clock_timestamp()`, r.InputID, r.ClaimID, ttl.Milliseconds())
	if err == nil && tag.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return err
}

type claimKey struct{}

func WithClaim(ctx context.Context, r Record) context.Context {
	return context.WithValue(ctx, claimKey{}, r)
}
func (s *Store) SaveCheckpoint(ctx context.Context, runID, stage, payload string) error {
	r, ok := ctx.Value(claimKey{}).(Record)
	if !ok {
		return nil
	}
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return err
	}
	defer release()
	tag, err := s.pool.Exec(ctx, `UPDATE accepted_inputs SET run_id=$3,checkpoint=$4,checkpoint_json=CASE WHEN $4='prepared' THEN jsonb_build_object('prepared',$5::jsonb) ELSE checkpoint_json||jsonb_build_object('checkpoint_data',$5::jsonb) END,updated_at=clock_timestamp() WHERE input_id=$1 AND claim_id=$2 AND status='running' AND claim_expires_at>clock_timestamp()`, r.InputID, r.ClaimID, runID, stage, payload)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return err
}
func (s *Store) LoadCheckpoint(ctx context.Context, runID string) (string, string, error) {
	var stage, payload string
	err := s.pool.QueryRow(ctx, `SELECT checkpoint,checkpoint_json::text FROM accepted_inputs WHERE run_id=$1`, runID).Scan(&stage, &payload)
	return stage, payload, err
}
func (s *Store) Finish(ctx context.Context, r Record, status, code, message string, delay time.Duration) error {
	tag, err := s.pool.Exec(ctx, `UPDATE accepted_inputs SET status=CASE WHEN $3='waiting_for_user' AND checkpoint='approval_ready' THEN 'queued' ELSE $3 END,attempts=CASE WHEN $4='session_busy' THEN GREATEST(attempts-1,0) ELSE attempts END,error_code=$4,error_message=$5,claim_id='',claim_expires_at=NULL,not_before=clock_timestamp()+$6*INTERVAL '1 millisecond',updated_at=clock_timestamp() WHERE input_id=$1 AND claim_id=$2 AND status='running' AND claim_expires_at>clock_timestamp()`, r.InputID, r.ClaimID, status, code, message, delay.Milliseconds())
	if err == nil && tag.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return err
}

// These hooks participate in the approval Service transaction. They keep input
// ownership/order changes inside the input subsystem rather than ad hoc callers.
func (s *Store) SuspendApprovalTx(ctx context.Context, tx pgx.Tx, runID, approvalID, payload string) error {
	r, ok := ctx.Value(claimKey{}).(Record)
	if !ok {
		return fmt.Errorf("durable input claim required for approval suspension")
	}
	tag, err := tx.Exec(ctx, `UPDATE accepted_inputs SET checkpoint='awaiting_approval',checkpoint_json=checkpoint_json||jsonb_build_object('approval_id',$3::text,'continuation',$4::jsonb),updated_at=clock_timestamp() WHERE input_id=$1 AND claim_id=$2 AND run_id=$5 AND status='running' AND claim_expires_at>clock_timestamp()`, r.InputID, r.ClaimID, approvalID, payload, runID)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return err
}
func (s *Store) ReadyApprovalTx(ctx context.Context, tx pgx.Tx, runID, approvalID string) error {
	tag, err := tx.Exec(ctx, `UPDATE accepted_inputs SET checkpoint='approval_ready',status=CASE WHEN status='running' THEN status ELSE 'queued' END,not_before=clock_timestamp(),updated_at=clock_timestamp() WHERE run_id=$1 AND checkpoint_json->>'approval_id'=$2 AND status IN ('running','waiting_for_user','blocked')`, runID, approvalID)
	if err == nil && tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return err
}
