package input

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/jackc/pgx/v5/pgxpool"
)

type killedApprovalProof struct {
	Session  string `json:"session"`
	Input    string `json:"input"`
	Task     string `json:"task"`
	Run      string `json:"run"`
	ToolCall string `json:"tool_call"`
}

// This supplements replacement-Service fixtures with a genuinely killed worker
// process. The effect remains a controlled test adapter; no working credential
// or provider login is required. The authoritative checkpoint is real PG.
func TestApprovalWorkerSIGKILLRestartIntegration(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("lab-only Linux SIGKILL proof")
	}
	if os.Getenv("BUTLER_TEST_POSTGRES_URL") == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL required")
	}
	if os.Getenv("BUTLER_ACCEPTANCE_CHILD") == "approval-wait" {
		f := newLifecycleFixture(t)
		worker, _ := f.worker(f.store, nil)
		worker.process(f.ctx, f.claim(t))
		r := f.assertReceipt(t, "waiting_for_user")
		a := f.decision(t)
		if f.tools.calls != 0 {
			t.Fatal("effect occurred before approval")
		}
		raw, err := json.Marshal(killedApprovalProof{f.key, r.InputID, r.TaskID, r.RunID, a.ToolCallID})
		if err != nil {
			t.Fatal(err)
		}
		path := os.Getenv("BUTLER_ACCEPTANCE_READY_FILE")
		if err = os.WriteFile(path+".tmp", raw, 0600); err != nil {
			t.Fatal(err)
		}
		if err = os.Rename(path+".tmp", path); err != nil {
			t.Fatal(err)
		}
		// A ready file is emitted only after durable approval/continuation commit.
		// Parent terminates this process; cleanup handlers cannot run on SIGKILL.
		select {}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "ready.json")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestApprovalWorkerSIGKILLRestartIntegration$", "-test.timeout=20s")
	cmd.Env = append(os.Environ(), "BUTLER_ACCEPTANCE_CHILD=approval-wait", "BUTLER_ACCEPTANCE_READY_FILE="+path)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	var proof killedApprovalProof
	for {
		raw, readErr := os.ReadFile(path)
		if readErr == nil {
			if json.Unmarshal(raw, &proof) != nil || proof.Run == "" || proof.Input == "" {
				t.Fatal("invalid ready proof")
			}
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("child did not persist approval checkpoint")
		case <-time.After(20 * time.Millisecond):
		}
	}
	if err = cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	err = cmd.Wait()
	waited = true
	exit, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatal("worker did not die by SIGKILL", err)
	}
	status, ok := exit.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
		t.Fatal("not SIGKILL", exit.Sys())
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_POSTGRES_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, proof.Session)
	}()
	f := &lifecycleFixture{ctx: ctx, pool: pool, key: proof.Session, store: NewStore(pool), sessions: session.NewPostgresRepository(pool), runs: run.NewService(run.NewPostgresRepository(pool), nil), tasks: task.NewStore(pool), transcript: transcript.NewStore(pool)}
	repo := approvals.NewPostgresRepository(pool)
	repo.SetCheckpointHooks(f.store)
	f.approvals = approvals.NewService(repo, nil)
	f.tools = &lifecycleTools{pool: pool, approvals: f.approvals}
	r, err := f.store.Get(ctx, proof.Input)
	if err != nil || r.Status != "waiting_for_user" || r.RunID != proof.Run {
		t.Fatal("kill lost durable work", r, err)
	}
	f.receipt = &flow.ExecutionResult{InputID: proof.Input, TaskID: proof.Task, RunID: proof.Run}
	if _, changed, err := f.resolve(t, true, ownerActor()); err != nil || !changed {
		t.Fatal("post-kill decision", changed, err)
	}
	if _, changed, err := f.resolve(t, true, ownerActor()); err != nil || changed {
		t.Fatal("duplicate decision after kill", changed, err)
	}
	worker, provider := f.worker(f.store, nil)
	worker.process(ctx, f.claim(t))
	finished := f.assertReceipt(t, "completed")
	if finished.RunID != proof.Run || f.tools.calls != 1 {
		t.Fatal("restarted worker changed intent/repeated dispatch", finished.RunID, f.tools.calls)
	}
	history, err := f.transcript.GetTranscript(ctx, proof.Session)
	if err != nil || len(history.ToolCalls) != 1 || history.ToolCalls[0].ToolCallID != proof.ToolCall {
		t.Fatal("stable action transcript", history, err)
	}
	if len(provider.calls) != 1 || provider.calls[0].Context.ProviderSessionRef != nil {
		t.Fatal("recovery borrowed provider/session authority")
	}
}
