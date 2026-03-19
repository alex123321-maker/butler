package deliveryevents

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ChannelTelegram = "telegram"
	ChannelWeb      = "web"

	StateSent    = "sent"
	StateWaiting = "waiting_reply"
	StateFailed  = "failed"

	TypeAssistantDelta  = "assistant_delta"
	TypeAssistantFinal  = "assistant_final"
	TypeApprovalRequest = "approval_request"
	TypeToolCallEvent   = "tool_call_event"
	TypeStatus          = "status"
)

type Record struct {
	EventID      int64
	RunID        string
	SessionKey   string
	Channel      string
	DeliveryType string
	State        string
	ErrorMessage string
	DetailsJSON  string
	CreatedAt    time.Time
}

type CreateParams struct {
	RunID        string
	SessionKey   string
	Channel      string
	DeliveryType string
	State        string
	ErrorMessage string
	DetailsJSON  string
	CreatedAt    time.Time
}

type ListParams struct {
	RunID      string
	SessionKey string
	Channel    string
	State      string
	Limit      int
	Offset     int
}

type Repository interface {
	CreateEvent(ctx context.Context, params CreateParams) (Record, error)
	ListEvents(ctx context.Context, params ListParams) ([]Record, error)
	LatestByRun(ctx context.Context, runID string) (*Record, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) CreateEvent(ctx context.Context, params CreateParams) (Record, error) {
	if params.CreatedAt.IsZero() {
		params.CreatedAt = time.Now().UTC()
	}
	if params.DetailsJSON == "" {
		params.DetailsJSON = "{}"
	}
	const query = `
		INSERT INTO channel_delivery_events (
			run_id, session_key, channel, delivery_type, state, error_message, details_json, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8)
		RETURNING event_id, run_id, session_key, channel, delivery_type, state, error_message, details_json::text, created_at
	`
	rec := Record{}
	err := r.pool.QueryRow(ctx, query,
		params.RunID,
		params.SessionKey,
		params.Channel,
		params.DeliveryType,
		params.State,
		params.ErrorMessage,
		params.DetailsJSON,
		params.CreatedAt,
	).Scan(&rec.EventID, &rec.RunID, &rec.SessionKey, &rec.Channel, &rec.DeliveryType, &rec.State, &rec.ErrorMessage, &rec.DetailsJSON, &rec.CreatedAt)
	if err != nil {
		return Record{}, fmt.Errorf("create delivery event: %w", err)
	}
	return rec, nil
}

func (r *PostgresRepository) ListEvents(ctx context.Context, params ListParams) ([]Record, error) {
	if params.Limit <= 0 {
		params.Limit = 50
	}
	if params.Limit > 200 {
		params.Limit = 200
	}
	if params.Offset < 0 {
		params.Offset = 0
	}
	const query = `
		SELECT event_id, run_id, session_key, channel, delivery_type, state, error_message, details_json::text, created_at
		FROM channel_delivery_events
		WHERE ($1 = '' OR run_id = $1)
		  AND ($2 = '' OR session_key = $2)
		  AND ($3 = '' OR channel = $3)
		  AND ($4 = '' OR state = $4)
		ORDER BY created_at DESC, event_id DESC
		LIMIT $5 OFFSET $6
	`
	rows, err := r.pool.Query(ctx, query, params.RunID, params.SessionKey, params.Channel, params.State, params.Limit, params.Offset)
	if err != nil {
		return nil, fmt.Errorf("list delivery events: %w", err)
	}
	defer rows.Close()

	items := make([]Record, 0)
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.EventID, &rec.RunID, &rec.SessionKey, &rec.Channel, &rec.DeliveryType, &rec.State, &rec.ErrorMessage, &rec.DetailsJSON, &rec.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan delivery event row: %w", err)
		}
		items = append(items, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate delivery events: %w", err)
	}
	return items, nil
}

func (r *PostgresRepository) LatestByRun(ctx context.Context, runID string) (*Record, error) {
	const query = `
		SELECT event_id, run_id, session_key, channel, delivery_type, state, error_message, details_json::text, created_at
		FROM channel_delivery_events
		WHERE run_id = $1
		ORDER BY created_at DESC, event_id DESC
		LIMIT 1
	`
	var rec Record
	err := r.pool.QueryRow(ctx, query, runID).Scan(&rec.EventID, &rec.RunID, &rec.SessionKey, &rec.Channel, &rec.DeliveryType, &rec.State, &rec.ErrorMessage, &rec.DetailsJSON, &rec.CreatedAt)
	if err != nil {
		return nil, nil
	}
	return &rec, nil
}
