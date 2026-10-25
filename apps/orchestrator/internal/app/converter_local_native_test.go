package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A new narrow converter-only acceptance scope. It cannot open, reset or qualify
// any sealed issue33 release-chain window or its retained UNKNOWN expenses.
const converterLocalNativeWindow = "issue20-converter-local-native-20261025e"
const converterLocalNativeCap = 40
const converterLocalNativePriorWindow = "issue20-converter-local-native-20261025a"
const converterLocalNativePriorCalls = 8
const converterLocalNativePriorTokens = 82953
const converterLocalNativePriorBWindow = "issue20-converter-local-native-20261025b"
const converterLocalNativePriorBCalls = 11
const converterLocalNativePriorBTokens = 116694
const converterLocalNativePriorCWindow = "issue20-converter-local-native-20261025c"
const converterLocalNativePriorCCalls = 12
const converterLocalNativePriorCTokens = 119510 // Lower bound: cancelled entry12 has NULL usage.
const converterLocalNativePriorDWindow = "issue20-converter-local-native-20261025d"
const converterLocalNativePriorDCalls = 15
const converterLocalNativePriorDTokens = 186253

var errConverterLocalNative = errors.New("local converter native boundary unavailable")

func converterLocalNativeConfig() releaseChainModelConfig {
	p := budget.DefaultPolicy()
	quote := int64(1048576 + 8192)
	p.Models[antigravity.Name+":"+antigravity.FlashHighModel] = budget.ModelPrice{Basis: "subscription_no_overage"}
	// Conservative model AND tool reservations are monotonic, not measured
	// elapsed wall time. Cover the declared model caps plus the mandate's20
	// finite tool slots before opening a new window; never refund old windows.
	p.Task.ModelTurns, p.Task.Tokens, p.Task.ActiveMillis = 32, 32*quote, 32*p.ModelMillis+20*p.ToolMillis
	p.Mandate.ModelTurns, p.Mandate.Tokens, p.Mandate.ActiveMillis = 40, 40*quote, 40*p.ModelMillis+20*p.ToolMillis
	p.Day, p.Conversation = p.Mandate, p.Task
	p.OutputTokens, p.ModelMillis, p.ModelInputBytes, p.TaskLifetimeMillis = 8192, 120000, 131072, 1200000
	return releaseChainModelConfig{Provider: antigravity.Name, Model: antigravity.FlashHighModel, InputTokens: 1048576, OutputTokens: 8192, Policy: p, FixtureTimeout: 20 * time.Minute, ChildBudget: budget.Units{ModelTurns: 8, Tokens: 8 * quote, ExternalCalls: 8, ActiveMillis: 8*p.ModelMillis + 8*p.ToolMillis, EgressBytes: 16 << 20, ProviderInputBytes: 16 << 20}, FailFastTerminalContinuation: true}
}

type converterLocalNativeProvider struct {
	transport.ModelProvider
	checker       *releaseChainEntryChecker
	journal       *pgxpool.Pool
	persistFailed atomic.Bool
}

func (p *converterLocalNativeProvider) InferenceLimits() (int64, int64) { return 1048576, 8192 }
func (p *converterLocalNativeProvider) InferenceTimeout() time.Duration { return 120 * time.Second }

func (p *converterLocalNativeProvider) invoke(ctx context.Context, kind, run string, input any, call func(context.Context) (transport.EventStream, error)) (transport.EventStream, error) {
	if p.persistFailed.Load() {
		return nil, errConverterLocalNative
	}
	if _, err := p.checker.guard(ctx, run); err != nil {
		return nil, errConverterLocalNative
	}
	// Opaque provider continuity is retained only by the actual adapter, not
	// copied into research artifacts. This copy never changes the live request.
	switch q := input.(type) {
	case transport.StartRunRequest:
		q.Context.ProviderSessionRef = nil
		input = q
	case transport.ContinueRunRequest:
		q.ProviderSessionRef = nil
		input = q
	case transport.SubmitToolResultRequest:
		q.ProviderSessionRef = nil
		input = q
	}
	raw, err := json.Marshal(input)
	if err != nil {
		return nil, errConverterLocalNative
	}
	tx, err := p.journal.Begin(ctx)
	if err != nil {
		return nil, errConverterLocalNative
	}
	defer tx.Rollback(ctx)
	var n int
	err = tx.QueryRow(ctx, `UPDATE public.converter_local_native_windows SET admitted=admitted+1 WHERE id=$1 AND NOT closed AND admitted<$2 AND expires_at>clock_timestamp() RETURNING admitted`, converterLocalNativeWindow, converterLocalNativeCap).Scan(&n)
	if err != nil {
		return nil, errConverterLocalNative
	}
	_, err = tx.Exec(ctx, `INSERT INTO public.converter_local_native_attempts(window_id,n,run_id,kind,input,status) VALUES($1,$2,$3,$4,$5,'unknown')`, converterLocalNativeWindow, n, run, kind, raw)
	if err != nil || tx.Commit(ctx) != nil {
		return nil, errConverterLocalNative
	}
	// Inference starts only after an UNKNOWN admission is durable. No owning
	// action/source transaction or model-supplied privilege spans native HTTP.
	var stage atomic.Value
	stage.Store("")
	observed := transport.WithProviderStageObserver(ctx, func(s transport.ProviderStage) { stage.Store(string(s)) })
	started := time.Now()
	stream, err := call(observed)
	finish := func(status string, events []map[string]any, usage *transport.Usage, version string) {
		persist, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		body, _ := json.Marshal(events)
		var in, out, total, thinking any
		if usage != nil {
			in, out, total = usage.InputTokens, usage.OutputTokens, usage.TotalTokens
			if usage.ThinkingTokens != nil {
				thinking = *usage.ThinkingTokens
			}
		}
		_, e := p.journal.Exec(persist, `UPDATE public.converter_local_native_attempts SET status=$3,stage=$4,milliseconds=$5,events=$6,input_tokens=$7,output_tokens=$8,total_tokens=$9,thinking_tokens=$10,model_version=NULLIF($11,'') WHERE window_id=$1 AND n=$2 AND status='unknown'`, converterLocalNativeWindow, n, status, stage.Load().(string), time.Since(started).Milliseconds(), body, in, out, total, thinking, version)
		if e != nil {
			p.persistFailed.Store(true)
		} // UNKNOWN stays spent; no further admission.
	}
	if err != nil {
		finish("entry_failed", []map[string]any{{"type": "entry_failed", "error_code": transport.DiagnosticCode(err), "safe_reason": transport.DiagnosticReason(err), "http_status": transport.DiagnosticHTTPStatus(err)}}, nil, "")
		return nil, errConverterLocalNative
	}
	output := make(chan transport.TransportEvent)
	go func() {
		defer close(output)
		status, version := "stream_closed", ""
		var events []map[string]any
		var usage *transport.Usage
		defer func() { finish(status, events, usage, version) }()
		for event := range stream {
			if event.Usage != nil {
				copy := *event.Usage
				usage = &copy
			}
			if usage == nil && event.AssistantFinal != nil && event.AssistantFinal.Usage != nil {
				copy := *event.AssistantFinal.Usage
				usage = &copy
			}
			if event.ProviderModelVersion != nil {
				version = *event.ProviderModelVersion
			}
			safe := map[string]any{"type": event.EventType}
			if event.ToolCall != nil {
				safe["tool_call"] = event.ToolCall
			}
			if event.ToolCallBatch != nil {
				safe["tool_batch"] = event.ToolCallBatch
			}
			if event.AssistantFinal != nil {
				safe["final"] = event.AssistantFinal
			}
			if event.Usage != nil {
				safe["usage"] = event.Usage
			}
			if event.TransportError != nil {
				safe["error_code"] = transport.DiagnosticCode(event.TransportError)
				safe["safe_reason"] = transport.DiagnosticReason(event.TransportError)
				if status := transport.DiagnosticHTTPStatus(event.TransportError); status > 0 {
					safe["http_status"] = status
				}
				status = "transport_error"
			}
			if event.EventType == transport.EventTypeRunCompleted && status != "transport_error" {
				status = "completed"
			}
			events = append(events, safe)
			select {
			case output <- event:
			case <-ctx.Done():
				status = "cancelled"
				return
			}
		}
	}()
	return output, nil
}
func (p *converterLocalNativeProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	return p.invoke(ctx, "start", q.Context.RunID, q, func(c context.Context) (transport.EventStream, error) { return p.ModelProvider.StartRun(c, q) })
}
func (p *converterLocalNativeProvider) ContinueRun(ctx context.Context, q transport.ContinueRunRequest) (transport.EventStream, error) {
	return p.invoke(ctx, "continue", q.RunID, q, func(c context.Context) (transport.EventStream, error) { return p.ModelProvider.ContinueRun(c, q) })
}
func (p *converterLocalNativeProvider) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.invoke(ctx, "submit", q.RunID, q, func(c context.Context) (transport.EventStream, error) { return p.ModelProvider.SubmitToolResult(c, q) })
}

func converterLocalNativeExport(t *testing.T, pool *pgxpool.Pool, dir, schema string, tables []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, table := range tables {
		// Schema comes from the actual isolated PG fixture, tables from this fixed
		// caller list. Neither identifier is model-controlled.
		identifier := pgx.Identifier{schema, table}.Sanitize()
		rows, err := pool.Query(ctx, `SELECT to_jsonb(t) FROM `+identifier+` t`)
		if err != nil {
			t.Error("local evidence table unavailable", table)
			continue
		}
		file, err := os.OpenFile(filepath.Join(dir, table+".jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			rows.Close()
			t.Error("local evidence file unavailable", table)
			continue
		}
		for rows.Next() {
			var raw []byte
			if rows.Scan(&raw) != nil {
				t.Error("evidence row unavailable", table)
				break
			}
			if _, err = file.Write(append(raw, '\n')); err != nil {
				t.Error("evidence write failed", table)
				break
			}
		}
		if rows.Err() != nil {
			t.Error("evidence query failed", table)
		}
		rows.Close()
		if file.Sync() != nil {
			t.Error("evidence sync failed", table)
		}
		_ = file.Close()
	}
}

func TestConverterDeveloperActualHighLocalContainerE2E(t *testing.T) {
	if os.Getenv("BUTLER_CONVERTER_LOCAL_NATIVE") != "synthetic-local-only" {
		t.Skip("explicit independent local native converter window required")
	}
	if runtime.GOOS != "linux" || os.Getenv("BUTLER_CONVERTER_DEVELOPER_E2E") != "1" {
		t.Fatal("local isolated Linux process fixture required")
	}
	dumpBinary, err := exec.LookPath("pg_dump")
	if err != nil {
		t.Fatal("local schema backup executable required before inference")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 22*time.Minute)
	defer cancel()
	journalConfig, err := pgxpool.ParseConfig("postgres://postgres@127.0.0.1:55449/converters20_test?sslmode=disable")
	if err != nil {
		t.Fatal("local journal config invalid")
	}
	journalConfig.MaxConns = 2
	journal, err := pgxpool.NewWithConfig(ctx, journalConfig)
	if err != nil {
		t.Fatal("isolated native journal unavailable")
	}
	t.Cleanup(journal.Close)
	dir := os.Getenv("BUTLER_CONVERTER_LOCAL_NATIVE_EVIDENCE")
	if dir == "" || !filepath.IsAbs(dir) || os.Mkdir(dir, 0700) != nil {
		t.Fatal("new absolute private evidence directory required")
	}
	cfg := converterLocalNativeConfig()
	observer := &releaseChainSourceObserver{}
	cfg.SourceObserver = observer
	var checks, totalNanos, maxNanos atomic.Int64
	observer.onComplete = func(role string, elapsed time.Duration) {
		if role != "source_check" {
			return
		}
		checks.Add(1)
		totalNanos.Add(int64(elapsed))
		for old := maxNanos.Load(); int64(elapsed) > old; old = maxNanos.Load() {
			if maxNanos.CompareAndSwap(old, int64(elapsed)) {
				break
			}
		}
	}
	source := os.Getenv("BUTLER_CONVERTER_LOCAL_NATIVE_SOURCE")
	if !regexp.MustCompile(`^git:[a-f0-9]{40};binary-sha256:[a-f0-9]{64}$`).MatchString(source) {
		t.Fatal("committed producer and binary identity required")
	}
	manifest, err := json.Marshal(map[string]any{
		"window": converterLocalNativeWindow, "source": source, "runtime": runtime.Version() + "/" + runtime.GOOS + "/" + runtime.GOARCH,
		"goal": converterOwnerAcceptanceGoal, "provider": cfg.Provider, "model": cfg.Model,
		"input_token_allowance": cfg.InputTokens, "output_token_allowance": cfg.OutputTokens,
		"policy": cfg.Policy, "child_budget": cfg.ChildBudget, "fixture_millis": cfg.FixtureTimeout.Milliseconds(),
		"profile": json.RawMessage(os.Getenv("BUTLER_CONVERTER_PROFILE_JSON")), "cap": converterLocalNativeCap,
		"prior_windows":                  []string{converterLocalNativePriorWindow, converterLocalNativePriorBWindow, converterLocalNativePriorCWindow, converterLocalNativePriorDWindow},
		"prior_actual_calls":             converterLocalNativePriorCalls + converterLocalNativePriorBCalls + converterLocalNativePriorCCalls + converterLocalNativePriorDCalls,
		"prior_known_tokens_lower_bound": converterLocalNativePriorTokens + converterLocalNativePriorBTokens + converterLocalNativePriorCTokens + converterLocalNativePriorDTokens,
		"prior_kernel_unknown_attempts":  2, "prior_kernel_unknown_before_native": 1, "prior_native_unknown_usage": 1,
		"cumulative_actual_call_cap": converterLocalNativePriorCalls + converterLocalNativePriorBCalls + converterLocalNativePriorCCalls + converterLocalNativePriorDCalls + converterLocalNativeCap,
		"scope":                      "single synthetic converter-only task, native planning, real Broker/runtime; no issue33 qualification, no working deployment or real owner delivery",
	})
	if err != nil {
		t.Fatal("prospective manifest invalid")
	}
	_, err = journal.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.converter_local_native_windows(id text PRIMARY KEY,manifest jsonb NOT NULL,admitted int NOT NULL DEFAULT 0,closed bool NOT NULL DEFAULT false,expires_at timestamptz NOT NULL); CREATE TABLE IF NOT EXISTS public.converter_local_native_attempts(window_id text NOT NULL,n int NOT NULL,run_id text NOT NULL,kind text NOT NULL,input jsonb NOT NULL,status text NOT NULL,stage text,milliseconds bigint,events jsonb,input_tokens bigint,output_tokens bigint,total_tokens bigint,thinking_tokens bigint,model_version text,started_at timestamptz NOT NULL DEFAULT clock_timestamp(),PRIMARY KEY(window_id,n))`)
	if err != nil {
		t.Fatal("native ledger unavailable")
	}
	var priorClosed bool
	var priorAdmitted, priorCalls, priorTokens, priorMissing int64
	if err = journal.QueryRow(ctx, `SELECT closed,admitted,(SELECT count(*) FROM public.converter_local_native_attempts WHERE window_id=$1),(SELECT COALESCE(sum(total_tokens),0) FROM public.converter_local_native_attempts WHERE window_id=$1),(SELECT count(*) FROM public.converter_local_native_attempts WHERE window_id=$1 AND (total_tokens IS NULL OR status='unknown')) FROM public.converter_local_native_windows WHERE id=$1`, converterLocalNativePriorWindow).Scan(&priorClosed, &priorAdmitted, &priorCalls, &priorTokens, &priorMissing); err != nil || !priorClosed || priorAdmitted != converterLocalNativePriorCalls || priorCalls != converterLocalNativePriorCalls || priorTokens != converterLocalNativePriorTokens || priorMissing != 0 {
		t.Fatal("prior failed window must remain closed with its complete original spent ledger")
	}
	if err = journal.QueryRow(ctx, `SELECT closed,admitted,(SELECT count(*) FROM public.converter_local_native_attempts WHERE window_id=$1),(SELECT COALESCE(sum(total_tokens),0) FROM public.converter_local_native_attempts WHERE window_id=$1),(SELECT count(*) FROM public.converter_local_native_attempts WHERE window_id=$1 AND (total_tokens IS NULL OR status='unknown')) FROM public.converter_local_native_windows WHERE id=$1`, converterLocalNativePriorBWindow).Scan(&priorClosed, &priorAdmitted, &priorCalls, &priorTokens, &priorMissing); err != nil || !priorClosed || priorAdmitted != converterLocalNativePriorBCalls || priorCalls != converterLocalNativePriorBCalls || priorTokens != converterLocalNativePriorBTokens || priorMissing != 0 {
		t.Fatal("prior budget-stopped window must remain closed with its complete original spent ledger")
	}
	if err = journal.QueryRow(ctx, `SELECT closed,admitted,(SELECT count(*) FROM public.converter_local_native_attempts WHERE window_id=$1),(SELECT COALESCE(sum(total_tokens),0) FROM public.converter_local_native_attempts WHERE window_id=$1),(SELECT count(*) FROM public.converter_local_native_attempts WHERE window_id=$1 AND (total_tokens IS NULL OR status='unknown')) FROM public.converter_local_native_windows WHERE id=$1`, converterLocalNativePriorCWindow).Scan(&priorClosed, &priorAdmitted, &priorCalls, &priorTokens, &priorMissing); err != nil || !priorClosed || priorAdmitted != converterLocalNativePriorCCalls || priorCalls != converterLocalNativePriorCCalls || priorTokens != converterLocalNativePriorCTokens || priorMissing != 1 {
		t.Fatal("prior stream failure and its unavailable usage must remain unchanged")
	}
	var cancelled bool
	if err = journal.QueryRow(ctx, `SELECT status='cancelled' AND stage='response_stream' AND total_tokens IS NULL FROM public.converter_local_native_attempts WHERE window_id=$1 AND n=12`, converterLocalNativePriorCWindow).Scan(&cancelled); err != nil || !cancelled {
		t.Fatal("prior actual cancelled HTTP stream cannot be replaced by zero usage")
	}
	if err = journal.QueryRow(ctx, `SELECT closed,admitted,(SELECT count(*) FROM public.converter_local_native_attempts WHERE window_id=$1),(SELECT COALESCE(sum(total_tokens),0) FROM public.converter_local_native_attempts WHERE window_id=$1),(SELECT count(*) FROM public.converter_local_native_attempts WHERE window_id=$1 AND (total_tokens IS NULL OR status='unknown')) FROM public.converter_local_native_windows WHERE id=$1`, converterLocalNativePriorDWindow).Scan(&priorClosed, &priorAdmitted, &priorCalls, &priorTokens, &priorMissing); err != nil || !priorClosed || priorAdmitted != converterLocalNativePriorDCalls || priorCalls != converterLocalNativePriorDCalls || priorTokens != converterLocalNativePriorDTokens || priorMissing != 0 {
		t.Fatal("prior action-count test failure must remain closed with its original ledger")
	}
	if _, err = journal.Exec(ctx, `INSERT INTO public.converter_local_native_windows(id,manifest,expires_at) VALUES($1,$2,clock_timestamp()+interval '20 minutes')`, converterLocalNativeWindow, manifest); err != nil {
		t.Fatal("native window already exists or unavailable; no resume/retry")
	}
	if err = os.WriteFile(filepath.Join(dir, "manifest.json"), append(manifest, '\n'), 0600); err != nil {
		t.Fatal("prospective manifest unavailable")
	}
	t.Cleanup(func() {
		_, _ = journal.Exec(context.Background(), `UPDATE public.converter_local_native_windows SET closed=true WHERE id=$1`, converterLocalNativeWindow)
		converterLocalNativeExport(t, journal, dir, "public", []string{"converter_local_native_windows", "converter_local_native_attempts"})
	})
	t.Cleanup(func() {
		proof, err := json.Marshal(map[string]any{"observation": observer.snapshot(), "source_checks": checks.Load(), "total_millis": time.Duration(totalNanos.Load()).Milliseconds(), "max_millis": time.Duration(maxNanos.Load()).Milliseconds()})
		if err != nil || os.WriteFile(filepath.Join(dir, "source-check-diagnostics.json"), append(proof, '\n'), 0600) != nil {
			t.Error("fixed source-check diagnostics unavailable")
		}
	})
	var f *developerFixture
	cfg.Factory = func(ctx context.Context, pool *pgxpool.Pool, root string) (transport.ModelProvider, error) {
		keyFile, err := privateNativeFixtureFile("/private/converter20/settings.key", 128)
		if err != nil {
			return nil, errConverterLocalNative
		}
		key, err := io.ReadAll(keyFile)
		_ = keyFile.Close()
		if err != nil {
			return nil, errConverterLocalNative
		}
		manager := providerauth.NewManager(config.NewPostgresSettingsStore(journal, config.WithSettingsCodec(config.StaticSecretValueCodec{Key: strings.TrimSpace(string(key))})))
		native, err := antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, Timeout: 120 * time.Second, AuthSource: manager})
		if err != nil {
			return nil, errConverterLocalNative
		}
		return &converterLocalNativeProvider{ModelProvider: native, journal: journal, checker: &releaseChainEntryChecker{pool: pool, sources: f.f.sources, root: root, document: f.document, need: f.needRef, spec: f.specRef}}, nil
	}
	hooks := &converterWorkerFaultHooks{verifyActions: verifyConverterLocalActionInventory, factory: func(t *testing.T, p generatedtools.Profile, path string) *developerFixture {
		f = newDeveloperWorkerFixtureWithModel(t, p, path, true, cfg.FixtureTimeout, &cfg)
		var schema string
		if f.f.pool.QueryRow(f.f.ctx, `SELECT current_schema()`).Scan(&schema) != nil {
			t.Fatal("fixture identity unavailable")
		}
		t.Cleanup(func() {
			converterLocalNativeExport(t, f.f.pool, dir, schema, []string{"tasks", "runs", "accepted_inputs", "execution_model_attempts", "execution_budget_scopes", "execution_budget_reservations", "tool_calls", "run_final_results", "memory_sources", "memory_scope_grants", "memory_run_sources", "memory_source_dependencies", "memory_source_revisions", "task_delegations", "task_authority_bindings", "task_results", "converter_versions", "converter_states", "converter_catalog", "converter_developer_snapshots", "converter_developer_returns", "converter_parent_continuations", "converter_events", "converter_needs", "converter_trials", "converter_usages", "converter_validations", "action_intents", "action_authorizations", "action_effect_dispatches", "action_effect_evidence", "action_effects", "action_mandates", "owner_goals", "owner_purpose_policies"})
			// Register after fixture construction, so this runs before its pool
			// closes and schema cleanup. No external lock changes execution or DROP.
			backupCtx, backupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer backupCancel()
			file, e := os.OpenFile(filepath.Join(dir, "schema.dump"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				t.Error("private local schema backup unavailable")
				return
			}
			defer file.Close()
			command := exec.CommandContext(backupCtx, dumpBinary, "--no-owner", "--no-privileges", "--format=custom", "--schema="+schema, "--dbname=postgres://postgres@127.0.0.1:55449/converters20_test?sslmode=disable")
			command.Stdout = file
			if command.Run() != nil || file.Sync() != nil {
				t.Error("private local schema backup failed; retain exported rows")
			}
		})
		return f
	}}
	testConverterDeveloperWorkersObserved(t, true, hooks)
}
