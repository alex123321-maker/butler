package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/transcript"
	pg "github.com/butler/butler/internal/storage/postgres"
	"github.com/butler/butler/internal/transport"
)

// Only the model is scripted. The CLI, authenticated HTTP ingress, worker,
// leases, prompt construction, inference admission/usage, final/outbox and
// evidence reader all use the ordinary product implementations.
type benchmarkFixtureModel struct{ answer string }

func (benchmarkFixtureModel) Name() string                    { return "benchmark-fixture" }
func (benchmarkFixtureModel) InferenceLimits() (int64, int64) { return 8192, 256 }
func (benchmarkFixtureModel) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsUsageMetadata: true}, nil
}
func (m benchmarkFixtureModel) StartRun(_ context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if len(r.InputItems) == 0 {
		return nil, errors.New("missing actual assembled prompt")
	}
	ch := make(chan transport.TransportEvent, 2)
	ch <- transport.NewAssistantFinalEvent(r.Context.RunID, m.Name(), transport.AssistantFinal{Content: m.answer, FinishReason: "completed", Usage: &transport.Usage{InputTokens: 19, OutputTokens: 3, TotalTokens: 22}})
	ch <- transport.NewTerminalEvent(r.Context.RunID, transport.EventTypeRunCompleted, m.Name())
	close(ch)
	return ch, nil
}
func (benchmarkFixtureModel) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errors.New("unexpected continuation")
}
func (benchmarkFixtureModel) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return nil, errors.New("unexpected tool result")
}
func (benchmarkFixtureModel) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}

func TestBenchmarkPublicIngressThroughOfflineComparison(t *testing.T) {
	dsn, bin := os.Getenv("BUTLER_BENCH_PRODUCT_DB"), os.Getenv("BUTLER_BENCH_TEST_EXECUTABLE")
	if dsn == "" || bin == "" {
		t.Skip("requires a disposable pgvector database and the built butler-bench")
	}
	u, err := url.Parse(dsn)
	if err != nil || u.Path != "/butler_bench_test" || u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" {
		t.Fatal("dedicated loopback butler_bench_test required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	db, err := pg.OpenWithAuthorityReserve(ctx, pg.Config{URL: dsn, MaxConns: 12, MinConns: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	root := filepath.Join("..", "..", "..", "..")
	if err = db.RunMigrations(ctx, filepath.Join(root, "migrations")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BUTLER_BENCH_DB", dsn)
	dir := t.TempDir()
	if keep := os.Getenv("BUTLER_BENCH_FIXTURE_OUTPUT"); keep != "" {
		if err := os.Mkdir(keep, 0700); err != nil {
			t.Fatal(err)
		}
		dir = keep
	}
	secret := make([]byte, 24)
	rand.Read(secret)
	token := hex.EncodeToString(secret)
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err = os.WriteFile(tokenFile, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	revision, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	setup := filepath.Join(dir, "setup.json")
	if err = os.WriteFile(setup, []byte(fmt.Sprintf(`{"butler_commit":%q,"environment_sha256":"%x"}`, strings.TrimSpace(string(revision)), sha256.Sum256([]byte("synthetic public ingress fixture v1")))), 0600); err != nil {
		t.Fatal(err)
	}
	cli := func(args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatal("benchmark command", args[0], err)
		}
		return out
	}
	for _, fixture := range []struct{ id, answer string }{{"correct", "НЕТ ДАННЫХ"}, {"incorrect", "42"}} {
		func() {
			m := benchmarkFixtureModel{fixture.answer}
			p := db.Pool()
			repo, leases := session.NewPostgresRepository(p), session.NewPostgresLeaseManagerWithAuthorityReads(p, db.AuthorityReadPool())
			runRepo := run.NewPostgresRepository(p)
			runs, inputs := run.NewService(runRepo, nil), input.NewStore(p)
			tasks, history := task.NewStore(p), transcript.NewStore(p)
			policy := budget.DefaultPolicy()
			for _, id := range []string{"correct", "incorrect"} {
				policy.Models[m.Name()+":"+id] = budget.ModelPrice{Basis: "local_no_billing"}
			}
			traceDir := t.TempDir()
			core := flow.NewService(repo, leases, runs, history, m, flow.Config{ProviderName: m.Name(), ModelName: fixture.id, Completion: completion.New(p), Checkpoints: inputs, Tasks: tasks, Budgets: budget.NewControllerWithAuthorityReads(p, db.AuthorityReadPool(), policy), BenchmarkTraceDir: traceDir}, nil)
			worker := input.NewService(inputs, repo, core, runs, nil)
			worker.SetTaskControls(task.NewController(tasks, leases.Authority(), leases, runRepo, approval.NewPostgresRepository(p), inputs, history))
			server := api.NewServer(worker, nil)
			server.SetOwnerControlAuth(api.NewOwnerControlAuth(token, "fixture-owner"))
			httpServer := httptest.NewServer(server.HTTPHandler())
			defer httpServer.Close()
			workerCtx, stop := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() { defer close(done); _ = worker.Run(workerCtx) }()
			defer func() { stop(); <-done }()
			out := filepath.Join(dir, fixture.id)
			cli("run", "-cases", "missing-fact", "-setup", setup, "-endpoint", httpServer.URL, "-provider", m.Name(), "-model", fixture.id, "-owner-token-file", tokenFile, "-trace-dir", traceDir, "-out", out)
			cli("replay", "-out", filepath.Join(dir, fixture.id+"-replay.json"), out)
		}()
	}
	compare := filepath.Join(dir, "comparison.json")
	cli("compare", "-out", compare, filepath.Join(dir, "correct"), filepath.Join(dir, "incorrect"))
	raw, err := os.ReadFile(compare)
	var result struct {
		AOnly    int `json:"a_only_pass"`
		Unscored int `json:"unpaired_or_unattributed"`
	}
	if err != nil || json.Unmarshal(raw, &result) != nil || result.AOnly != 1 || result.Unscored != 0 {
		t.Fatalf("expected one paired correct result, got %+v (decode/read: %v)", result, err)
	}
}
