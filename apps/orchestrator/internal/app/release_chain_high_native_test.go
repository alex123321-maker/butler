package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

func chainHighConfiguration() releaseChainModelConfig {
	p := budget.DefaultPolicy()
	p.Models[antigravity.Name+":"+antigravity.FlashHighModel] = budget.ModelPrice{Basis: "subscription_no_overage"}
	quote := int64(1048576 + 32768)
	p.Task.ModelTurns, p.Task.Tokens, p.Task.ActiveMillis = 24, 24*quote, 24*120000
	p.Mandate.ModelTurns, p.Mandate.Tokens, p.Mandate.ActiveMillis = 40, 40*quote, 40*120000
	p.Day = p.Mandate
	p.Conversation = p.Task
	p.OutputTokens, p.ModelMillis, p.ToolMillis, p.ModelInputBytes, p.TaskLifetimeMillis = 32768, 120000, 30000, 131072, 1200000
	return releaseChainModelConfig{Provider: antigravity.Name, Model: antigravity.FlashHighModel, InputTokens: 1048576, OutputTokens: 32768, Policy: p, FixtureTimeout: 20 * time.Minute, ChildBudget: budget.Units{ModelTurns: 4, Tokens: 4 * quote, ExternalCalls: 4, ActiveMillis: 600000, EgressBytes: 16 << 20, ProviderInputBytes: 16 << 20}}
}

func newChainHighNativeProvider(ctx context.Context, pool *pgxpool.Pool, importPath, keyPath string) (transport.ModelProvider, error) {
	if _, err := readChainHighPrerequisite(); err != nil {
		return nil, errNativeFixtureDenied
	}
	key, err := privateNativeFixtureKey(keyPath)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	store := config.NewPostgresSettingsStore(pool, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: key}))
	manager := providerauth.NewManager(store)
	state, err := manager.State(ctx, modelprovider.ProviderAntigravity)
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	if !state.Connected {
		file, err := privateNativeFixtureFile(importPath, 64*1024)
		if err != nil {
			return nil, errNativeFixtureDenied
		}
		err = manager.ImportAntigravity(ctx, file, "isolated-issue33-native-release-chain")
		_ = file.Close()
		if err != nil {
			return nil, errNativeFixtureDenied
		}
	}
	return antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, Timeout: 120 * time.Second, AuthSource: manager, MaximumOutputTokens: 32768})
}

// Faults observe actual owning PG admission. They do not read model prose or
// script a final/result. A child waits before inference until the real parent
// admission receipt has caused the reviewed restart or current owner Stop.
type chainHighFault struct {
	f         *developerFixture
	scenario  string
	gate      chan struct{}
	once      sync.Once
	err       error
	oldChild  ownership.Token
	triggered chan struct{}
}

func (h *chainHighFault) before(ctx context.Context, kind, run string) error {
	var parent bool
	if h.f.f.pool.QueryRow(ctx, `SELECT task_id=$2 FROM runs WHERE run_id=$1`, run, h.f.f.parent.TaskID).Scan(&parent) != nil {
		return errNativeFixtureDenied
	}
	if !parent && h.gate != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-h.gate:
		}
		return h.err
	}
	if kind != "submit" || h.scenario == "lost_ack" {
		return nil
	}
	var admitted bool
	if h.f.f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_delegations d JOIN converter_developer_snapshots s ON s.task_id=d.task_id WHERE d.parent_task_id=$1 AND s.parent_task_id=$1 AND d.state IN('queued','executing','result_ready'))`, h.f.f.parent.TaskID).Scan(&admitted) != nil {
		return errNativeFixtureDenied
	}
	if !admitted {
		return nil
	}
	h.once.Do(func() {
		defer close(h.triggered)
		defer close(h.gate)
		if h.scenario == "restart_sent" {
			h.err = h.f.restartBroker()
			return
		}
		until := time.Now().Add(15 * time.Second)
		for time.Now().Before(until) {
			e := h.f.f.pool.QueryRow(ctx, `SELECT r.session_key,r.run_id,o.lease_id,o.fence_epoch FROM runs r JOIN tasks t ON t.task_id=r.task_id JOIN session_ownership o ON o.session_key=r.session_key AND o.run_id=r.run_id WHERE t.parent_task_id=$1 AND r.current_state='model_running' AND o.expires_at>clock_timestamp()`, h.f.f.parent.TaskID).Scan(&h.oldChild.SessionKey, &h.oldChild.RunID, &h.oldChild.LeaseID, &h.oldChild.FenceEpoch)
			if e == nil {
				break
			}
			select {
			case <-ctx.Done():
				h.err = ctx.Err()
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
		if h.oldChild.RunID == "" {
			h.err = errors.New("live actual child lease not observed before Stop")
			return
		}
		actor := owneridentity.WithActor(h.f.f.ctx, owneridentity.Actor{OwnerID: h.f.f.parent.OwnerID, SessionKey: h.f.f.parent.SessionKey, Channel: "web", DeliveryID: "native-Stop-once"})
		req := task.ControlRequest{TaskID: h.f.f.parent.TaskID, ExpectedRevision: h.f.f.parent.Revision, Kind: task.ControlStop}
		first, e := releaseChainController(h.f).Apply(actor, req)
		if e != nil {
			h.err = e
			return
		}
		second, e := releaseChainController(h.f).Apply(actor, req)
		if e != nil || second.ControlID != first.ControlID {
			h.err = errNativeFixtureDenied
		}
	})
	return h.err
}

func TestScenarioReleaseChainActualHighE2E(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_CHAIN_HIGH_ACCEPTANCE") != "1" {
		t.Skip("separately frozen actual High scope required")
	}
	if _, e := readChainHighPrerequisite(); e != nil {
		t.Fatal("independent V2 PASS prerequisite unavailable")
	}
	private := os.Getenv("BUTLER_SCENARIO_CHAIN_HIGH_PRIVATE")
	if private != "/private" {
		t.Fatal("fixed private artifact mount required")
	}
	results := map[string]bool{}
	var snapshot json.RawMessage
	defer func() {
		status := "failed"
		if len(results) == 3 && results["restart_sent"] && results["lost_ack"] && results["owner_stop"] && !t.Failed() {
			status = "passed"
		}
		body, _ := json.Marshal(map[string]any{"status": status, "window_id": chainHighWindowID, "baseline_attempts": chainHighBaseline, "maximum_total_attempts": chainHighCeiling, "maximum_new_attempts": chainHighNewCap, "scenarios": results, "scope": snapshot, "planning": "actual_native_High", "human_messages": 0, "issue33_ready": false})
		file, e := os.OpenFile(filepath.Join(private, "scope.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			t.Error("private native scope artifact unavailable")
			return
		}
		defer file.Close()
		if _, e = file.Write(body); e != nil {
			t.Error("native scope artifact not persisted")
		}
		if e = file.Sync(); e != nil {
			t.Error("native scope artifact not synced")
		}
	}()
	for _, scenario := range []string{"restart_sent", "lost_ack", "owner_stop"} {
		t.Run(scenario, func(t *testing.T) {
			var f *developerFixture
			var fault *chainHighFault
			var caseWindow *chainHighWindow
			cfg := chainHighConfiguration()
			cfg.Factory = func(ctx context.Context, pool *pgxpool.Pool, root string) (transport.ModelProvider, error) {
				w, e := newChainHighWindow(ctx, pool)
				if e != nil {
					return nil, e
				}
				caseWindow = w
				native, e := newChainHighNativeProvider(ctx, pool, os.Getenv("BUTLER_SCENARIO_CHAIN_HIGH_IMPORT_FILE"), filepath.Join(private, "encrypted-settings-key"))
				if e != nil {
					return nil, e
				}
				fault = &chainHighFault{f: f, scenario: scenario, triggered: make(chan struct{})}
				if scenario != "lost_ack" {
					fault.gate = make(chan struct{})
				}
				return &chainHighProvider{ModelProvider: native, window: w, scenario: scenario, checker: &releaseChainEntryChecker{pool: pool, sources: f.f.sources, root: root, document: f.document, need: f.needRef, spec: f.specRef}, before: fault.before}, nil
			}
			hooks := &converterWorkerFaultHooks{factory: func(t *testing.T, p generatedtools.Profile, path string) *developerFixture {
				f = newReleaseChainFixtureWithModel(t, p, path, &cfg)
				return f
			}, after: func(t *testing.T, f *developerFixture, p *converterWorkerProvider) {
				if scenario == "restart_sent" {
					select {
					case <-fault.triggered:
					default:
						t.Fatal("actual Broker restart absent")
					}
					if fault.err != nil {
						t.Fatal("actual Broker restart failed")
					}
				}
				mode := "baseline"
				if scenario == "lost_ack" {
					mode = "ack_commit_failure"
				}
				verifyReleaseChainAndDeliver(t, f, p, mode)
				results[scenario] = true
			}}
			if scenario == "owner_stop" {
				hooks.terminal = func(t *testing.T, f *developerFixture, p *converterWorkerProvider, drain func()) {
					select {
					case <-fault.triggered:
					case <-time.After(5 * time.Minute):
						t.Fatal("native owner Stop admission not reached")
					}
					if fault.err != nil {
						t.Fatal("native Stop current owner transition failed")
					}
					old := ownership.WithToken(f.f.ctx, nil, fault.oldChild)
					check := &releaseChainEntryChecker{pool: f.f.pool, sources: f.f.sources, root: f.f.parent.TaskID, document: f.document, need: f.needRef, spec: f.specRef}
					if _, e := check.guard(old, fault.oldChild.RunID); e == nil {
						t.Fatal("old child proof survived native Stop")
					}
					raw, _ := json.Marshal(map[string]any{"source_ref": f.needRef})
					if _, e := f.existingClient.PrepareAction(old, &toolbrokerv1.ToolCall{RunId: fault.oldChild.RunID, ToolCallId: "native-stopped-child-prepare", ToolName: "converters.discover", ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}); e == nil {
						t.Fatal("actual Broker accepted stopped native child")
					}
					drain()
					var cancelled, actions, notices, live, retained, finals int
					e := f.f.pool.QueryRow(f.f.ctx, `SELECT (SELECT count(*) FROM tasks WHERE (task_id=$1 OR parent_task_id=$1) AND status='cancelled'),(SELECT count(*) FROM action_intents WHERE run_id IN(SELECT r.run_id FROM runs r JOIN tasks c ON c.task_id=r.task_id WHERE c.parent_task_id=$1)),(SELECT count(*) FROM task_verified_notifications),(SELECT count(*) FROM session_ownership WHERE lease_id=$2 AND expires_at>clock_timestamp()),(SELECT count(*) FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id JOIN runs r ON r.run_id=i.run_id WHERE r.task_id=$1 AND e.state='effect_confirmed'),(SELECT count(*) FROM run_final_results WHERE task_id=$1)`, f.f.parent.TaskID, fault.oldChild.LeaseID).Scan(&cancelled, &actions, &notices, &live, &retained, &finals)
					if e != nil || cancelled != 2 || actions != 0 || notices != 0 || live != 0 || retained != 2 || finals != 0 {
						t.Fatal("native Stop failed current tree/drain/no effect invariant")
					}
					results[scenario] = true
				}
			}
			defer func() {
				if caseWindow != nil {
					var attempts, unknown, failed int
					e := f.f.pool.QueryRow(context.Background(), `SELECT (SELECT attempts FROM `+caseWindow.table("fixture_chain_high_window")+`),(SELECT count(*) FROM `+caseWindow.table("fixture_chain_high_requests")+` WHERE total_tokens IS NULL OR model_version IS NULL),(SELECT count(*) FROM `+caseWindow.table("fixture_chain_high_requests")+` WHERE outcome<>'completed')`).Scan(&attempts, &unknown, &failed)
					if e != nil {
						t.Error("durable native scope unavailable")
						return
					}
					rows, e := f.f.pool.Query(context.Background(), `SELECT ordinal,scenario,kind,root_task_id,run_id,is_parent,input_hash,admitted_at,outcome,input_tokens,output_tokens,total_tokens,model_version,finished_at,elapsed_ms,failure_stage,diagnostic_code FROM `+caseWindow.table("fixture_chain_high_requests")+` ORDER BY ordinal`)
					if e != nil {
						t.Error("safe native request evidence unavailable")
						return
					}
					items := []map[string]any{}
					for rows.Next() {
						var ordinal int
						var scenario, kind, root, run, hash, outcome string
						var parent bool
						var at time.Time
						var input, output, total *int64
						var version *string
						var finished *time.Time
						var elapsed *int64
						var stage, code *string
						if e = rows.Scan(&ordinal, &scenario, &kind, &root, &run, &parent, &hash, &at, &outcome, &input, &output, &total, &version, &finished, &elapsed, &stage, &code); e != nil {
							break
						}
						items = append(items, map[string]any{"ordinal": ordinal, "scenario": scenario, "kind": kind, "root_task_id": root, "run_id": run, "is_parent": parent, "input_sha256": hash, "admitted_at_utc": at, "finished_at_utc": finished, "elapsed_ms": elapsed, "outcome": outcome, "failure_stage": stage, "diagnostic_code": code, "actual_input_tokens": input, "actual_output_tokens": output, "actual_total_tokens": total, "model_version": version})
					}
					if e != nil || rows.Err() != nil {
						rows.Close()
						t.Error("native request evidence incomplete")
						return
					}
					rows.Close()
					snapshot, _ = json.Marshal(map[string]any{"total_attempts": attempts, "new_attempts": attempts - chainHighBaseline, "unknown_full_token_cost": unknown, "failed_provider_entries": failed, "requests": items})
					if unknown != 0 || failed != 0 {
						results[scenario] = false
						t.Error("complete observed cost/version/provider outcomes unavailable")
					}
				}
			}()
			testConverterDeveloperWorkersObserved(t, true, hooks)
		})
		if t.Failed() {
			break
		}
	}
}
