package workspaceexec

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5/pgxpool"
)

type discardFixture struct {
	*runtimeFixture
	workspaceID, ticket string
	req                 DiscardRequest
	owner               context.Context
}

func TestWorkspaceOwnerDiscardActualRuntimeProcessLossIntegration(t *testing.T) {
	if os.Getenv("BUTLER_DISCARD_PROCESS_CHILD") == "1" {
		cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_POSTGRES_URL"))
		if err != nil {
			t.Fatal("synthetic child PG config")
		}
		cfg.MaxConns = 2
		cfg.ConnConfig.RuntimeParams["search_path"] = os.Getenv("BUTLER_DISCARD_PROCESS_SCHEMA")
		pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
		if err != nil {
			t.Fatal("child pool")
		}
		defer pool.Close()
		manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "independent-empty-deletion-anchor"))
		if manifest.Initialize() != nil {
			t.Fatal("fixture anchor")
		}
		sources := catalog.New(pool, manifest)
		provenance, err := NewProvenance(pool, sources)
		if err != nil {
			t.Fatal("provenance")
		}
		resourcesStore, _ := resources.New(resources.WorkerGuard{}, noOwnerRecovery{})
		dispatch, err := NewDispatchStore(pool, resourcesStore, budget.NewController(pool, budget.DefaultPolicy()), provenance, Profile{ID: "fixture-runtime", Target: os.Getenv("BUTLER_DISCARD_PROCESS_TARGET")})
		if err != nil {
			t.Fatal("child dispatch")
		}
		runtime, err := NewRuntime(context.Background(), dispatch, os.Getenv("BUTLER_DISCARD_PROCESS_ROOT"), strings.Repeat("fixture-workspace-service-", 3))
		if err != nil {
			t.Fatal("actual child runtime")
		}
		defer runtime.Close()
		println("workspace-runtime-ready")
		select {}
	}
	f := newDiscardFixture(t, false)
	status := f.request(t)
	if f.runtime.Close() != nil {
		t.Fatal("parent runtime drain")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestWorkspaceOwnerDiscardActualRuntimeProcessLossIntegration$")
	cmd.Env = append(os.Environ(), "BUTLER_DISCARD_PROCESS_CHILD=1", "BUTLER_DISCARD_PROCESS_SCHEMA="+f.pool.Config().ConnConfig.RuntimeParams["search_path"], "BUTLER_DISCARD_PROCESS_ROOT="+f.root, "BUTLER_DISCARD_PROCESS_TARGET="+f.dispatch.profile.Target)
	pipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Start() != nil {
		t.Fatal("child process start")
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	ready := make(chan bool, 1)
	go func() {
		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			if scanner.Text() == "workspace-runtime-ready" {
				ready <- true
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		if !ok {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			t.Fatal("real runtime child never ready")
		}
	case <-time.After(8 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("child startup timeout")
	}
	if other, err := NewRuntime(f.ctx, f.dispatch, f.root, f.token); err == nil {
		_ = other.Close()
		t.Fatal("live old physical process admitted recovery runtime")
	}
	f.retained(t, false)
	if cmd.Process.Kill() != nil {
		t.Fatal("kill child")
	}
	if cmd.Wait() == nil {
		t.Fatal("SIGKILL child reported normal completion")
	}
	replacement, err := NewRuntime(f.ctx, f.dispatch, f.root, f.token)
	if err != nil {
		t.Fatal("kernel did not release root after process death")
	}
	t.Cleanup(func() { _ = replacement.Close() })
	f.runtime = replacement
	if replacement.ProcessRecovery(f.ctx, status.RecoveryID) != nil {
		t.Fatal("matching inode recovery after actual process loss denied")
	}
	f.retained(t, true)
}

func TestWorkspaceOwnerDiscardRechecksAfterActualPool2RowWaitIntegration(t *testing.T) {
	for _, mode := range []string{"source", "resource"} {
		t.Run(mode, func(t *testing.T) {
			f := newDiscardFixture(t, false)
			f.expired(t)
			blocker, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer blocker.Rollback(context.Background())
			if mode == "source" {
				_, err = blocker.Exec(f.ctx, `UPDATE memory_sources SET access_revision=access_revision+1 WHERE source_id=$1`, f.source.ID)
			} else {
				_, err = blocker.Exec(f.ctx, `UPDATE execution_resources SET version=version+1 WHERE resource_id=$1`, "workspace:"+f.workspaceID)
			}
			if err != nil {
				t.Fatal("row barrier")
			}
			result := make(chan error, 1)
			go func() { _, err := f.runtime.recovery.RequestDiscard(f.owner, f.req); result <- err }()
			deadline := time.Now().Add(3 * time.Second)
			for {
				if _, err := blocker.Exec(f.ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
					t.Fatal("row wait snapshot refresh")
				}
				var waiting bool
				if blocker.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity a WHERE pg_backend_pid()=ANY(pg_blocking_pids(a.pid)))`).Scan(&waiting) != nil {
					t.Fatal("pool2 observer")
				}
				if waiting {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("actual recovery row wait missing")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if blocker.Commit(f.ctx) != nil {
				t.Fatal("barrier commit")
			}
			select {
			case err := <-result:
				if err == nil {
					t.Fatal("changed authority/resource after wait admitted")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("recovery exceeded row-wait bound")
			}
			var count int
			if f.pool.QueryRow(f.ctx, `SELECT count(*) FROM code_workspace_recoveries`).Scan(&count) != nil || count != 0 {
				t.Fatal("denial queued erasure")
			}
			if _, err = os.Stat(filepath.Join(f.root, f.workspaceID, "private", "write-pending")); err != nil {
				t.Fatal("refusal changed private copy")
			}
			if mode == "source" {
				f.retained(t, false)
			}
		})
	}
}

func TestWorkspaceOwnerDiscardDoesNotBlockIndependentActualCopyIntegration(t *testing.T) {
	f := newRuntimeFixture(t)
	req, ticket := f.claim(t, "first-copy", CreateTool, `{}`)
	result := f.execute(t, req, ticket)
	var first struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if result.Status != "completed" || json.Unmarshal([]byte(result.ResultJson), &first) != nil {
		t.Fatal("first copy")
	}
	release, err := f.runtime.copies.acquire(f.ctx, first.WorkspaceID, false)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	req, ticket = f.claim(t, "independent-copy", CreateTool, `{}`)
	result = f.execute(t, req, ticket)
	var second struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if result.Status != "completed" || json.Unmarshal([]byte(result.ResultJson), &second) != nil || second.WorkspaceID == first.WorkspaceID {
		t.Fatal("held old-copy lane globally blocked real private dispatch")
	}
	for _, id := range []string{first.WorkspaceID, second.WorkspaceID} {
		if _, err = os.Stat(filepath.Join(f.root, id, "tree")); err != nil {
			t.Fatal("independent physical copy absent")
		}
	}
}

func TestWorkspaceOwnerDiscardRetainsImmutableDecisionAndDispatchIntegration(t *testing.T) {
	f := newDiscardFixture(t, false)
	status := f.request(t)
	for _, query := range []string{
		`UPDATE code_workspace_recoveries SET owner_id='foreign' WHERE recovery_id=$1`,
		`UPDATE code_workspace_recoveries SET choice='adopt' WHERE recovery_id=$1`,
		`DELETE FROM code_workspace_recoveries WHERE recovery_id=$1`,
	} {
		if _, err := f.pool.Exec(f.ctx, query, status.RecoveryID); err == nil {
			t.Fatal("immutable owner decision changed")
		}
	}
	for _, query := range []string{
		`UPDATE code_workspace_dispatches SET resource_fence=resource_fence+1 WHERE ticket_id=$1`,
		`UPDATE code_workspace_executor_bindings SET root_identity_hash=repeat('a',64) WHERE ticket_id=$1`,
		`DELETE FROM code_workspace_executor_bindings WHERE ticket_id=$1`,
	} {
		if _, err := f.pool.Exec(f.ctx, query, f.ticket); err == nil {
			t.Fatal("old executor/dispatch binding changed")
		}
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE code_workspace_recoveries SET phase='discarded',finished_at=clock_timestamp() WHERE recovery_id=$1`, status.RecoveryID); err == nil {
		t.Fatal("one-use attempt skipped")
	}
	changed := f.req
	changed.TaskRevision++
	if _, err := f.runtime.recovery.RequestDiscard(f.owner, changed); err == nil {
		t.Fatal("delivery identity reused for different CAS")
	}
	if f.runtime.ProcessRecovery(f.ctx, status.RecoveryID) != nil {
		t.Fatal("actual discard after immutable denials")
	}
	if _, err := f.pool.Exec(f.ctx, `UPDATE code_workspace_erasure_receipts SET observation_hash=repeat('b',64) WHERE recovery_id=$1`, status.RecoveryID); err == nil {
		t.Fatal("independent receipt changed")
	}
	f.retained(t, true)
}

func TestWorkspaceOwnerDiscardFairQueueSkipsRetainedDeniedRowsIntegration(t *testing.T) {
	f := newDiscardFixture(t, false)
	f.expired(t)
	// Retained unusable history is deliberately NOT owner authority: the old
	// immutable ticket's copy differs. Every such row must deny without effects.
	// A fixed first-page scan would hide the later genuine owner choice forever.
	for i := 1; i <= 16; i++ {
		id := fmt.Sprintf("%032x", i)
		if _, err := f.pool.Exec(f.ctx, `INSERT INTO code_workspace_recoveries(recovery_id,owner_id,channel,delivery_id,request_hash,workspace_id,ticket_id,resource_version,workspace_generation,task_revision,constraints_revision,phase,attempted_at,created_at) VALUES($1,'wp-owner','web',$2,repeat('a',64),$1,$3,1,0,2,1,'erasing',clock_timestamp(),clock_timestamp()-interval '1 minute')`, id, "retained-denied-"+id, f.ticket); err != nil {
			t.Fatal("retained denial fixture")
		}
	}
	status, err := f.runtime.recovery.RequestDiscard(f.owner, f.req)
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(f.ctx, 8*time.Second)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- f.runtime.RunOwnerRecovery(ctx) }()
	for {
		var phase string
		if f.pool.QueryRow(ctx, `SELECT phase FROM code_workspace_recoveries WHERE recovery_id=$1`, status.RecoveryID).Scan(&phase) != nil {
			t.Fatal("owner queue observation")
		}
		if phase == "discarded" {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("retained unknown first page starved independent owner choice")
		case <-time.After(30 * time.Millisecond):
		}
	}
	stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("owner job did not stop")
	}
	f.retained(t, true)
	var untouched int
	if f.pool.QueryRow(f.ctx, `SELECT count(*) FROM code_workspace_recoveries WHERE delivery_id LIKE 'retained-denied-%' AND phase='erasing'`).Scan(&untouched) != nil || untouched != 16 {
		t.Fatal("denied history silently reconciled")
	}
}

func TestWorkspaceOwnerDiscardTerminalEnumWithoutReceiptIsNotProofIntegration(t *testing.T) {
	f := newDiscardFixture(t, false)
	f.expired(t)
	id := strings.Repeat("b", 32)
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO code_workspace_recoveries(recovery_id,owner_id,channel,delivery_id,request_hash,workspace_id,ticket_id,resource_version,workspace_generation,task_revision,constraints_revision,phase,finished_at) VALUES($1,'wp-owner','web','incomplete-terminal-history',$2,$3,$4,$5,$6,$7,$8,'discarded',clock_timestamp())`, id, requestDigest(f.req), f.workspaceID, f.ticket, f.req.ResourceVersion, f.req.WorkspaceGeneration, f.req.TaskRevision, f.req.ConstraintsRevision); err != nil {
		t.Fatal("incomplete history fixture")
	}
	if _, err := f.runtime.recovery.Get(f.owner, f.workspaceID); err == nil {
		t.Fatal("terminal phase invented independently proved erasure")
	}
	listed, err := f.runtime.recovery.ListForTask(f.owner, "wp-task")
	if err != nil || len(listed) != 1 || listed[0].Phase != "quarantined" || listed[0].CanDiscard {
		t.Fatal("owner discovery accepted unproved terminal enum", err)
	}
	f.retained(t, false)
}

func newDiscardFixture(t *testing.T, write bool) *discardFixture {
	t.Helper()
	return newDiscardFixtureWithInputDeadline(t, write, 1500*time.Millisecond)
}

func newDiscardFixtureWithInputDeadline(t *testing.T, write bool, inputTTL time.Duration) *discardFixture {
	t.Helper()
	f := newRuntimeFixture(t)
	var id string
	if write {
		req, ticket := f.claim(t, "recovery-initial-create", CreateTool, `{}`)
		result := f.execute(t, req, ticket)
		var created struct {
			WorkspaceID string `json:"workspace_id"`
		}
		if result.Status != "completed" || json.Unmarshal([]byte(result.ResultJson), &created) != nil {
			t.Fatal("initial real copy")
		}
		id = created.WorkspaceID
	}
	// The real admission takes this natural finite input deadline; no clock,
	// resource expiry or production hook is replaced by the fixture.
	if _, err := f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET claim_expires_at=clock_timestamp()+$1*interval '1 millisecond' WHERE input_id='wp-input'`, inputTTL.Milliseconds()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `CREATE FUNCTION fixture_recovery_confirmation_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture independent publication failure'; END $$; CREATE TRIGGER fixture_recovery_confirmation_failure BEFORE INSERT ON code_workspace_sources FOR EACH ROW EXECUTE FUNCTION fixture_recovery_confirmation_failure()`); err != nil {
		t.Fatal(err)
	}
	tool, args := CreateTool, `{}`
	if write {
		tool = WriteTool
		raw, _ := json.Marshal(map[string]any{"workspace_id": id, "path": "notes.go", "content": "package notes\n", "expected_version": 0, "expected_sha256": "", "expected_bytes": 0})
		args = string(raw)
	}
	req, ticket := f.claim(t, "recovery-unknown", tool, args)
	result := f.execute(t, req, ticket)
	if result.Status != "effect_unknown" {
		t.Fatal("physical publication fault invented completion")
	}
	var authID string
	if f.pool.QueryRow(f.ctx, `SELECT a.workspace_id,d.authorization_id FROM code_workspace_dispatches d JOIN code_workspace_authorizations a USING(authorization_id) WHERE ticket_id=$1`, ticket).Scan(&id, &authID) != nil {
		t.Fatal("missing original dispatch")
	}
	grant, err := f.auth.GetGrant(f.ctx, authID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.service.Outcome(f.ctx, grant, result, false); err != nil {
		t.Fatal("unknown outcome journal", err)
	}
	if _, err = os.Stat(filepath.Join(f.root, id, "private", "write-pending")); err != nil {
		t.Fatal("physical pending checkpoint absent")
	}
	if _, err = f.pool.Exec(f.ctx, `DROP TRIGGER fixture_recovery_confirmation_failure ON code_workspace_sources;DROP FUNCTION fixture_recovery_confirmation_failure(); UPDATE tasks SET status='cancelled',revision=revision+1 WHERE task_id='wp-task';UPDATE session_ownership SET lease_id='',fence_epoch=fence_epoch+1,expires_at=clock_timestamp() WHERE session_key='wp-session'`); err != nil {
		t.Fatal(err)
	}
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "wp-owner", Channel: "web", DeliveryID: "discard-one"})
	var version, generation, revision, constraints int64
	if f.pool.QueryRow(f.ctx, `SELECT r.version,COALESCE(w.generation,0),t.revision,t.constraints_revision FROM code_workspace_dispatches d JOIN execution_resources r USING(resource_id) JOIN code_workspace_authorizations a USING(authorization_id) JOIN tasks t ON t.task_id=a.source_context->>'task_id' LEFT JOIN code_workspaces w ON w.workspace_id=a.workspace_id WHERE ticket_id=$1`, ticket).Scan(&version, &generation, &revision, &constraints) != nil {
		t.Fatal("recovery CAS")
	}
	return &discardFixture{f, id, ticket, DiscardRequest{WorkspaceID: id, ResourceVersion: version, WorkspaceGeneration: generation, TaskRevision: revision, ConstraintsRevision: constraints}, owner}
}
func (f *discardFixture) expired(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(resources.MaxLeaseTTL + time.Second)
	for {
		var expired bool
		if f.pool.QueryRow(f.ctx, `SELECT l.expires_at<=clock_timestamp() FROM code_workspace_dispatches d JOIN execution_resource_leases l ON l.lease_id=d.resource_lease_id WHERE ticket_id=$1`, f.ticket).Scan(&expired) != nil {
			t.Fatal("expiry observation")
		}
		if expired {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("natural lease never expired")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
func (f *discardFixture) request(t *testing.T) RecoveryStatus {
	t.Helper()
	f.expired(t)
	status, err := f.runtime.recovery.RequestDiscard(f.owner, f.req)
	if err != nil || status.Phase != "requested" {
		t.Fatal("explicit owner discard denied", err)
	}
	return status
}
func (f *discardFixture) retained(t *testing.T, discarded bool) {
	t.Helper()
	var inflight, phase, effect, lease, reason string
	var version, fence, originalFence, receipts, reservations int64
	if f.pool.QueryRow(f.ctx, `SELECT r.inflight_lease_id,r.version,r.fence_epoch,d.resource_fence,d.phase,e.state,l.state,COALESCE(w.blocked_reason,''),(SELECT count(*) FROM code_workspace_erasure_receipts),(SELECT count(*) FROM execution_budget_reservations) FROM code_workspace_dispatches d JOIN execution_resources r USING(resource_id) JOIN execution_resource_leases l ON l.lease_id=d.resource_lease_id JOIN action_effects e ON e.action_id=d.effect_id JOIN code_workspace_authorizations a ON a.authorization_id=d.authorization_id LEFT JOIN code_workspaces w ON w.workspace_id=a.workspace_id WHERE d.ticket_id=$1`, f.ticket).Scan(&inflight, &version, &fence, &originalFence, &phase, &effect, &lease, &reason, &receipts, &reservations) != nil {
		t.Fatal("retention projection")
	}
	if phase != "running" || effect != "effect_unknown" || reservations < 1 {
		t.Fatal("original unknown effect or spent budget changed", phase, effect, reservations)
	}
	if discarded {
		if inflight != "" || version != f.req.ResourceVersion+1 || fence != originalFence+1 || lease != "reconciled" || reason != "owner_discarded" || receipts != 1 {
			t.Fatal("discard monotonic fence/receipt/retirement", version, fence, lease, reason, receipts)
		}
	} else if inflight == "" || version != f.req.ResourceVersion || fence != originalFence || receipts != 0 {
		t.Fatal("denial changed unknown resource")
	}
}
func TestWorkspaceOwnerDiscardUnknownPhysicalCopyIntegration(t *testing.T) {
	for _, write := range []bool{false, true} {
		t.Run(map[bool]string{false: "create_rolled_back", true: "write_quarantined"}[write], func(t *testing.T) {
			// This negative case needs an actually live lease after preparation.
			// Select a finite input deadline before normal admission; never edit
			// the resulting resource lease, substitute the clock or skip expiry.
			f := newDiscardFixtureWithInputDeadline(t, write, 10*time.Second)
			remaining := func() float64 {
				t.Helper()
				var seconds float64
				if err := f.pool.QueryRow(f.ctx, `SELECT EXTRACT(EPOCH FROM l.expires_at-clock_timestamp())::double precision FROM code_workspace_dispatches d JOIN execution_resource_leases l ON l.lease_id=d.resource_lease_id WHERE ticket_id=$1`, f.ticket).Scan(&seconds); err != nil {
					t.Fatal("old lease remaining-time observation", err)
				}
				return seconds
			}
			if seconds := remaining(); seconds < 5 {
				t.Fatalf("fixture needs a live lease before recovery; remaining %.3fs", seconds)
			}
			if _, err := f.runtime.recovery.RequestDiscard(f.owner, f.req); !errors.Is(err, ErrRecoveryConflict) {
				t.Fatal("live old lease did not return the exact recovery conflict", err)
			}
			if seconds := remaining(); seconds <= 0 {
				t.Fatal("fixture lease expired during the negative assertion")
			}
			f.retained(t, false)
			var decisions int
			if err := f.pool.QueryRow(f.ctx, `SELECT count(*) FROM code_workspace_recoveries`).Scan(&decisions); err != nil || decisions != 0 {
				t.Fatal("denied recovery published an owner intent", decisions, err)
			}
			status := f.request(t)
			listed, err := f.runtime.recovery.ListForTask(f.owner, "wp-task")
			if err != nil || len(listed) != 1 || listed[0].WorkspaceID != f.workspaceID || listed[0].RecoveryID != status.RecoveryID || listed[0].Phase != "requested" {
				t.Fatal("owner could not discover unknown opaque copy", err)
			}
			if _, err = f.runtime.recovery.ListForTask(context.Background(), "wp-task"); err == nil {
				t.Fatal("anonymous owner-copy listing")
			}
			if err := f.runtime.ProcessRecovery(f.ctx, status.RecoveryID); err != nil {
				t.Fatal("actual synced owner erasure", err)
			}
			if _, err := os.Stat(filepath.Join(f.root, f.workspaceID)); !os.IsNotExist(err) {
				t.Fatal("private copy not erased")
			}
			f.retained(t, true)
			replay, err := f.runtime.recovery.RequestDiscard(f.owner, f.req)
			if err != nil || replay.Phase != "discarded" || replay.RecoveryID != status.RecoveryID || replay.CanDiscard {
				t.Fatal("lost owning ACK did not return current terminal observation", err)
			}
			raw, _ := json.Marshal(replay)
			for _, private := range []string{f.root, f.token, f.ticket, "wp-lease", "package notes"} {
				if strings.Contains(string(raw), private) {
					t.Fatal("recovery exposed private material")
				}
			}
			if err = f.runtime.ProcessRecovery(f.ctx, status.RecoveryID); err != nil {
				t.Fatal("completed duplicate processing")
			}
			f.retained(t, true)
		})
	}
}
func TestWorkspaceOwnerDiscardRejectsForeignStaleAndFabricatedProofIntegration(t *testing.T) {
	for _, mode := range []string{"anonymous", "foreign", "stale_version", "stale_generation", "stale_task", "changed_source", "live_tree", "live_worker", "fake_receipt", "missing_binding", "disabled_history"} {
		t.Run(mode, func(t *testing.T) {
			f := newDiscardFixture(t, false)
			f.expired(t)
			ctx, req := f.owner, f.req
			switch mode {
			case "anonymous":
				ctx = context.Background()
			case "foreign":
				ctx = owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "foreign", Channel: "web", DeliveryID: "discard"})
			case "stale_version":
				req.ResourceVersion++
			case "stale_generation":
				req.WorkspaceGeneration++
			case "stale_task":
				req.TaskRevision++
			case "changed_source":
				if _, err := f.pool.Exec(f.ctx, `UPDATE memory_sources SET access_revision=access_revision+1 WHERE source_id=$1`, f.source.ID); err != nil {
					t.Fatal(err)
				}
			case "live_tree":
				if _, err := f.pool.Exec(f.ctx, `UPDATE tasks SET status='active' WHERE task_id='wp-task'`); err != nil {
					t.Fatal(err)
				}
			case "live_worker":
				if _, err := f.pool.Exec(f.ctx, `UPDATE session_ownership SET lease_id='actual-successor',expires_at=clock_timestamp()+interval '1 minute' WHERE session_key='wp-session'`); err != nil {
					t.Fatal(err)
				}
			case "missing_binding":
				if _, err := f.pool.Exec(f.ctx, `ALTER TABLE code_workspace_executor_bindings DISABLE TRIGGER code_workspace_executor_bindings_immutable;DELETE FROM code_workspace_executor_bindings;ALTER TABLE code_workspace_executor_bindings ENABLE TRIGGER code_workspace_executor_bindings_immutable`); err != nil {
					t.Fatal(err)
				}
			case "disabled_history":
				if _, err := f.pool.Exec(f.ctx, `ALTER TABLE code_workspace_recoveries DISABLE TRIGGER code_workspace_recoveries_immutable`); err != nil {
					t.Fatal(err)
				}
			case "fake_receipt":
				store, _ := resources.New(resources.WorkerGuard{}, f.runtime.recovery)
				tx, err := f.pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(context.Background())
				var id, lease string
				if tx.QueryRow(ctx, `SELECT resource_id,resource_lease_id FROM code_workspace_dispatches WHERE ticket_id=$1`, f.ticket).Scan(&id, &lease) != nil {
					t.Fatal("lease")
				}
				if _, err = store.ReconcileTx(ctx, tx, "wp-owner", id, lease, "model-says-erased", req.ResourceVersion); err == nil {
					t.Fatal("owner/model receipt satisfied physical guard")
				}
				_ = tx.Rollback(ctx)
				f.retained(t, false)
				return
			}
			if _, err := f.runtime.recovery.RequestDiscard(ctx, req); err == nil {
				t.Fatal("stale or foreign recovery admitted")
			}
			f.retained(t, false)
		})
	}
}
func TestWorkspaceOwnerDiscardLostErasureAcknowledgmentObservesOnlyIntegration(t *testing.T) {
	for _, removed := range []bool{false, true} {
		t.Run(map[bool]string{false: "copy_still_present", true: "already_synced_absent"}[removed], func(t *testing.T) {
			f := newDiscardFixture(t, false)
			status := f.request(t)
			// Durable marker is exactly the phase committed before local effect. The
			// retained copy must never be removed on this resumption branch.
			if _, err := f.pool.Exec(f.ctx, `UPDATE code_workspace_recoveries SET phase='erasing',attempted_at=clock_timestamp() WHERE recovery_id=$1`, status.RecoveryID); err != nil {
				t.Fatal(err)
			}
			if removed {
				if err := f.runtime.purge.PurgeWorkspace(f.ctx, f.workspaceID); err != nil {
					t.Fatal(err)
				}
			}
			if err := f.runtime.Close(); err != nil {
				t.Fatal(err)
			}
			replacement, err := NewRuntime(f.ctx, f.dispatch, f.root, f.token)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = replacement.Close() })
			f.runtime = replacement
			err = replacement.ProcessRecovery(f.ctx, status.RecoveryID)
			if removed {
				if err != nil {
					t.Fatal("independent absence could not finalize", err)
				}
				f.retained(t, true)
				return
			}
			if err == nil {
				t.Fatal("lost erasure ACK caused physical replay")
			}
			if _, err = os.Stat(filepath.Join(f.root, f.workspaceID, "private", "write-pending")); err != nil {
				t.Fatal("resumption touched uncertain copy")
			}
			f.retained(t, false)
			req := f.req
			req.PreviousRecoveryID = status.RecoveryID
			owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "wp-owner", Channel: "web", DeliveryID: "fresh-explicit-discard"})
			fresh, err := replacement.recovery.RequestDiscard(owner, req)
			if err != nil || fresh.RecoveryID == status.RecoveryID {
				t.Fatal("fresh owner choice did not create distinct attempt", err)
			}
			if err = replacement.ProcessRecovery(f.ctx, fresh.RecoveryID); err != nil {
				t.Fatal(err)
			}
			f.retained(t, true)
		})
	}
}
func TestWorkspaceOwnerDiscardReplacementRootAndActiveCopyRefuseIntegration(t *testing.T) {
	f := newDiscardFixture(t, false)
	status := f.request(t)
	release, err := f.runtime.copies.acquire(f.ctx, f.workspaceID, false)
	if err != nil {
		t.Fatal(err)
	}
	bounded, stop := context.WithTimeout(f.ctx, 80*time.Millisecond)
	err = f.runtime.ProcessRecovery(bounded, status.RecoveryID)
	stop()
	if err == nil {
		release()
		t.Fatal("active old copy executor was not drained")
	}
	// A distinct copy can enter despite this old executor, without a global lane.
	other, err := f.runtime.copies.acquire(f.ctx, strings.Repeat("a", 32), true)
	if err != nil {
		release()
		t.Fatal("unrelated copy was globally blocked")
	}
	other()
	release()
	f.retained(t, false)
	if err = f.runtime.Close(); err != nil {
		t.Fatal(err)
	}
	old := f.root + "-retained-old"
	if err = os.Rename(f.root, old); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(old) })
	if err = os.Mkdir(f.root, 0700); err != nil {
		t.Fatal(err)
	}
	replacement, err := NewRuntime(f.ctx, f.dispatch, f.root, f.token)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = replacement.Close() })
	if replacement.ProcessRecovery(f.ctx, status.RecoveryID) == nil {
		t.Fatal("replacement root claimed old executor quiescence")
	}
	if _, err = os.Stat(filepath.Join(old, f.workspaceID)); err != nil {
		t.Fatal("old private copy lost")
	}
	f.retained(t, false)
}
