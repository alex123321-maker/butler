package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
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

const releaseChainV5ToolPlanningPolicy = "Use exactly one structured tool call per response. Wait for its correlated result before choosing the next call. This Butler Core does not support tool batches."

func releaseChainV5BasePrompt() string {
	return prompt.DefaultBasePrompt + "\n\n" + releaseChainV5ToolPlanningPolicy
}

func releaseChainV5Configuration() releaseChainModelConfig {
	p := budget.DefaultPolicy()
	p.Models[antigravity.Name+":"+antigravity.FlashHighModel] = budget.ModelPrice{Basis: "subscription_no_overage"}
	quote := int64(1048576 + 32768)
	p.Task.ModelTurns, p.Task.Tokens, p.Task.ActiveMillis = 24, 24*quote, 24*120000
	p.Mandate.ModelTurns, p.Mandate.Tokens, p.Mandate.ActiveMillis = 40, 40*quote, 40*120000
	p.Day = p.Mandate
	p.Day.ModelTurns, p.Day.Tokens, p.Day.ActiveMillis = releaseChainV5NewCap, releaseChainV5NewCap*quote, releaseChainV5NewCap*120000
	p.Conversation = p.Task
	p.OutputTokens, p.ModelMillis, p.ToolMillis, p.ModelInputBytes, p.TaskLifetimeMillis = 32768, 120000, 30000, 131072, 1200000
	return releaseChainModelConfig{Provider: antigravity.Name, Model: antigravity.FlashHighModel, InputTokens: 1048576, OutputTokens: 32768, Policy: p, FixtureTimeout: 20 * time.Minute, ChildBudget: budget.Units{ModelTurns: 4, Tokens: 4 * quote, ExternalCalls: 4, ActiveMillis: 600000, EgressBytes: 16 << 20, ProviderInputBytes: 16 << 20}}
}

func releaseChainHighConfigurationFor(policy releaseChainHighWindowPolicy) releaseChainModelConfig {
	c := releaseChainV5Configuration()
	if policy == (releaseChainHighWindowPolicy{}) {
		return c
	}
	c.Policy.Day.ModelTurns = 76
	c.Policy.Day.Tokens = 76 * int64(1048576+32768)
	c.ChildBudget.ActiveMillis = 4 * (int64(policy.attemptMillis()) + c.Policy.ToolMillis)
	ms := int64(policy.attemptMillis())
	c.Policy.ModelMillis = ms
	c.Policy.Task.ModelTurns = 24
	c.Policy.Task.ActiveMillis = 24 * ms
	c.Policy.Mandate.ActiveMillis = 40 * ms
	c.Policy.Day.ActiveMillis = 76 * ms
	c.Policy.Conversation = c.Policy.Task
	c.Policy.TaskLifetimeMillis = int64(policy.caseMillis())
	c.FixtureTimeout = time.Duration(policy.caseMillis()) * time.Millisecond
	return c
}

func newReleaseChainV5NativeProvider(ctx context.Context, pool *pgxpool.Pool, importPath, keyPath string) (transport.ModelProvider, error) {
	return newReleaseChainHighNativeProviderFor(ctx, pool, importPath, keyPath, releaseChainHighWindowPolicy{})
}

func newReleaseChainHighNativeProviderFor(ctx context.Context, pool *pgxpool.Pool, importPath, keyPath string, policy releaseChainHighWindowPolicy) (transport.ModelProvider, error) {
	if !policy.valid() {
		return nil, errNativeFixtureDenied
	}
	if _, err := readReleaseChainV5Prerequisite(); err != nil {
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
	return antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, Timeout: time.Duration(policy.attemptMillis()) * time.Millisecond, AuthSource: manager, MaximumOutputTokens: policy.generation()})
}

// Faults observe actual owning PG admission. They do not read model prose or
// script a final/result. A child waits before inference until the real parent
// admission receipt has caused the reviewed restart or current owner Stop.
type releaseChainV5Fault struct {
	f               *developerFixture
	scenario        string
	gate            chan struct{}
	once            sync.Once
	err             error
	oldChild        ownership.Token
	triggered       chan struct{}
	oldSourceDenied bool
	oldBrokerDenied bool
}

func (h *releaseChainV5Fault) before(ctx context.Context, kind, run string) error {
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
	if kind != "submit" || h.scenario != "restart_sent" && h.scenario != "owner_stop" {
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

func testReleaseChainHighNative(t *testing.T, entry releaseChainHighEntry) {
	if os.Getenv(entry.AcceptanceEnv) != "1" {
		t.Skip("separately frozen actual High scope required")
	}
	if _, e := entry.ReadQuality(); e != nil {
		t.Fatal("independent V5 PASS prerequisite unavailable")
	}
	// This declared scenario policy enters through ordinary Core's operator
	// PromptManager, not a transport wrapper or a hidden provider instruction.
	t.Setenv(prompt.SettingKeyBasePrompt, releaseChainV5BasePrompt())
	t.Setenv(prompt.SettingKeyBasePromptEnabled, "true")
	private := os.Getenv(entry.PrivateEnv)
	if private != "/private" {
		t.Fatal("fixed private artifact mount required")
	}
	results := map[string]bool{}
	var snapshot json.RawMessage
	defer func() {
		status := "failed"
		if len(results) == 5 && results["normal_sent"] && results["restart_sent"] && results["explicit_absence"] && results["lost_ack"] && results["owner_stop"] && !t.Failed() {
			status = "passed"
		}
		body, _ := json.Marshal(map[string]any{"status": status, "window_id": entry.WindowID, "baseline_attempts": entry.Baseline, "maximum_total_attempts": entry.Baseline + entry.Maximum, "maximum_new_attempts": entry.Maximum, "scenarios": results, "scope": snapshot, "planning": "actual_native_High", "human_messages": 0, "issue33_ready": false})
		if len(body) > releaseChainV5SafeArtifactOutputMax {
			t.Error("bounded safe scope artifact unavailable")
			return
		}
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
	for _, scenario := range releaseChainV5Scenarios() {
		t.Run(scenario, func(t *testing.T) {
			var f *developerFixture
			var fault *releaseChainV5Fault
			var caseWindow *releaseChainV5Window
			cfg := releaseChainHighConfigurationFor(entry.Policy)
			cfg.Factory = func(ctx context.Context, pool *pgxpool.Pool, root string) (transport.ModelProvider, error) {
				prerequisite, e := entry.ReadQuality()
				if e != nil {
					return nil, e
				}
				w, e := entry.NewWindow(ctx, pool)
				if e != nil {
					return nil, e
				}
				if w.policy != entry.Policy {
					return nil, errNativeFixtureDenied
				}
				caseWindow = w
				if e := recordReleaseChainV5InitialEvidence(ctx, w, f, scenario); e != nil {
					return nil, e
				}
				native, e := newReleaseChainHighNativeProviderFor(ctx, pool, os.Getenv(entry.ImportEnv), filepath.Join(private, "encrypted-settings-key"), entry.Policy)
				if e != nil {
					return nil, e
				}
				fault = &releaseChainV5Fault{f: f, scenario: scenario, triggered: make(chan struct{})}
				if scenario == "restart_sent" || scenario == "owner_stop" {
					fault.gate = make(chan struct{})
				}
				return &releaseChainV5Provider{ModelProvider: native, window: w, scenario: scenario, expectedVersion: prerequisite.ObservedVersion, checker: &releaseChainEntryChecker{pool: pool, sources: f.f.sources, root: root, document: f.document, need: f.needRef, spec: f.specRef}, before: fault.before}, nil
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
				} else if scenario == "explicit_absence" {
					mode = "explicit_absence"
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
					fault.oldSourceDenied = true
					raw, _ := json.Marshal(map[string]any{"source_ref": f.needRef})
					if _, e := f.existingClient.PrepareAction(old, &toolbrokerv1.ToolCall{RunId: fault.oldChild.RunID, ToolCallId: "native-stopped-child-prepare", ToolName: "converters.discover", ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}); e == nil {
						t.Fatal("actual Broker accepted stopped native child")
					}
					fault.oldBrokerDenied = true
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
					if e := recordReleaseChainV5ProductEvidence(context.Background(), caseWindow, f, scenario, t.Failed(), fault); e != nil {
						results[scenario] = false
						t.Error("content-free owning product witness unavailable")
					}
					var attempts, unknown, failed, overlong, unknownCost, unknownVersion int
					var window json.RawMessage
					e := f.f.pool.QueryRow(context.Background(), `SELECT (SELECT attempts FROM `+caseWindow.table("fixture_chain_v5_high_window")+`),(SELECT count(*) FROM `+caseWindow.table("fixture_chain_v5_high_requests")+` WHERE input_tokens IS NULL OR output_tokens IS NULL OR total_tokens IS NULL OR model_version IS NULL),(SELECT count(*) FROM `+caseWindow.table("fixture_chain_v5_high_requests")+` WHERE outcome<>'completed' OR provider_operation_stage IS DISTINCT FROM 'response_stream'),(SELECT count(*) FROM `+caseWindow.table("fixture_chain_v5_high_requests")+` WHERE elapsed_ms IS NULL OR elapsed_ms>`+strconv.Itoa(caseWindow.policy.attemptMillis())+`),(SELECT count(*) FROM `+caseWindow.table("fixture_chain_v5_high_requests")+` WHERE input_tokens IS NULL OR output_tokens IS NULL OR total_tokens IS NULL),(SELECT count(*) FROM `+caseWindow.table("fixture_chain_v5_high_requests")+` WHERE model_version IS NULL),(SELECT `+caseWindow.metadataSQL()+` FROM `+caseWindow.table("fixture_chain_v5_high_window")+`)`).Scan(&attempts, &unknown, &failed, &overlong, &unknownCost, &unknownVersion, &window)
					if e != nil {
						t.Error("durable native scope unavailable")
						return
					}
					rows, e := f.f.pool.Query(context.Background(), `SELECT ordinal,scenario,kind,root_task_id,run_id,is_parent,input_hash,admitted_at,outcome,input_tokens,output_tokens,total_tokens,model_version,finished_at,elapsed_ms,failure_stage,diagnostic_code,final_count,terminal_count,completed_count,cancelled_count,timed_out_count,transport_error_count,finish_reason,usage_observation_uncertain,version_observation_uncertain,stop_final_count,max_tokens_final_count,finish_observation_uncertain,provider_operation_stage,kernel_operation_id FROM `+caseWindow.table("fixture_chain_v5_high_requests")+` ORDER BY ordinal`)
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
						var finals, terminals, completed, cancelled, timedOut, transportErrors *int
						var finishReason *string
						var usageUncertain, versionUncertain, finishUncertain *bool
						var stopFinals, maxTokensFinals *int
						var providerStage, kernelOperation *string
						if e = rows.Scan(&ordinal, &scenario, &kind, &root, &run, &parent, &hash, &at, &outcome, &input, &output, &total, &version, &finished, &elapsed, &stage, &code, &finals, &terminals, &completed, &cancelled, &timedOut, &transportErrors, &finishReason, &usageUncertain, &versionUncertain, &stopFinals, &maxTokensFinals, &finishUncertain, &providerStage, &kernelOperation); e != nil {
							break
						}
						items = append(items, map[string]any{"ordinal": ordinal, "scenario": scenario, "kind": kind, "root_task_id": root, "run_id": run, "is_parent": parent, "input_sha256": hash, "admitted_at_utc": at, "finished_at_utc": finished, "elapsed_ms": elapsed, "outcome": outcome, "failure_stage": stage, "diagnostic_code": code, "actual_input_tokens": input, "actual_output_tokens": output, "actual_total_tokens": total, "model_version": version, "final_count": finals, "terminal_count": terminals, "completed_count": completed, "cancelled_count": cancelled, "timed_out_count": timedOut, "transport_error_count": transportErrors, "finish_reason": finishReason, "usage_observation_uncertain": usageUncertain, "version_observation_uncertain": versionUncertain, "stop_final_count": stopFinals, "max_tokens_final_count": maxTokensFinals, "finish_observation_uncertain": finishUncertain, "provider_operation_stage": providerStage, "kernel_operation_id": kernelOperation})
					}
					if e != nil || rows.Err() != nil {
						rows.Close()
						t.Error("native request evidence incomplete")
						return
					}
					rows.Close()
					acceptedRetries := 0
					var retryMeasurements releaseChainV6RetryMeasurements
					var retryError error
					if caseWindow.kernelOperations {
						retryMeasurements, retryError = releaseChainV6AcceptedRetryMeasurements(context.Background(), caseWindow)
						retryMeasurements = releaseChainHighAcceptedMeasurements(retryMeasurements, retryError)
						acceptedRetries = retryMeasurements.Failures
					}
					scopeSnapshot := map[string]any{"window": window, "total_attempts": attempts, "new_attempts": attempts - entry.Baseline, "unknown_full_token_cost": unknown, "failed_provider_entries": failed, "accepted_model_retry_failures": acceptedRetries, "overlong_provider_entries": overlong, "requests": items}
					if caseWindow.kernelOperations {
						scopeSnapshot["unknown_token_cost_entries"] = unknownCost
						scopeSnapshot["unknown_model_version_entries"] = unknownVersion
						scopeSnapshot["accepted_unknown_timeout_cost_entries"] = retryMeasurements.UnknownCost
						scopeSnapshot["accepted_unknown_timeout_version_entries"] = retryMeasurements.UnknownVersion
						scopeSnapshot["accepted_unmeasured_timeout_entries"] = retryMeasurements.UnknownMeasurement
					}
					snapshot, _ = json.Marshal(scopeSnapshot)
					if unknown != retryMeasurements.UnknownMeasurement || failed != acceptedRetries || overlong != 0 || retryError != nil {
						results[scenario] = false
						t.Error("complete observed cost/version/provider outcomes or request deadline unavailable")
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
