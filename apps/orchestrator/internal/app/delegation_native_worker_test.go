package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func nativeExperimentPolicy() budget.Policy {
	p := budget.DefaultPolicy()
	p.Task.ModelTurns, p.Task.Children = 6, 2
	p.Task.Tokens, p.Task.ActiveMillis = 8*1048576, 1200000
	p.Task.ExternalCalls, p.Task.EgressBytes, p.Task.ProviderInputBytes = 2, 8388608, 6*131072
	p.Mandate, p.Day = p.Task, p.Task
	p.Mandate.ModelTurns, p.Day.ModelTurns = 12, 12
	p.Mandate.Tokens, p.Day.Tokens = 16*1048576, 16*1048576
	p.Mandate.Children, p.Day.Children = 4, 4
	p.Mandate.ExternalCalls, p.Day.ExternalCalls = 4, 4
	p.Mandate.ActiveMillis, p.Day.ActiveMillis = 2400000, 2400000
	p.Mandate.EgressBytes, p.Day.EgressBytes = 16777216, 16777216
	p.Mandate.ProviderInputBytes, p.Day.ProviderInputBytes = 12*131072, 12*131072
	p.Conversation = p.Task
	p.ModelMillis, p.ToolMillis, p.ModelInputBytes = 120000, 30000, 131072
	p.TaskLifetimeMillis = 3600000
	p.Models = map[string]budget.ModelPrice{antigravity.Name + ":" + antigravity.DefaultModel: {Basis: "subscription_no_overage"}}
	return p
}

type nativeWorkerConfig struct {
	Width        int    `json:"width"`
	Gate         bool   `json:"gate"`
	Broker       string `json:"broker"`
	TokenFile    string `json:"token_file"`
	ManifestFile string `json:"manifest_file"`
	ImportFile   string `json:"import_file"`
	KeyFile      string `json:"key_file"`
	CorpusFile   string `json:"corpus_file"`
}

type nativeResearchCorpus struct {
	Version  int    `json:"version"`
	Question string `json:"question"`
	Sources  []struct {
		Key           string `json:"key"`
		Title         string `json:"title"`
		Body          string `json:"body"`
		ExpectedValue int64  `json:"expected_value"`
		ExpectedDate  string `json:"expected_date"`
	} `json:"sources"`
	ResponseFields      []string `json:"response_fields"`
	RequiredUncertainty string   `json:"required_uncertainty"`
}

func readNativeCorpus(path string) (nativeResearchCorpus, error) {
	var c nativeResearchCorpus
	if path == "" {
		_, file, _, _ := runtime.Caller(0)
		path = filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "scripts", "dev", "fixtures", "delegation-native-research.json")
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) > 16384 || json.Unmarshal(b, &c) != nil || c.Version != 1 || len(c.Sources) != 2 || c.RequiredUncertainty != "source_only" || c.Sources[0].ExpectedValue == c.Sources[1].ExpectedValue {
		return c, errNativeFixtureDenied
	}
	return c, nil
}

// This fixture gate delays only entry into the actual child executor. It cannot
// fabricate receipts, tool results, provider output or ownership. SIGKILL leaves
// the production 60-second input claim to expire naturally before takeover.
type nativeChildGate struct {
	executor *flow.Service
	pool     *pgxpool.Pool
	gate     bool
}

func (g nativeChildGate) Execute(ctx context.Context, event ingress.InputEvent) (*flow.ExecutionResult, error) {
	if g.gate && event.Source == "delegation" {
		var p struct {
			TaskID string `json:"task_id"`
		}
		if json.Unmarshal([]byte(event.PayloadJSON), &p) != nil || p.TaskID == "" {
			return nil, errNativeFixtureDenied
		}
		if _, err := g.pool.Exec(ctx, `INSERT INTO fixture_native_gates(task_id,entered_at) VALUES($1,clock_timestamp()) ON CONFLICT DO NOTHING`, p.TaskID); err != nil {
			return nil, errNativeFixtureDenied
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return g.executor.Execute(ctx, event)
}

func (g nativeChildGate) Resume(ctx context.Context, runID string, event ingress.InputEvent) (*flow.ExecutionResult, error) {
	return g.executor.Resume(ctx, runID, event)
}

// Observe the actual prepared request without changing it or substituting a
// model. Only hashes/ref identifiers and validation booleans are persisted.
func nativeContextObserver(pool *pgxpool.Pool, corpus nativeResearchCorpus) func(context.Context, transport.StartRunRequest) error {
	return func(ctx context.Context, r transport.StartRunRequest) error {
		if r.Context.ProviderSessionRef != nil || r.Context.ModelName != antigravity.DefaultModel || r.Context.ProviderName != antigravity.Name {
			return errNativeFixtureDenied
		}
		var raw []byte
		err := pool.QueryRow(ctx, `SELECT d.contract_json FROM runs r JOIN task_delegations d ON d.task_id=r.task_id WHERE r.run_id=$1`, r.Context.RunID).Scan(&raw)
		child := err == nil
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return errNativeFixtureDenied
		}
		refs := []string{}
		if child {
			var c delegation.Contract
			if json.Unmarshal(raw, &c) != nil || len(c.Permissions) != 0 || len(c.Sources) != 1 || len(r.ToolDefinitions) != 0 {
				return errNativeFixtureDenied
			}
			var key string
			if pool.QueryRow(ctx, `SELECT key FROM fixture_native_sources WHERE source_id=$1`, c.Sources[0].ID).Scan(&key) != nil {
				return errNativeFixtureDenied
			}
			selected, contract := false, false
			for _, item := range r.InputItems {
				for _, s := range corpus.Sources {
					if s.Key != key && strings.Contains(item.Content, s.Body) {
						return errNativeFixtureDenied
					}
				}
				o := content.ItemOrigin(item)
				for _, id := range o.SourceRefs {
					if id == c.Sources[0].ID || id == c.ContractSource.ID {
						if o.IsInstruction() || o.Kind == content.OwnerInput || o.Sensitivity == content.Secret {
							return errNativeFixtureDenied
						}
						refs = append(refs, id)
						selected = selected || id == c.Sources[0].ID
						contract = contract || id == c.ContractSource.ID
					}
				}
			}
			if !selected || !contract {
				return errNativeFixtureDenied
			}
		} else if len(r.ToolDefinitions) != 1 || r.ToolDefinitions[0].Name != delegation.AdmissionTool {
			return errNativeFixtureDenied
		}
		b, _ := json.Marshal(r)
		h := sha256.Sum256(b)
		refsJSON, _ := json.Marshal(refs)
		_, err = pool.Exec(ctx, `INSERT INTO fixture_native_contexts(run_id,is_child,input_hash,source_ids,tool_count) VALUES($1,$2,$3,$4,$5)`, r.Context.RunID, child, hex.EncodeToString(h[:]), refsJSON, len(r.ToolDefinitions))
		return err
	}
}

func TestNativeDelegationWorkerHarness(t *testing.T) {
	if os.Getenv("BUTLER_DELEGATION_NATIVE_WORKER_HARNESS") != "1" || os.Getenv("BUTLER_DELEGATION_NATIVE_ACCEPTANCE") != "1" {
		t.Skip("explicit private native experiment worker only")
	}
	var config nativeWorkerConfig
	b, err := os.ReadFile(os.Getenv("BUTLER_DELEGATION_NATIVE_WORKER_CONFIG"))
	if err != nil || len(b) > 8192 || json.Unmarshal(b, &config) != nil || config.Width < 1 || config.Width > 2 {
		t.Fatal("private worker composition unavailable")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_DELEGATION_POSTGRES_URL"))
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") || !strings.HasPrefix(cfg.ConnConfig.RuntimeParams["search_path"], "memory_") {
		t.Fatal("private worker database required")
	}
	cfg.MaxConns = 2
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Minute)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("private worker storage unavailable")
	}
	defer pool.Close()
	corpus, err := readNativeCorpus(config.CorpusFile)
	if err != nil {
		t.Fatal("bounded immutable corpus required")
	}
	limit := &nativeDelegationLimiter{pool}
	provider, err := nativeFixtureProvider(ctx, pool, config.ImportFile, config.KeyFile, limit)
	if err != nil {
		t.Fatal("native encrypted runtime import unavailable")
	}
	provider.observe = nativeContextObserver(pool, corpus)
	bc := budget.NewController(pool, nativeExperimentPolicy())
	bc.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	tasks := task.NewStore(pool)
	tasks.SetBudgets(bc, "")
	sessions := session.NewPostgresRepository(pool)
	inputs := durableinput.NewStore(pool)
	sources := catalog.New(pool, catalog.NewFileManifest(config.ManifestFile))
	limits := task.DefaultDelegationLimits()
	limits.MaxParallel = config.Width
	admission, err := NewDelegationAdmission(pool, tasks, sessions, inputs, sources, limits)
	if err != nil || admission.CheckReady(ctx) != nil {
		t.Fatal("durable child lifecycle unavailable")
	}
	broker, err := tools.DialWithTokenFile(config.Broker, config.TokenFile)
	if err != nil {
		t.Fatal("private Broker unavailable")
	}
	defer broker.Close()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	runs := runservice.NewService(runservice.NewPostgresRepository(pool), log)
	leases := session.NewPostgresLeaseManager(pool)
	approvalRepo := approvals.NewPostgresRepository(pool)
	approvalRepo.SetCheckpointHooks(inputs)
	gate := flow.NewApprovalGate()
	approvalService := approvals.NewService(approvalRepo, gate)
	results := completion.New(pool)
	results.DataGuard = sources.CheckRunSourcesTx
	results.Evidence = effectEvidence{journal: actionauth.NewPostgresStore(pool)}
	ownerGoals := goal.New(pool, tasks, nil)
	executor := flow.NewService(sessions, leases, runs, transcript.NewStore(pool), provider, flow.Config{
		ProviderName: antigravity.Name, ModelName: antigravity.DefaultModel, OwnerID: "native-fixture-worker", LeaseTTL: 60,
		AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, Tasks: tasks, Checkpoints: inputs, Completion: results,
		Budgets: bc, OwnerGoals: ownerGoals, MemorySources: sources, DelegationContexts: delegatedContextReader{sources: sources, manager: admission.Manager},
		Tools: broker, ToolCatalog: broker, ActionAuthorizer: broker, ApprovalChecker: broker, ApprovalGate: gate, ApprovalService: approvalService,
	}, log)
	worker := durableinput.NewService(inputs, sessions, nativeChildGate{executor: executor, pool: pool, gate: config.Gate}, runs, log)
	maintenance := &delegationMaintenance{pool: pool, manager: admission.Manager}
	worker.SetMaintenance(maintenance.Sweep)
	stopped := make(chan os.Signal, 1)
	signal.Notify(stopped, os.Interrupt)
	defer signal.Stop(stopped)
	go func() {
		select {
		case <-stopped:
			cancel()
		case <-ctx.Done():
		}
	}()
	if worker.Run(ctx) != nil {
		t.Fatal("private worker stopped unexpectedly")
	}
}
