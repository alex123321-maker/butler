package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type submitResponse struct {
	RunID      string `json:"run_id"`
	SessionKey string `json:"session_key"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	baseURL := valueOrDefault("BUTLER_SMOKE_BASE_URL", "http://localhost:8080")
	postgresURL := os.Getenv("BUTLER_SMOKE_POSTGRES_URL")
	if postgresURL == "" {
		postgresURL = os.Getenv("BUTLER_POSTGRES_URL")
	}
	if postgresURL == "" {
		fail("set BUTLER_SMOKE_POSTGRES_URL or BUTLER_POSTGRES_URL")
	}

	suffix := time.Now().UTC().Format("20060102T150405")
	sessionKey := "smoke:session:" + suffix
	eventID := "smoke-event-" + suffix
	body := map[string]any{
		"event_id":        eventID,
		"event_type":      "user_message",
		"session_key":     sessionKey,
		"source":          "smoke",
		"payload":         map[string]any{"text": "Reply with the word smoke."},
		"created_at":      time.Now().UTC().Format(time.RFC3339Nano),
		"idempotency_key": eventID,
	}

	result := postEvent(ctx, baseURL+"/api/v1/events", body)
	fmt.Printf("submitted run %s for session %s\n", result.RunID, result.SessionKey)

	pool, err := pgxpool.New(ctx, postgresURL)
	if err != nil {
		fail("connect postgres: %v", err)
	}
	defer pool.Close()

	state := waitForCompletedRun(ctx, pool, result.RunID)
	fmt.Printf("run reached terminal state: %s\n", state)

	userCount, assistantCount := verifyTranscript(ctx, pool, result.RunID)
	fmt.Printf("transcript verified: user=%d assistant=%d\n", userCount, assistantCount)
	fmt.Println("smoke verification passed")
}

func postEvent(ctx context.Context, url string, payload map[string]any) submitResponse {
	encoded, err := json.Marshal(payload)
	if err != nil {
		fail("marshal request: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		fail("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail("post event: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		fail("unexpected response status: %s", resp.Status)
	}
	var result submitResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fail("decode response: %v", err)
	}
	if strings.TrimSpace(result.RunID) == "" {
		fail("response missing run_id")
	}
	return result
}

func waitForCompletedRun(ctx context.Context, pool *pgxpool.Pool, runID string) string {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		var state string
		err := pool.QueryRow(ctx, `SELECT current_state FROM runs WHERE run_id = $1`, runID).Scan(&state)
		if err == nil {
			switch state {
			case "completed":
				return state
			case "failed", "cancelled", "timed_out":
				fail("run reached unexpected terminal state: %s", state)
			}
		}
		select {
		case <-ctx.Done():
			fail("timed out waiting for run completion")
		case <-ticker.C:
		}
	}
}

func verifyTranscript(ctx context.Context, pool *pgxpool.Pool, runID string) (int, int) {
	var userCount, assistantCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE run_id = $1 AND role = 'user'`, runID).Scan(&userCount); err != nil {
		fail("count user messages: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE run_id = $1 AND role = 'assistant'`, runID).Scan(&assistantCount); err != nil {
		fail("count assistant messages: %v", err)
	}
	if userCount < 1 || assistantCount < 1 {
		fail("expected at least one user and one assistant message, got user=%d assistant=%d", userCount, assistantCount)
	}
	return userCount, assistantCount
}

func valueOrDefault(value, fallback string) string {
	trimmed := strings.TrimSpace(os.Getenv(value))
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
