// Package deliveryoutbox retries stored notifications, never work or tool calls.
package deliveryoutbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"log/slog"
	"time"
	"unicode/utf8"
)

type Message struct {
	NotificationID string `json:"notification_id"`
	ResultID       string `json:"result_id"`
	RunID          string `json:"run_id"`
	SessionKey     string `json:"session_key"`
	Channel        string `json:"channel"`
	ChunkIndex     int    `json:"chunk_index"`
	Content        string `json:"content"`
	Status         string `json:"status"`
	AttemptCount   int    `json:"attempt_count"`
	ClaimID        string `json:"-"`
	ChannelACK     string `json:"channel_ack,omitempty"`
	ErrorCode      string `json:"error_code,omitempty"`
}

// Absent is allowed only when the channel explicitly confirms no message was accepted.
type Outcome struct {
	Status     string
	ACK        string
	ErrorCode  string
	Retryable  bool
	RetryAfter time.Duration
}
type Sender interface {
	SendStored(context.Context, Message) Outcome
}
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const columns = `notification_id,result_id,run_id,session_key,channel,chunk_index,content,status,attempt_count,claim_id,channel_ack,error_code`

func scan(row interface{ Scan(...any) error }) (Message, error) {
	var m Message
	err := row.Scan(&m.NotificationID, &m.ResultID, &m.RunID, &m.SessionKey, &m.Channel, &m.ChunkIndex, &m.Content, &m.Status, &m.AttemptCount, &m.ClaimID, &m.ChannelACK, &m.ErrorCode)
	return m, err
}

// Split uses Telegram's UTF-16 length rather than bytes, preserving every rune.
func Split(content string, limit int) []string {
	if limit < 2 {
		limit = 4096
	}
	parts := []string{}
	start, units := 0, 0
	for pos, r := range content {
		size := 1
		if r > 0xffff {
			size = 2
		}
		if units+size > limit {
			parts = append(parts, content[start:pos])
			start = pos
			units = 0
		}
		units += size
	}
	if start < len(content) {
		parts = append(parts, content[start:])
	}
	return parts
}
func (s *Store) EnqueueTx(ctx context.Context, tx pgx.Tx, resultID, runID, key, channel, content string, now time.Time) error {
	if !utf8.ValidString(content) {
		return fmt.Errorf("notification must be valid UTF-8")
	}
	chunks := []string{content}
	state := "failed"
	code := "unsupported_channel"
	if channel == "telegram" {
		chunks = Split(content, 4096)
		state = "pending"
		code = ""
	}
	if channel == "web" {
		state = "result_ready"
		code = ""
	}
	for i, chunk := range chunks {
		_, err := tx.Exec(ctx, `INSERT INTO delivery_outbox(notification_id,result_id,run_id,session_key,channel,chunk_index,content,status,error_code,deadline_at,created_at,updated_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$11)`, fmt.Sprintf("%s:%s:%d", resultID, channel, i), resultID, runID, key, channel, i, chunk, state, code, now.Add(24*time.Hour), now)
		if err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) ListByRun(ctx context.Context, id string) ([]Message, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM delivery_outbox WHERE run_id=$1 ORDER BY channel,chunk_index`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Message{}
	for rows.Next() {
		m, e := scan(rows)
		if e != nil {
			return nil, e
		}
		result = append(result, m)
	}
	return result, rows.Err()
}
func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto randomness unavailable")
	}
	return hex.EncodeToString(b[:])
}

var ErrClaimLost = errors.New("outbox claim lost")

// Claim first quarantines expired sending claims: Telegram has no idempotency key.
func (s *Store) Claim(ctx context.Context, ttl time.Duration) (*Message, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `UPDATE delivery_outbox SET status='delivery_unknown',error_code='sending_claim_expired',claim_id='',claim_until=NULL,updated_at=NOW() WHERE status='sending' AND claim_until<=clock_timestamp()`); err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE delivery_outbox SET status='failed',error_code='delivery_deadline_exceeded',updated_at=NOW() WHERE status='pending' AND (deadline_at<=clock_timestamp() OR attempt_count>=6)`); err != nil {
		return nil, err
	}
	id := randomID()
	m, err := scan(tx.QueryRow(ctx, `UPDATE delivery_outbox SET status='sending',claim_id=$1,claim_until=clock_timestamp()+$2::interval,attempt_count=attempt_count+1,updated_at=NOW() WHERE notification_id=(SELECT d.notification_id FROM delivery_outbox d WHERE d.status='pending' AND d.next_attempt_at<=clock_timestamp() AND d.deadline_at>clock_timestamp() AND d.attempt_count<6 AND NOT EXISTS(SELECT 1 FROM delivery_outbox p WHERE p.result_id=d.result_id AND p.channel=d.channel AND p.chunk_index<d.chunk_index AND p.status<>'sent') ORDER BY d.created_at,d.chunk_index FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING `+columns, id, ttl.String()))
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
func (s *Store) Finish(ctx context.Context, m Message, o Outcome) error {
	status := o.Status
	if status != "sent" && status != "absent" {
		status = "delivery_unknown"
	}
	if status == "sent" && o.ACK == "" {
		status = "delivery_unknown"
		o.ErrorCode = "missing_channel_ack"
	}
	delay := o.RetryAfter
	if delay < time.Second {
		delay = time.Duration(1<<min(m.AttemptCount, 8)) * time.Second
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	if status == "absent" {
		status = "failed"
		if o.Retryable && m.AttemptCount < 6 {
			status = "pending"
		}
	}
	tag, err := s.pool.Exec(ctx, `UPDATE delivery_outbox SET status=$3,channel_ack=$4,error_code=$5,next_attempt_at=clock_timestamp()+$6::interval,claim_id='',claim_until=NULL,updated_at=NOW() WHERE notification_id=$1 AND claim_id=$2 AND status='sending' AND claim_until>clock_timestamp()`, m.NotificationID, m.ClaimID, status, o.ACK, safeCode(o.ErrorCode), delay.String())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrClaimLost
	}
	return nil
}
func safeCode(code string) string {
	switch code {
	case "", "telegram_rejected", "telegram_unconfigured", "telegram_chat_not_allowed", "transport_outcome_unknown", "response_outcome_unknown", "missing_channel_ack":
		return code
	}
	return "channel_error"
}

type Worker struct {
	Store        *Store
	Sender       Sender
	PollInterval time.Duration
}

// AuthorizeSend checks the current stored claim/payload immediately before the
// channel boundary. A previously copied payload cannot send after redaction.
func (s *Store) AuthorizeSend(ctx context.Context, m Message) error {
	var live bool
	e := s.pool.QueryRow(ctx, `SELECT TRUE FROM delivery_outbox d WHERE notification_id=$1 AND claim_id=$2 AND status='sending' AND claim_until>clock_timestamp() AND content=$3 AND NOT COALESCE((to_jsonb(d)->>'privacy_redacted')::BOOLEAN,FALSE)`, m.NotificationID, m.ClaimID, m.Content).Scan(&live)
	if e != nil {
		return ErrClaimLost
	}
	return nil
}

func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	m, err := w.Store.Claim(ctx, 30*time.Second)
	if err != nil || m == nil {
		return false, err
	}
	outcome := Outcome{Status: "absent", ErrorCode: "telegram_unconfigured", Retryable: true}
	if w.Sender != nil {
		if err = w.Store.AuthorizeSend(ctx, *m); err != nil {
			return true, err
		}
		sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		outcome = w.Sender.SendStored(sendCtx, *m)
		cancel()
	}
	// A failed ACK write leaves a sending claim which recovery marks unknown.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return true, w.Store.Finish(finishCtx, *m, outcome)
}
func (w *Worker) Run(ctx context.Context) error {
	delay := w.PollInterval
	if delay <= 0 {
		delay = time.Second
	}
	ticker := time.NewTicker(delay)
	defer ticker.Stop()
	for {
		if _, err := w.ProcessOne(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("outbox storage operation failed", slog.String("component", "delivery-outbox"))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
