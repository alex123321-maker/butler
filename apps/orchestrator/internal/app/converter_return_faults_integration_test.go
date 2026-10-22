package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/delegation"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func (f *developerFixture) commonLifecycle(t *testing.T) *task.DelegationManager {
	t.Helper()
	initial, err := NewDelegationAdmission(f.f.pool, f.f.store, session.NewPostgresRepository(f.f.pool), durableinput.NewStore(f.f.pool), f.f.sources, task.DefaultDelegationLimits())
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewDelegationLifecycleManager(f.f.store, session.NewPostgresRepository(f.f.pool), durableinput.NewStore(f.f.pool), f.f.sources, task.DefaultDelegationLimits(), initial, f.service)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func (f *developerFixture) returnedChild(t *testing.T) (task.Record, generatedtools.Record, catalog.SourceRef) {
	t.Helper()
	child, worker, candidate := f.testedChild(t)
	result := f.finishChild(t, child, worker)
	m := f.commonLifecycle(t)
	collected, err := m.CollectGenerated(f.f.ctx, f.f.parent.TaskID, child.TaskID, result.ResultID)
	if err != nil || collected.Report == nil {
		t.Fatal("current generated technical return", err)
	}
	// Technical preparation is insufficient for the general semantic decision.
	if _, err = m.Decide(owneridentity.WithActor(f.f.ctx, owneridentity.Actor{OwnerID: f.f.parent.OwnerID, Channel: "web", DeliveryID: "no-semantic-promotion"}), f.f.parent.TaskID, child.TaskID, task.DelegationDecision{Accept: true, Report: *collected.Report, Evidence: []delegation.SourceRef{*collected.Report}}); err == nil {
		t.Fatal("technical return bypassed independent result verification")
	}
	return child, candidate, catalog.SourceRef(*collected.Report)
}

func (f *developerFixture) collectCall(t *testing.T, child task.Record, candidate generatedtools.Record, id string) (*toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract) {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"child_task_id": child.TaskID, "digest": candidate.Digest})
	for _, c := range generatedtools.Contracts() {
		if c.ToolName == "converters.collect" {
			return &toolbrokerv1.ToolCall{RunId: f.parentRun, ToolCallId: id, ToolName: c.ToolName, ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}, c
		}
	}
	t.Fatal("collection contract absent")
	return nil, nil
}

func (f *developerFixture) deleteSource(t *testing.T, ref catalog.SourceRef) {
	t.Helper()
	actor := owneridentity.WithActor(f.f.ctx, owneridentity.Actor{OwnerID: f.f.parent.OwnerID, Channel: "web", DeliveryID: "delete-converter-source"})
	if _, err := f.f.sources.Delete(actor, ref.ID, "delete", ref.Revision); err != nil {
		t.Fatal(err)
	}
}

func TestConverterDeveloperReturnReleaseRevocationIntegration(t *testing.T) {
	for _, target := range []string{"candidate", "need", "contract", "report", "final", "goal", "lease", "effect"} {
		t.Run(target, func(t *testing.T) {
			f := newDeveloperFixture(t)
			child, candidate, report := f.returnedChild(t)
			call, contract := f.collectCall(t, child, candidate, "return-before-revoke")
			p, err := f.auth.Prepare(f.f.ctx, call, contract)
			if err != nil {
				t.Fatal("prepare current return", err)
			}
			g, err := f.auth.Authorize(f.f.ctx, call, contract, p.Binding.ActionID, "", "")
			if err != nil {
				t.Fatal(err)
			}
			switch target {
			case "candidate":
				f.deleteSource(t, candidate.BodySource)
			case "need":
				f.deleteSource(t, f.needRef)
				var body string
				if err = f.f.pool.QueryRow(f.f.ctx, `SELECT body FROM memory_source_revisions WHERE source_id=$1`, candidate.BodySource.ID).Scan(&body); err != nil || body != "" {
					t.Fatal("candidate descendant bytes retained", err)
				}
			case "contract":
				f.deleteSource(t, f.specRef)
			case "report":
				f.deleteSource(t, report)
			case "final":
				var ref catalog.SourceRef
				var raw []byte
				if err = f.f.pool.QueryRow(f.f.ctx, `SELECT projection_json->'final_source' FROM converter_developer_returns WHERE child_task_id=$1`, child.TaskID).Scan(&raw); err != nil || json.Unmarshal(raw, &ref) != nil {
					t.Fatal(err)
				}
				f.deleteSource(t, ref)
			case "goal":
				f.goal.Status = "paused"
				if _, err = f.goals.PutGoal(owneridentity.WithActor(f.f.ctx, owneridentity.Actor{OwnerID: f.f.parent.OwnerID, Channel: "web", DeliveryID: "return-goal-pause"}), goal.GoalRequest{Goal: f.goal, ExpectedRevision: f.goal.Revision}); err != nil {
					t.Fatal(err)
				}
			case "lease":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-interval '1 second' WHERE run_id=$1`, f.parentRun)
			case "effect":
				_, err = f.f.pool.Exec(f.f.ctx, `UPDATE action_effects SET state='effect_unknown' WHERE action_id=(SELECT projection_json->>'test_effect_id' FROM converter_developer_returns WHERE child_task_id=$1)`, child.TaskID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = f.auth.Claim(f.f.ctx, g.ID); err == nil {
				t.Fatal("revoked technical return dispatched")
			}
			var grants int
			if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM memory_scope_grants WHERE task_id=$1 AND source_id=$2`, f.f.parent.TaskID, candidate.BodySource.ID).Scan(&grants); err != nil || grants != 0 {
				t.Fatal("failed claim widened parent source scope", grants, err)
			}
		})
	}
}

func TestConverterDeveloperCollectedReportDeletionIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	child, candidate, report := f.returnedChild(t)
	f.dispatch(t, f.parentRun, "converters.collect", map[string]any{"child_task_id": child.TaskID, "digest": candidate.Digest})
	f.deleteSource(t, report)
	raw, _ := json.Marshal(map[string]any{"digest": candidate.Digest})
	for _, c := range generatedtools.Contracts() {
		if c.ToolName == "converters.test" {
			if _, err := f.auth.Prepare(f.f.ctx, &toolbrokerv1.ToolCall{RunId: f.parentRun, ToolCallId: "late-check", ToolName: c.ToolName, ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}, c); err == nil {
				t.Fatal("collected source outlived technical report")
			}
		}
	}
	var lease session.LeaseRecord
	if err := f.f.pool.QueryRow(f.f.ctx, `SELECT lease_id,fence_epoch FROM session_ownership WHERE run_id=$1`, f.parentRun).Scan(&lease.LeaseID, &lease.FenceEpoch); err != nil {
		t.Fatal(err)
	}
	ctx := runContextForDeveloperTest(f, f.parentRun, lease)
	tx, err := f.f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if err = generatedtools.CheckDeveloperReturnLifecycleTx(ctx, tx, f.f.sources, f.parentRun); err == nil {
		t.Fatal("provider/recovery/final could release deleted report-derived input")
	}
}

func TestCommunicationConverterReturnStillRequiresOwnedScopeIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	child, candidate, _ := f.returnedChild(t)
	f.dispatch(t, f.parentRun, "converters.collect", map[string]any{"child_task_id": child.TaskID, "digest": candidate.Digest})
	var pins int
	if err := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM memory_run_sources WHERE run_id=$1 AND source_id=$2`, f.parentRun, candidate.BodySource.ID).Scan(&pins); err != nil || pins != 1 {
		t.Fatal("returned source was not captured", pins, err)
	}
	tx, err := f.f.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err = generatedtools.CheckDeveloperReturnLifecycleTx(context.Background(), tx, f.f.sources, f.parentRun); err == nil {
		t.Fatal("returned converter acquired ambient completed-run authority")
	}
}

func TestConverterDeveloperUnknownFinalCannotReturnIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	child, worker, _ := f.testedChild(t)
	result := f.finishChild(t, child, worker)
	if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE action_effects SET state='effect_unknown' WHERE action_id IN(SELECT effect_id FROM action_intents WHERE run_id='converter-child-run')`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.commonLifecycle(t).CollectGenerated(f.f.ctx, f.f.parent.TaskID, child.TaskID, result.ResultID); err == nil {
		t.Fatal("unknown child effect returned as tested candidate")
	}
	var count int
	if err := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM converter_developer_returns`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed report left immutable return", count, err)
	}
}

func TestConverterDeveloperReadySchemaIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	if err := generatedtools.CheckDeveloperReady(f.f.ctx, f.f.pool); err != nil {
		t.Fatal(err)
	}
	if _, err := f.f.pool.Exec(f.f.ctx, `ALTER TABLE converter_developer_returns DISABLE TRIGGER converter_return_immutable`); err != nil {
		t.Fatal(err)
	}
	if err := generatedtools.CheckDeveloperReady(context.Background(), f.f.pool); err == nil {
		t.Fatal("mutable return schema advertised ready")
	}
}

// Only the fixture can inject a technical checker. Expiring the actual worker
// while it waits must still roll back the helper's candidate grant, even when
// the caller deliberately commits the surrounding transaction after the error.
type expiringCandidateChecker struct{ owner string }

func (expiringCandidateChecker) PinnedMemorySources() []catalog.SourceRef { return nil }

func (c expiringCandidateChecker) CheckMemoryAccessTx(context.Context, pgx.Tx, []catalog.SourceRef, string) (string, error) {
	return c.owner, nil
}
func (c expiringCandidateChecker) FinalizeMemoryAccessTx(context.Context, pgx.Tx, []catalog.SourceRef, string) error {
	time.Sleep(300 * time.Millisecond)
	return nil
}

func TestConverterDeveloperCandidateGrantSavepointExpiryIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	child, candidate, _ := f.returnedChild(t)
	var lease session.LeaseRecord
	if err := f.f.pool.QueryRow(f.f.ctx, `SELECT lease_id,fence_epoch FROM session_ownership WHERE run_id=$1`, f.parentRun).Scan(&lease.LeaseID, &lease.FenceEpoch); err != nil {
		t.Fatal(err)
	}
	ctx := runContextForDeveloperTest(f, f.parentRun, lease)
	if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()+interval '200 milliseconds' WHERE run_id=$1`, f.parentRun); err != nil {
		t.Fatal(err)
	}
	tx, err := f.f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = f.f.sources.GrantDelegatedCandidateTx(ctx, tx, child.TaskID, candidate.BodySource, []catalog.SourceRef{f.needRef, f.specRef}, expiringCandidateChecker{f.f.parent.OwnerID}); err == nil {
		t.Fatal("candidate grant outlived its actual parent worker")
	}
	if err = tx.Commit(f.f.ctx); err != nil {
		t.Fatal("failed helper poisoned surrounding transaction", err)
	}
	var grants int
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM memory_scope_grants WHERE task_id=$1 AND source_id=$2`, f.f.parent.TaskID, candidate.BodySource.ID).Scan(&grants); err != nil || grants != 0 {
		t.Fatal("failed publication retained candidate scope grant", grants, err)
	}
}

func TestConverterDeveloperReadyRejectsFallbackSchemaIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	var own string
	if err := f.f.pool.QueryRow(f.f.ctx, `SELECT current_schema()`).Scan(&own); err != nil {
		t.Fatal(err)
	}
	fallback := own + "_fallback"
	quoted := pgx.Identifier{fallback}.Sanitize()
	if _, err := f.f.pool.Exec(f.f.ctx, "CREATE SCHEMA "+quoted+"; ALTER TABLE converter_developer_returns SET SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.f.pool.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE") })
	cfg := f.f.pool.Config().Copy()
	cfg.ConnConfig.RuntimeParams["search_path"] = pgx.Identifier{own}.Sanitize() + "," + quoted + ",public"
	other, err := pgxpool.NewWithConfig(f.f.ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	// Bare table and trigger lookups would find a structurally correct fallback.
	var fallbackVisible bool
	if err = other.QueryRow(f.f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='converter_developer_returns'::regclass AND tgname='converter_return_immutable' AND tgenabled='O')`).Scan(&fallbackVisible); err != nil || !fallbackVisible {
		t.Fatal("fallback regression fixture absent", err)
	}
	if err = generatedtools.CheckDeveloperReady(f.f.ctx, other); err == nil {
		t.Fatal("missing owned054 accepted another search_path schema")
	}
}

func TestConverterDeveloperReadyRequiresProjectionColumnsIntegration(t *testing.T) {
	for _, mutation := range []string{"ALTER TABLE converter_developer_returns DROP COLUMN projection_json", "ALTER TABLE converter_developer_snapshots ALTER COLUMN snapshot_hash DROP NOT NULL"} {
		t.Run(mutation, func(t *testing.T) {
			f := newDeveloperFixture(t)
			if _, err := f.f.pool.Exec(f.f.ctx, mutation); err != nil {
				t.Fatal(err)
			}
			if err := generatedtools.CheckDeveloperReady(f.f.ctx, f.f.pool); err == nil {
				t.Fatal("damaged owned schema advertised readiness")
			}
		})
	}
}

func TestConverterDeveloperAmbiguousReturnDeniesClaimIntegration(t *testing.T) {
	f := newDeveloperFixture(t)
	child, candidate, _ := f.returnedChild(t)
	// Fault injection simulates an invalid retained/restore row. The owned
	// publisher could not derive this projection from the second child's final.
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tasks SELECT (jsonb_populate_record(NULL::tasks,to_jsonb(t)||'{"task_id":"damaged-second-child"}')).* FROM tasks t WHERE task_id=$1`, []any{child.TaskID}},
		{`INSERT INTO converter_developer_snapshots(task_id,owner_id,parent_task_id,parent_revision,parent_constraints_revision,execution_action_id,effect_id,authorization_id,action_hash,args_hash,contract_hash,snapshot_hash,accepted_json) SELECT 'damaged-second-child',owner_id,parent_task_id,parent_revision,parent_constraints_revision,execution_action_id,(SELECT effect_id FROM action_intents WHERE run_id=$2 AND binding->>'tool_name'='converters.discover' LIMIT 1),authorization_id,action_hash,args_hash,contract_hash,snapshot_hash,accepted_json FROM converter_developer_snapshots WHERE task_id=$1`, []any{child.TaskID, f.parentRun}},
		{`INSERT INTO converter_developer_returns(child_task_id,owner_id,parent_task_id,result_id,child_run_id,digest,projection_hash,projection_json) SELECT 'damaged-second-child',owner_id,parent_task_id,result_id,child_run_id,digest,projection_hash,projection_json FROM converter_developer_returns WHERE child_task_id=$1`, []any{child.TaskID}},
	} {
		if _, err := f.f.pool.Exec(f.f.ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}
	call, contract := f.collectCall(t, child, candidate, "ambiguous-return")
	p, err := f.auth.Prepare(f.f.ctx, call, contract)
	if err != nil {
		t.Fatal("exact child prepare", err)
	}
	g, err := f.auth.Authorize(f.f.ctx, call, contract, p.Binding.ActionID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.auth.Claim(f.f.ctx, g.ID); err == nil {
		t.Fatal("ambiguous return chose a first row")
	}
	var grants int
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM memory_scope_grants WHERE task_id=$1 AND source_id=$2`, f.f.parent.TaskID, candidate.BodySource.ID).Scan(&grants); err != nil || grants != 0 {
		t.Fatal("ambiguous metadata widened parent scope", grants, err)
	}
}
