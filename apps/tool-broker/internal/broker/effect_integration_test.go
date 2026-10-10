package broker

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/jackc/pgx/v5/pgxpool"
)

type acceptedThenLostExecutor struct {
	site  *httptest.Server
	calls atomic.Int32
}

func (e *acceptedThenLostExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, _ *toolbrokerv1.ToolContract, _ []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	e.calls.Add(1)
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, e.site.URL, nil)
	response, err := e.site.Client().Do(request)
	if err != nil {
		return nil, err
	}
	response.Body.Close()
	return nil, errors.New("injected response loss after external acceptance")
}
func TestBrokerPostgresLostExternalAcknowledgementStopsDuplicateDispatch(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_ACTIONAUTH_POSTGRES_URL")
	if dsn == "" {
		t.Skip("isolated PostgreSQL required")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("separate test database required")
	}
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	schema := "broker_effect_" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	for _, migration := range []string{"031_action_authorization.up.sql", "035_external_effect_journal.up.sql"} {
		data, e := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", migration))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(data)); e != nil {
			t.Fatal(e)
		}
	}
	_, err = pool.Exec(ctx, `CREATE TABLE session_ownership(session_key TEXT PRIMARY KEY,run_id TEXT,lease_id TEXT,fence_epoch BIGINT,expires_at TIMESTAMPTZ);
 CREATE TABLE runs(run_id TEXT PRIMARY KEY,task_id TEXT,task_revision BIGINT);
 CREATE TABLE tasks(task_id TEXT PRIMARY KEY,owner_id TEXT,status TEXT,revision BIGINT,constraints_revision BIGINT,execution_policy TEXT NOT NULL DEFAULT 'normal');
 INSERT INTO runs VALUES('run','task',1);INSERT INTO tasks(task_id,owner_id,status,revision,constraints_revision) VALUES('task','owner','active',1,1);
 INSERT INTO session_ownership VALUES('web:owner','run','fixture-lease',1,clock_timestamp()+INTERVAL '1 hour')`)
	if err != nil {
		t.Fatal(err)
	}
	var effects atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { effects.Add(1); w.WriteHeader(201) }))
	defer site.Close()
	executor := &acceptedThenLostExecutor{site: site}
	server := NewServer(loadTestRegistry(t), executor, nil, nil)
	authority := newAuthFixture()
	store := actionauth.NewPostgresStore(pool)
	server.SetActionAuthorizer(actionauth.NewService(store, authority, nil))
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "call", ToolName: "http.request", AutonomyMode: 3, ArgsJson: `{"method":"POST","url":"` + site.URL + `"}`}
	prepared, err := server.PrepareToolCall(ctx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call, LogicalActionRef: "trusted-task-slot"})
	if err != nil {
		t.Fatal(err)
	}
	intent, _ := store.GetIntent(ctx, prepared.GetAction().GetActionId())
	authority.approvals["approval"] = actionauth.ApprovalSnapshot{ApprovalID: "approval", Status: "approved", Binding: intent.Binding, ResolvedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	grant, err := server.AuthorizeToolCall(ctx, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: intent.Binding.ActionID, ApprovalId: "approval"})
	if err != nil {
		t.Fatal(err)
	}
	request := &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: grant.GetAuthorizationId()}
	result, err := server.ExecuteToolCall(ctx, request)
	if err != nil || result.GetResult().GetStatus() != "effect_unknown" || result.GetResult().GetActionId() != prepared.GetAction().GetEffectId() || len(result.GetResult().GetEvidenceRefs()) != 1 {
		t.Fatal("uncertain effect or journal metadata lost")
	}
	restarted := NewServer(loadTestRegistry(t), executor, nil, nil)
	restarted.SetActionAuthorizer(actionauth.NewService(actionauth.NewPostgresStore(pool), authority, nil))
	duplicate, err := restarted.ExecuteToolCall(ctx, request)
	if err != nil || duplicate.GetResult().GetStatus() != "effect_unknown" || duplicate.GetResult().GetError().GetRetryable() || effects.Load() != 1 || executor.calls.Load() != 1 {
		t.Fatal("repeated RPC dispatched an uncertain external action")
	}
}
