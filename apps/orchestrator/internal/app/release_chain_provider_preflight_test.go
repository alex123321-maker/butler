package app

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This exercises the generic factory through actual owning composition. The
// injected provider checks every inference entry and delegates only planning to
// the deterministic proposal script; it makes no provider/network inference.
func TestScenarioReleaseChainGenericProviderCurrentOwnershipContainerE2E(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_RELEASE_CHAIN_E2E") != "1" {
		t.Skip("explicit isolated whole chain required")
	}
	var f *developerFixture
	var checker *releaseChainEntryChecker
	config := scriptedReleaseChainModelConfiguration()
	config.Factory = func(ctx context.Context, pool *pgxpool.Pool, root string) (transport.ModelProvider, error) {
		args := f.args()
		args.Budget = config.ChildBudget
		planner := &converterWorkerProvider{need: f.needRef, spec: f.specRef, document: f.document, args: args, admissionInput: f.admissionInput, prepareProject: true, states: map[string]*converterWorkerState{}}
		checker = &releaseChainEntryChecker{ModelProvider: planner, pool: pool, sources: f.f.sources, root: root, document: f.document, need: f.needRef, spec: f.specRef}
		return checker, nil
	}
	hooks := &converterWorkerFaultHooks{factory: func(t *testing.T, p generatedtools.Profile, path string) *developerFixture {
		f = newReleaseChainFixtureWithModel(t, p, path, &config)
		return f
	}, after: func(t *testing.T, f *developerFixture, p *converterWorkerProvider) {
		if checker == nil || checker.starts.Load() != 4 || checker.submits.Load() != 13 || checker.continues.Load() != 0 || checker.parentStarts.Load() != 3 || checker.childStarts.Load() != 1 {
			t.Fatal("generic provider routing missed owning parent/child entries")
		}
		verifyReleaseChainAndDeliver(t, f, p, "generic_provider_preflight")
		t.Log("generic_factory_entries=17 current_actual_lease=true owner_actor_absent=true parent_starts=3 child_starts=1 starts=4 submits=13 continues=0 exact_original_need_spec_pins=true actual_native_calls=0")
	}}
	testConverterDeveloperWorkersObserved(t, true, hooks)
}

func scriptedReleaseChainModelConfiguration() releaseChainModelConfig {
	policy := budget.DefaultPolicy()
	policy.Models["ollama:converter-worker-fixture"] = budget.ModelPrice{Basis: "local_no_billing"}
	policy.Task.ModelTurns, policy.Task.Tokens, policy.Task.ActiveMillis = 32, 500000, 1200000
	policy.Mandate.ModelTurns, policy.Mandate.Tokens, policy.Mandate.ActiveMillis = 64, 1000000, 2400000
	policy.Day.ModelTurns, policy.Day.Tokens, policy.Day.ActiveMillis = 64, 1000000, 2400000
	policy.Conversation = policy.Task
	policy.ModelMillis, policy.ToolMillis, policy.OutputTokens, policy.ModelInputBytes, policy.TaskLifetimeMillis = 20000, 30000, 128, 131072, 1800000
	return releaseChainModelConfig{Provider: "ollama", Model: "converter-worker-fixture", InputTokens: 8192, OutputTokens: 128, Policy: policy, ChildBudget: budget.Units{ModelTurns: 4, Tokens: 65536, ExternalCalls: 4, ActiveMillis: 600000, EgressBytes: 16 << 20, ProviderInputBytes: 16 << 20}}
}

type releaseChainStartFailureProvider struct {
	*converterWorkerProvider
	entries atomic.Int32
}

func (p *releaseChainStartFailureProvider) StartRun(context.Context, transport.StartRunRequest) (transport.EventStream, error) {
	p.entries.Add(1)
	return nil, errors.New("synthetic provider start failure")
}

func TestScenarioReleaseChainProviderStartFailureRetainsAdmissionContainerE2E(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_RELEASE_CHAIN_E2E") != "1" {
		t.Skip("explicit isolated whole chain required")
	}
	var f *developerFixture
	var failed *releaseChainStartFailureProvider
	config := scriptedReleaseChainModelConfiguration()
	config.Factory = func(ctx context.Context, pool *pgxpool.Pool, root string) (transport.ModelProvider, error) {
		failed = &releaseChainStartFailureProvider{converterWorkerProvider: &converterWorkerProvider{states: map[string]*converterWorkerState{}}}
		return &releaseChainEntryChecker{ModelProvider: failed, pool: pool, sources: f.f.sources, root: root, document: f.document, need: f.needRef, spec: f.specRef}, nil
	}
	hooks := &converterWorkerFaultHooks{factory: func(t *testing.T, p generatedtools.Profile, path string) *developerFixture {
		f = newReleaseChainFixtureWithModel(t, p, path, &config)
		return f
	}, terminal: func(t *testing.T, f *developerFixture, p *converterWorkerProvider, drain func()) {
		waitConverterWorker(t, func() bool {
			var status string
			e := f.f.pool.QueryRow(f.f.ctx, `SELECT status FROM accepted_inputs WHERE input_id=$1`, f.initialInputID).Scan(&status)
			return e == nil && status == "blocked"
		}, "actual failed provider entry receipt")
		drain()
		var attempts, unknown, used, notices, actions int
		e := f.f.pool.QueryRow(f.f.ctx, `SELECT
 (SELECT count(*) FROM execution_model_attempts WHERE run_id IN(SELECT run_id FROM runs WHERE task_id=$1)),
 (SELECT count(*) FROM execution_model_attempts WHERE run_id IN(SELECT run_id FROM runs WHERE task_id=$1) AND actual_input_tokens IS NULL AND actual_output_tokens IS NULL),
 (SELECT (used_json->>'model_turns')::int FROM execution_budget_scopes WHERE kind='task' AND subject_id=$1),
 (SELECT count(*) FROM task_verified_notifications),(SELECT count(*) FROM action_intents WHERE run_id IN(SELECT run_id FROM runs WHERE task_id=$1))`, f.f.parent.TaskID).Scan(&attempts, &unknown, &used, &notices, &actions)
		if e != nil || failed == nil || failed.entries.Load() != 1 || attempts != 1 || unknown != 1 || used != 1 || notices != 0 || actions != 0 {
			t.Fatal("failed entry lost/shared/reset model admission", e, attempts, unknown, used, notices, actions)
		}
		t.Log("provider_start_failure=retained actual_entries=1 actual_model_admissions=1 unknown_usage=1 root_model_turns=1 tool_effects=0 success_notices=0 native_calls=0 no_retry=true")
	}}
	testConverterDeveloperWorkersObserved(t, true, hooks)
}

type releaseChainEntryChecker struct {
	transport.ModelProvider
	pool                                                  *pgxpool.Pool
	sources                                               *catalog.Store
	root                                                  string
	document, need, spec                                  catalog.SourceRef
	starts, submits, continues, parentStarts, childStarts atomic.Int32
}

func (p *releaseChainEntryChecker) InferenceLimits() (int64, int64) { return 8192, 128 }

func (p *releaseChainEntryChecker) guard(ctx context.Context, runID string) (bool, error) {
	if _, ok := owneridentity.FromContext(ctx); ok {
		return false, errors.New("provider received owner authority")
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return false, errors.New("provider current execution proof absent")
	}
	tx, e := p.pool.Begin(ctx)
	if e != nil {
		return false, e
	}
	defer tx.Rollback(ctx)
	parent, e := p.guardTx(ctx, tx, runID)
	if e != nil {
		return false, e
	}
	return parent, tx.Commit(ctx)
}

// The caller already owns this transaction. The native fixture also uses this
// read proof before and after its finite counter CAS, without borrowing a pool.
func (p *releaseChainEntryChecker) guardTx(ctx context.Context, tx pgx.Tx, runID string) (bool, error) {
	if _, ok := owneridentity.FromContext(ctx); ok {
		return false, errors.New("provider received owner authority")
	}
	token, ok := ownership.TokenFromContext(ctx)
	if !ok || token.RunID != runID {
		return false, errors.New("provider current execution proof absent")
	}
	var parent bool
	var taskID string
	e := tx.QueryRow(ctx, `SELECT t.task_id=$5,t.task_id FROM runs r JOIN tasks t ON t.task_id=r.task_id
 JOIN session_ownership o ON o.session_key=r.session_key
 WHERE r.run_id=$1 AND r.session_key=$2 AND r.lease_id=$3 AND o.run_id=r.run_id AND o.lease_id=$3 AND o.fence_epoch=$4
 AND o.expires_at>clock_timestamp()
 AND t.status='active' AND t.revision=r.task_revision AND (t.task_id=$5 OR (t.parent_task_id=$5
 AND EXISTS(SELECT 1 FROM converter_developer_snapshots d JOIN task_delegations c ON c.task_id=d.task_id WHERE d.task_id=t.task_id AND d.parent_task_id=$5 AND c.state IN('accepted','executing'))))
 AND r.current_state IN('model_running','awaiting_model_resume') FOR SHARE OF r,t,o`, runID, token.SessionKey, token.LeaseID, token.FenceEpoch, p.root).Scan(&parent, &taskID)
	if e != nil {
		return false, errors.New("provider current owning lineage denied")
	}
	if e = p.sources.CheckRunSourcesTx(ctx, tx, runID); e != nil {
		return false, errors.New("provider current original source denied")
	}
	if e = delegation.CheckTaskTx(ctx, tx, taskID); e != nil {
		return false, errors.New("provider current owning delegation denied")
	}
	if parent {
		var pins int
		e = tx.QueryRow(ctx, `SELECT count(*) FROM memory_run_sources WHERE run_id=$1 AND
 ((source_id=$2 AND source_revision=$3 AND access_revision=$4 AND content_hash=$5)
 OR (source_id=$6 AND source_revision=$7 AND access_revision=$8 AND content_hash=$9)
 OR (source_id=$10 AND source_revision=$11 AND access_revision=$12 AND content_hash=$13))`, runID,
			p.document.ID, p.document.Revision, p.document.AccessRevision, p.document.Hash, p.need.ID, p.need.Revision, p.need.AccessRevision, p.need.Hash, p.spec.ID, p.spec.Revision, p.spec.AccessRevision, p.spec.Hash).Scan(&pins)
		if e != nil || pins != 3 {
			return false, errors.New("provider original Need/spec/document pins missing")
		}
	}
	var live bool
	if e = tx.QueryRow(ctx, `SELECT TRUE FROM session_ownership WHERE session_key=$1 AND run_id=$2 AND lease_id=$3 AND fence_epoch=$4 AND expires_at>clock_timestamp()`, token.SessionKey, runID, token.LeaseID, token.FenceEpoch).Scan(&live); e != nil {
		return false, errors.New("provider lease expired after source wait")
	}
	return parent, nil
}
func (p *releaseChainEntryChecker) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	parent, e := p.guard(ctx, q.Context.RunID)
	if e != nil {
		return nil, e
	}
	p.starts.Add(1)
	if parent {
		p.parentStarts.Add(1)
	} else {
		p.childStarts.Add(1)
	}
	return p.ModelProvider.StartRun(ctx, q)
}
func (p *releaseChainEntryChecker) ContinueRun(ctx context.Context, q transport.ContinueRunRequest) (transport.EventStream, error) {
	if _, e := p.guard(ctx, q.RunID); e != nil {
		return nil, e
	}
	p.continues.Add(1)
	return p.ModelProvider.ContinueRun(ctx, q)
}
func (p *releaseChainEntryChecker) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if _, e := p.guard(ctx, q.RunID); e != nil {
		return nil, e
	}
	p.submits.Add(1)
	return p.ModelProvider.SubmitToolResult(ctx, q)
}
