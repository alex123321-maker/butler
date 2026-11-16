package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravityacp"
	"github.com/jackc/pgx/v5/pgxpool"
)

type candidateACPCapture struct {
	transport.ModelProvider
	mu     sync.Mutex
	output string
	usage  bool
}

func (p *candidateACPCapture) AgentExecutionLimits() transport.AgentExecutionLimits {
	return p.ModelProvider.(interface {
		AgentExecutionLimits() transport.AgentExecutionLimits
	}).AgentExecutionLimits()
}
func (p *candidateACPCapture) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	p.mu.Lock()
	p.output, p.usage = "", false
	p.mu.Unlock()
	s, err := p.ModelProvider.StartRun(ctx, r)
	if err != nil {
		return nil, err
	}
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		for event := range s {
			p.mu.Lock()
			if event.AssistantFinal != nil {
				p.output = event.AssistantFinal.Content
			}
			if event.Usage != nil {
				p.usage = true
			}
			p.mu.Unlock()
			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

// A new persistent, once-only window. It never reads, resets or re-executes the
// historical native windows. Unknown native usage is expected by the explicit
// owner-approved accounting mode; semantic/citation/safety thresholds are intact.
func TestCandidateACPRussianQuality(t *testing.T) {
	proof := os.Getenv("BUTLER_ACP_CANDIDATE_EVIDENCE")
	if proof == "" {
		t.Skip("explicit fresh persistent ACP candidate window required")
	}
	raw, err := os.ReadFile("../../../../testdata/acceptance/ru-memory-candidates-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	holdoutRaw, err := os.ReadFile("../../../../testdata/acceptance/ru-memory-candidates-holdout-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var suite, holdout struct{ Cases []candidateNativeCase }
	if json.Unmarshal(raw, &suite) != nil || len(suite.Cases) != 24 || json.Unmarshal(holdoutRaw, &holdout) != nil || len(holdout.Cases) != 8 {
		t.Fatal("fixed corpus unavailable")
	}
	var caps struct {
		WindowID                                                     string
		CorpusSHA256, HoldoutSHA256, InstructionSHA256, SourceSHA256 string
		MaxCalls                                                     int
		ExpiresAt                                                    time.Time
		PriorWindowID, PriorResultsSHA256                            string
		ResumeAfter                                                  int
	}
	capsRaw, err := os.ReadFile(filepath.Join(proof, "caps.json"))
	if err != nil || json.Unmarshal(capsRaw, &caps) != nil || !strings.HasPrefix(caps.WindowID, "candidate-v") || len(caps.WindowID) > 32 || caps.CorpusSHA256 != candidateHash(raw) || caps.HoldoutSHA256 != candidateHash(holdoutRaw) || caps.InstructionSHA256 != candidateHash([]byte((candidates.Handler{}).Instruction())) || len(caps.SourceSHA256) != 64 || !time.Now().Before(caps.ExpiresAt) {
		t.Fatal("predeclared ACP caps unavailable")
	}
	remainder := strings.HasSuffix(caps.WindowID, "-remainder") && ((caps.PriorWindowID == "candidate-v2" && caps.ResumeAfter == 30 && caps.MaxCalls == 2) || (caps.PriorWindowID == "candidate-v4-remainder" && caps.ResumeAfter == 31 && caps.MaxCalls == 1)) && len(caps.PriorResultsSHA256) == 64
	if !remainder && (caps.PriorWindowID != "" || caps.ResumeAfter != 0 || caps.MaxCalls != 32) {
		t.Fatal("undeclared window shape")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_ACP_MEMORY_POSTGRES_URL"))
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("dedicated persistent test database required")
	}
	cfg.MaxConns = 2
	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS acp_qualification_windows(window_id TEXT PRIMARY KEY,started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),caps JSONB NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	tag, err := pool.Exec(ctx, `INSERT INTO acp_qualification_windows(window_id,caps) VALUES($1,$2) ON CONFLICT DO NOTHING`, caps.WindowID, capsRaw)
	if err != nil || tag.RowsAffected() != 1 {
		t.Fatal("ACP window already consumed; old failures/spending must remain", err)
	}
	manifest := catalog.NewFileManifest(filepath.Join(proof, "deletions.log"))
	if remainder {
		_, err = os.Stat(filepath.Join(proof, "deletions.log"))
	} else {
		err = manifest.Initialize()
	}
	if err != nil {
		t.Fatal(err)
	}
	c := catalog.New(pool, manifest)
	policy := budget.DefaultPolicy()
	policy.ModelMillis = 300000
	policy.Day.ModelTurns = 40
	policy.Models["antigravity-acp:gemini-3.8-flash"] = budget.ModelPrice{Basis: budget.AgentSubscriptionBasis}
	b := budget.NewController(pool, policy)
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	j := jobs.New(pool, c, b, nil)
	p, err := antigravityacp.NewProvider(antigravityacp.Config{Enabled: true, SocketPath: os.Getenv("BUTLER_ACP_MEMORY_SOCKET"), Model: "gemini-3.8-flash", Timeout: 300 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	capture := &candidateACPCapture{ModelProvider: p}
	w := &jobs.ModelWorker{Jobs: j, Provider: capture, Model: "gemini-3.8-flash", Handlers: map[string]jobs.ModelHandler{candidates.Version: candidates.Handler{}}}
	srv := httptest.NewServer((&MemoryCatalogServer{Store: c, Auth: NewOwnerControlAuth(strings.Repeat("c", 40), "owner"), ModelJobs: j, ModelWorkerConfigured: true}).Handle())
	defer srv.Close()
	f := &candidateFixture{t: t, pool: pool, catalog: c, jobs: j, budgets: b, worker: w, server: srv, owner: owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "acp-quality-owner"})}
	f.enable()
	defer j.SetModelControl(ctx, "disabled")
	type result struct {
		ID                                  string
		Citation, Semantic, UnexpectedUsage bool
		Millis                              int64
		Error                               string
	}
	results := []result{}
	coreMatches, holdoutMatches, citations, unsafe := 0, 0, 0, 0
	allCases := append(append([]candidateNativeCase{}, suite.Cases...), holdout.Cases...)
	if remainder {
		// A failed infrastructure window remains failed. Carry its exact retained
		// scores and run only the two cases that never acquired a model attempt.
		priorRaw, err := os.ReadFile(filepath.Join(proof, "prior-results.json"))
		if err != nil || candidateHash(priorRaw) != caps.PriorResultsSHA256 || json.Unmarshal(priorRaw, &results) != nil || len(results) != caps.ResumeAfter {
			t.Fatal("frozen prior receipts required")
		}
		for index, o := range results {
			if o.ID != allCases[index].ID || !o.Citation || o.UnexpectedUsage || o.Error != "" {
				t.Fatal("prior receipt cannot be carried")
			}
			var count int
			originalWindow := caps.PriorWindowID
			if index < 30 {
				originalWindow = "candidate-v2"
			}
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM memory_model_attempts a JOIN memory_aux_jobs j USING(job_id) WHERE j.policy_id=$1 AND a.state='published' AND a.usage IS NULL AND a.provider='antigravity-acp' AND a.model='gemini-3.8-flash'`, "acp-"+originalWindow+":"+o.ID).Scan(&count); err != nil || count != 1 {
				t.Fatal("prior publication missing", err)
			}
			citations++
			if o.Semantic {
				if index < 24 {
					coreMatches++
				} else {
					holdoutMatches++
				}
			}
			if allCases[index].Skip && !o.Semantic {
				unsafe++
			}
		}
		for _, tc := range allCases[caps.ResumeAfter:] {
			var count int
			oldID := "acp-" + caps.PriorWindowID + ":" + tc.ID
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM memory_model_attempts a JOIN memory_aux_jobs j USING(job_id) WHERE j.policy_id LIKE 'acp-candidate-v%:'||$1`, tc.ID).Scan(&count); err != nil || count != 0 {
				t.Fatal("remainder would repeat a model attempt", err)
			}
			var exists bool
			if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM memory_model_policies WHERE policy_id=$1 AND jobs_created=0)`, oldID).Scan(&exists); err != nil {
				t.Fatal(err)
			}
			if exists {
				if err := j.ControlModel(f.owner, oldID, "revoked"); err != nil {
					t.Fatal("unspent prior policy could still schedule", err)
				}
			}
		}
	}
	defer func() {
		var ledger json.RawMessage
		e := pool.QueryRow(ctx, `SELECT jsonb_build_object('attempts',(SELECT COALESCE(jsonb_agg(to_jsonb(a)),'[]'::jsonb) FROM memory_model_attempts a),'scopes',(SELECT COALESCE(jsonb_agg(to_jsonb(s)),'[]'::jsonb) FROM execution_budget_scopes s),'confirmed_claims',(SELECT count(*) FROM memory_claims WHERE status='confirmed'),'tasks',(SELECT count(*) FROM tasks),'mandates',(SELECT count(*) FROM action_mandates))`).Scan(&ledger)
		if e != nil {
			t.Error(e)
		}
		var authority struct {
			ConfirmedClaims int `json:"confirmed_claims"`
			Tasks           int `json:"tasks"`
			Mandates        int `json:"mandates"`
		}
		if json.Unmarshal(ledger, &authority) != nil || authority.ConfirmedClaims != 0 || authority.Tasks != 0 || authority.Mandates != 0 {
			t.Error("candidate generation created authority")
		}
		passed := len(results) == 32 && coreMatches >= 23 && holdoutMatches == 8 && citations == 32 && unsafe == 0 && !t.Failed()
		write := func(name string, value any) {
			out, e := json.MarshalIndent(value, "", "  ")
			if e == nil {
				e = os.WriteFile(filepath.Join(proof, name), out, 0600)
			}
			if e != nil {
				t.Error(e)
			}
		}
		write("ledger.json", ledger)
		write("results.json", results)
		write("quality.json", map[string]any{"passed": passed, "cases": len(results), "carried_cases": caps.ResumeAfter, "new_cases": len(results) - caps.ResumeAfter, "core_semantic_matches": coreMatches, "holdout_semantic_matches": holdoutMatches, "citation_cases": citations, "unsafe_promotions": unsafe, "native_usage": "unknown", "accounting_mode": "opaque_subscription", "caps": caps})
		if !passed {
			t.Error("predeclared ACP semantic/citation/safety screen failed")
		}
	}()
	for index, tc := range allCases {
		if index < caps.ResumeAfter {
			continue
		}
		if !time.Now().Before(caps.ExpiresAt) {
			t.Error("window expired; remaining cases unexecuted")
			break
		}
		src := f.source("acp-"+caps.WindowID+":"+tc.ID, tc.Body)
		q := jobs.ModelPolicyRequest{ID: "acp-" + caps.WindowID + ":" + tc.ID, Handler: candidates.Version, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", AccountingMode: "opaque_subscription", Job: jobs.Request{Sources: []catalog.SourceRef{{ID: src.ID, Revision: src.Revision}}, Allowance: budget.Units{ModelTurns: 1, ActiveMillis: 300000, ProviderInputBytes: 8388608, EgressBytes: 65536}, Deadline: caps.ExpiresAt}}
		f.request("/api/v4/memory/model-policies", "POST", q, 201, nil)
		start := time.Now()
		// The production scanner processes a bounded batch. Exhausted historical
		// policies may occupy its first pass; wait for ordinary queue admission,
		// without bypassing the scanner or repeating an actual model attempt.
		var err error
		worked := false
		for scan := 0; scan < 20; scan++ {
			if err = j.PollModelPolicies(ctx, func(name string) bool { return name == candidates.Version }); err != nil {
				break
			}
			worked, err = w.ProcessOne(ctx)
			if worked || err != nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if !worked && err == nil {
			t.Fatal("ordinary scanner did not admit the bounded case")
		}
		capture.mu.Lock()
		output, usage := capture.output, capture.usage
		capture.mu.Unlock()
		o := result{ID: tc.ID, Millis: time.Since(start).Milliseconds(), UnexpectedUsage: usage}
		canonical, valid := (candidates.Handler{}).Validate([]catalog.Source{src}, output)
		if valid == nil && err == nil {
			citations++
			o.Citation = true
			var units candidates.Result
			_ = json.Unmarshal([]byte(canonical), &units)
			if tc.Skip {
				o.Semantic = len(units.Candidates) == 0
				if !o.Semantic {
					unsafe++
				}
			} else if len(units.Candidates) == 1 {
				u := units.Candidates[0]
				o.Semantic = u.Type == tc.Type && u.SpeechAct == tc.SpeechAct && u.Quote == tc.Body && ((tc.Scope == "" && u.ScopeQuote == nil) || (u.ScopeQuote != nil && strings.Contains(*u.ScopeQuote, tc.Scope))) && ((tc.Time == "" && u.ValidTimeQuote == nil) || (u.ValidTimeQuote != nil && strings.Contains(*u.ValidTimeQuote, tc.Time)))
			}
		}
		if o.Semantic {
			if index < 24 {
				coreMatches++
			} else {
				holdoutMatches++
			}
		}
		if err != nil {
			o.Error = transport.DiagnosticCode(err)
			if o.Error == "" {
				o.Error = "worker_rejected"
			}
		}
		results = append(results, o)
		t.Logf("ACP case=%s citation=%t semantic=%t millis=%d", o.ID, o.Citation, o.Semantic, o.Millis)
		if usage {
			t.Error("opaque provider invented usage")
			break
		}
		if err != nil && output == "" {
			t.Error("provider failed; remaining cases unexecuted")
			break
		}
	}
}
