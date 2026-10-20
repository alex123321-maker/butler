package transcript

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

type Message struct {
	MessageID      string
	Sequence       int64
	SourceRevision int64
	SessionKey     string
	RunID          string
	Role           string
	Content        string
	ToolCallID     string
	MetadataJSON   string
	CreatedAt      time.Time
}

type ToolCall struct {
	ToolCallID    string
	RunID         string
	ToolName      string
	ArgsJSON      string
	Status        string
	RuntimeTarget string
	StartedAt     time.Time
	FinishedAt    *time.Time
	ResultJSON    string
	ErrorJSON     string
}

type Transcript struct {
	Messages       []Message
	ToolCalls      []ToolCall
	SourceRevision int64
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) AppendMessage(ctx context.Context, message Message) (Message, error) {
	if token, ok := ownership.TokenFromContext(ctx); ok && (token.SessionKey != message.SessionKey || token.RunID != message.RunID) {
		return Message{}, ownership.ErrStale
	}
	ctx, release, fenceErr := ownership.Begin(ctx)
	if fenceErr != nil {
		return Message{}, fenceErr
	}
	defer release()

	if message.MessageID == "" {
		message.MessageID = newID("msg")
	}
	if message.CreatedAt.IsZero() {
		message.CreatedAt = time.Now().UTC()
	}
	if message.MetadataJSON == "" {
		message.MetadataJSON = "{}"
	}

	const query = `
		INSERT INTO messages (message_id, session_key, run_id, role, content, tool_call_id, metadata, created_at)
		VALUES ($1, $2, NULLIF($3, ''), $4, $5, NULLIF($6, ''), $7::jsonb, $8)
		RETURNING message_id, session_key, COALESCE(run_id, ''), role, content, COALESCE(tool_call_id, ''), metadata::text, created_at, message_sequence
	`

	stored := Message{}
	err := s.pool.QueryRow(ctx, query,
		message.MessageID,
		message.SessionKey,
		message.RunID,
		message.Role,
		message.Content,
		message.ToolCallID,
		message.MetadataJSON,
		message.CreatedAt,
	).Scan(
		&stored.MessageID,
		&stored.SessionKey,
		&stored.RunID,
		&stored.Role,
		&stored.Content,
		&stored.ToolCallID,
		&stored.MetadataJSON,
		&stored.CreatedAt,
		&stored.Sequence,
	)
	if err != nil {
		return Message{}, fmt.Errorf("append message: %w", err)
	}
	return stored, nil
}

func (s *Store) AppendToolCall(ctx context.Context, toolCall ToolCall) (ToolCall, error) {
	if token, ok := ownership.TokenFromContext(ctx); ok && token.RunID != toolCall.RunID {
		return ToolCall{}, ownership.ErrStale
	}
	ctx, release, fenceErr := ownership.Begin(ctx)
	if fenceErr != nil {
		return ToolCall{}, fenceErr
	}
	defer release()

	if toolCall.ToolCallID == "" {
		toolCall.ToolCallID = newID("tool")
	}
	if toolCall.StartedAt.IsZero() {
		toolCall.StartedAt = time.Now().UTC()
	}
	if toolCall.ArgsJSON == "" {
		toolCall.ArgsJSON = "{}"
	}
	if toolCall.ResultJSON == "" {
		toolCall.ResultJSON = "{}"
	}
	if toolCall.ErrorJSON == "" {
		toolCall.ErrorJSON = "{}"
	}

	const query = `
		INSERT INTO tool_calls (tool_call_id, run_id, tool_name, args, status, runtime_target, started_at, finished_at, result, error)
		VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, $9::jsonb, $10::jsonb)
		RETURNING tool_call_id, run_id, tool_name, args::text, status, runtime_target, started_at, finished_at, COALESCE(result::text, '{}'), COALESCE(error::text, '{}')
	`

	stored := ToolCall{}
	err := s.pool.QueryRow(ctx, query,
		toolCall.ToolCallID,
		toolCall.RunID,
		toolCall.ToolName,
		toolCall.ArgsJSON,
		toolCall.Status,
		toolCall.RuntimeTarget,
		toolCall.StartedAt,
		toolCall.FinishedAt,
		toolCall.ResultJSON,
		toolCall.ErrorJSON,
	).Scan(
		&stored.ToolCallID,
		&stored.RunID,
		&stored.ToolName,
		&stored.ArgsJSON,
		&stored.Status,
		&stored.RuntimeTarget,
		&stored.StartedAt,
		&stored.FinishedAt,
		&stored.ResultJSON,
		&stored.ErrorJSON,
	)
	if err != nil {
		return ToolCall{}, fmt.Errorf("append tool call: %w", err)
	}
	return stored, nil
}

func (s *Store) GetTranscript(ctx context.Context, sessionKey string) (Transcript, error) {
	messages, err := s.loadMessages(ctx, `
		SELECT m.message_id, m.session_key, COALESCE(m.run_id, ''), m.role, m.content, COALESCE(m.tool_call_id, ''), m.metadata::text, m.created_at, m.message_sequence, h.source_revision
		FROM messages m JOIN transcript_heads h ON h.session_key = m.session_key
		WHERE m.session_key = $1
		ORDER BY m.message_sequence ASC
	`, sessionKey)
	if err != nil {
		return Transcript{}, err
	}
	toolCalls, err := s.loadToolCalls(ctx, `
		SELECT tc.tool_call_id, tc.run_id, tc.tool_name, tc.args::text, tc.status, tc.runtime_target, tc.started_at, tc.finished_at, COALESCE(tc.result::text, '{}'), COALESCE(tc.error::text, '{}')
		FROM tool_calls tc
		JOIN runs r ON r.run_id = tc.run_id
		WHERE r.session_key = $1
		ORDER BY tc.started_at ASC
	`, sessionKey)
	if err != nil {
		return Transcript{}, err
	}
	return transcriptSnapshot(messages, toolCalls), nil
}

// GetTaskTranscript selects only this task's primary messages, retaining the
// session revision for ordinary privacy fencing. It does not move session data.
func (s *Store) GetTaskTranscript(ctx context.Context, sessionKey, taskID string) (Transcript, error) {
	messages, err := s.loadMessages(ctx, `
	 SELECT m.message_id,m.session_key,COALESCE(m.run_id,''),m.role,m.content,COALESCE(m.tool_call_id,''),m.metadata::text,m.created_at,m.message_sequence,h.source_revision
	 FROM messages m JOIN transcript_heads h USING(session_key) JOIN runs r ON r.run_id=m.run_id
	 WHERE m.session_key=$1 AND r.task_id=$2 ORDER BY m.message_sequence`, sessionKey, taskID)
	if err != nil {
		return Transcript{}, err
	}
	var revision int64
	if err = s.pool.QueryRow(ctx, `SELECT COALESCE((SELECT source_revision FROM transcript_heads WHERE session_key=$1),0)`, sessionKey).Scan(&revision); err != nil {
		return Transcript{}, err
	}
	return Transcript{Messages: messages, SourceRevision: revision}, nil
}

func (s *Store) GetRunTranscript(ctx context.Context, runID string) (Transcript, error) {
	messages, err := s.loadMessages(ctx, `
		SELECT m.message_id, m.session_key, COALESCE(m.run_id, ''), m.role, m.content, COALESCE(m.tool_call_id, ''), m.metadata::text, m.created_at, m.message_sequence, h.source_revision
		FROM messages m JOIN transcript_heads h ON h.session_key = m.session_key
		WHERE m.run_id = $1
		ORDER BY m.message_sequence ASC
	`, runID)
	if err != nil {
		return Transcript{}, err
	}
	toolCalls, err := s.loadToolCalls(ctx, `
		SELECT tool_call_id, run_id, tool_name, args::text, status, runtime_target, started_at, finished_at, COALESCE(result::text, '{}'), COALESCE(error::text, '{}')
		FROM tool_calls
		WHERE run_id = $1
		ORDER BY started_at ASC
	`, runID)
	if err != nil {
		return Transcript{}, err
	}
	return transcriptSnapshot(messages, toolCalls), nil
}

func transcriptSnapshot(messages []Message, toolCalls []ToolCall) Transcript {
	result := Transcript{Messages: messages, ToolCalls: toolCalls}
	if len(messages) > 0 {
		result.SourceRevision = messages[0].SourceRevision
	}
	return result
}

func (s *Store) loadMessages(ctx context.Context, query string, args ...any) ([]Message, error) {
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query transcript messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var message Message
		if err := rows.Scan(&message.MessageID, &message.SessionKey, &message.RunID, &message.Role, &message.Content, &message.ToolCallID, &message.MetadataJSON, &message.CreatedAt, &message.Sequence, &message.SourceRevision); err != nil {
			return nil, fmt.Errorf("scan transcript message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transcript messages: %w", err)
	}
	return messages, nil
}

func (s *Store) loadToolCalls(ctx context.Context, query, arg string) ([]ToolCall, error) {
	rows, err := s.pool.Query(ctx, query, arg)
	if err != nil {
		return nil, fmt.Errorf("query transcript tool calls: %w", err)
	}
	defer rows.Close()

	var toolCalls []ToolCall
	for rows.Next() {
		var toolCall ToolCall
		if err := rows.Scan(&toolCall.ToolCallID, &toolCall.RunID, &toolCall.ToolName, &toolCall.ArgsJSON, &toolCall.Status, &toolCall.RuntimeTarget, &toolCall.StartedAt, &toolCall.FinishedAt, &toolCall.ResultJSON, &toolCall.ErrorJSON); err != nil {
			return nil, fmt.Errorf("scan transcript tool call: %w", err)
		}
		toolCalls = append(toolCalls, toolCall)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transcript tool calls: %w", err)
	}
	return toolCalls, nil
}

func newID(prefix string) string {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("%s-%d-%s", prefix, time.Now().UTC().UnixNano(), hex.EncodeToString(raw[:]))
}
