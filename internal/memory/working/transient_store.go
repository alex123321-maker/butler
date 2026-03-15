package working

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	redislib "github.com/redis/go-redis/v9"
)

var ErrTransientStateNotFound = errors.New("transient working state not found")

type TransientStore struct {
	client *redislib.Client
}

type TransientState struct {
	SessionKey  string `json:"session_key"`
	RunID       string `json:"run_id"`
	Status      string `json:"status"`
	ScratchJSON string `json:"scratch_json"`
	UpdatedAt   string `json:"updated_at"`
}

func NewTransientStore(client *redislib.Client) *TransientStore {
	return &TransientStore{client: client}
}

func (s *TransientStore) Save(ctx context.Context, state TransientState, ttl time.Duration) (TransientState, error) {
	if err := validateTransientState(state, ttl); err != nil {
		return TransientState{}, err
	}
	if strings.TrimSpace(state.UpdatedAt) == "" {
		state.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return TransientState{}, fmt.Errorf("marshal transient working state: %w", err)
	}
	if err := s.client.Set(ctx, transientKey(state.SessionKey, state.RunID), payload, ttl).Err(); err != nil {
		return TransientState{}, fmt.Errorf("save transient working state: %w", err)
	}
	return state, nil
}

func (s *TransientStore) Get(ctx context.Context, sessionKey, runID string) (TransientState, error) {
	payload, err := s.client.Get(ctx, transientKey(sessionKey, runID)).Bytes()
	if err != nil {
		if errors.Is(err, redislib.Nil) {
			return TransientState{}, ErrTransientStateNotFound
		}
		return TransientState{}, fmt.Errorf("get transient working state: %w", err)
	}
	var state TransientState
	if err := json.Unmarshal(payload, &state); err != nil {
		return TransientState{}, fmt.Errorf("unmarshal transient working state: %w", err)
	}
	return state, nil
}

func (s *TransientStore) Clear(ctx context.Context, sessionKey, runID string) error {
	deleted, err := s.client.Del(ctx, transientKey(sessionKey, runID)).Result()
	if err != nil {
		return fmt.Errorf("clear transient working state: %w", err)
	}
	if deleted == 0 {
		return ErrTransientStateNotFound
	}
	return nil
}

func (s *TransientStore) TTL(ctx context.Context, sessionKey, runID string) (time.Duration, error) {
	ttl, err := s.client.TTL(ctx, transientKey(sessionKey, runID)).Result()
	if err != nil {
		return 0, fmt.Errorf("transient working state ttl: %w", err)
	}
	if ttl < 0 {
		return 0, ErrTransientStateNotFound
	}
	return ttl, nil
}

func transientKey(sessionKey, runID string) string {
	return "butler:memory:working:transient:" + strings.TrimSpace(sessionKey) + ":" + strings.TrimSpace(runID)
}

func validateTransientState(state TransientState, ttl time.Duration) error {
	if strings.TrimSpace(state.SessionKey) == "" {
		return fmt.Errorf("session_key is required")
	}
	if strings.TrimSpace(state.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	if ttl <= 0 {
		return fmt.Errorf("ttl must be greater than zero")
	}
	if trimmed := strings.TrimSpace(state.ScratchJSON); trimmed != "" && !json.Valid([]byte(trimmed)) {
		return fmt.Errorf("scratch_json must be valid json")
	}
	return nil
}
