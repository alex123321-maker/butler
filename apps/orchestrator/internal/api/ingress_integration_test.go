package api

import (
	"context"
	"encoding/json"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/ingress"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type detachedExecutor struct {
	started chan struct{}
	finish  chan struct{}
}

func (e detachedExecutor) Execute(ctx context.Context, _ ingress.InputEvent) (*orchestrator.ExecutionResult, error) {
	close(e.started)
	select {
	case <-e.finish:
		return &orchestrator.ExecutionResult{CurrentState: commonv1.RunState_RUN_STATE_COMPLETED}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func TestHTTPAcceptedInputSurvivesClientDisconnectIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 4, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.RunMigrations(ctx, filepath.Join("..", "..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	key := "integration:http:" + time.Now().UTC().Format("150405.000000000")
	defer db.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, key)
	store := durableinput.NewStore(db.Pool())
	executor := detachedExecutor{started: make(chan struct{}), finish: make(chan struct{})}
	service := durableinput.NewService(store, session.NewPostgresRepository(db.Pool()), executor, run.NewService(run.NewPostgresRepository(db.Pool()), nil), nil)
	server := httptest.NewServer(NewServer(service, nil).HTTPHandler())
	defer server.Close()
	clientCtx, disconnect := context.WithCancel(ctx)
	r, err := http.NewRequestWithContext(clientCtx, http.MethodPost, server.URL, strings.NewReader(`{"event_id":"delivery-1","event_type":"user_message","session_key":"`+key+`","source":"web","payload":{"text":"hello","user_id":"owner"}}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Accepted bool   `json:"accepted"`
		InputID  string `json:"input_id"`
	}
	err = json.NewDecoder(response.Body).Decode(&receipt)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusAccepted || !receipt.Accepted || receipt.InputID == "" {
		t.Fatalf("HTTP durable receipt: %#v %v", receipt, err)
	}
	disconnect()
	workerCtx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- service.Run(workerCtx) }()
	select {
	case <-executor.started:
	case <-ctx.Done():
		t.Fatal("disconnected receipt did not execute")
	}
	close(executor.finish)
	for {
		r, err := store.Get(ctx, receipt.InputID)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status == "completed" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("accepted work cancelled: %#v", r)
		case <-time.After(10 * time.Millisecond):
		}
	}
	stop()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
