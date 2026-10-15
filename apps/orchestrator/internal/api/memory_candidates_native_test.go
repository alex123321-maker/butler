package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

type candidateCaptureProvider struct {
	transport.ModelProvider
	output string
	usage  *transport.Usage
}

func (p *candidateCaptureProvider) InferenceLimits() (int64, int64) {
	return p.ModelProvider.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
}
func (p *candidateCaptureProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	p.output = ""
	p.usage = nil
	s, e := p.ModelProvider.StartRun(ctx, r)
	if e != nil {
		return nil, e
	}
	ch := make(chan transport.TransportEvent)
	go func() {
		defer close(ch)
		for event := range s {
			if event.AssistantFinal != nil {
				p.output = event.AssistantFinal.Content
			}
			if event.Usage != nil {
				copy := *event.Usage
				p.usage = &copy
			}
			select {
			case ch <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}
func candidateHash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// The native window is opt-in, fresh and persistent: unlike isolated storage
// fixtures its paid attempts/reservations MUST survive test failure and restart.
func TestCandidateNativeRussianQuality(t *testing.T) {
	proof := os.Getenv("BUTLER_CANDIDATE_NATIVE_EVIDENCE")
	if proof == "" {
		t.Skip("fresh isolated native window required")
	}
	raw, e := os.ReadFile("../../../../testdata/acceptance/ru-memory-candidates-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var suite struct {
		Version  string          `json:"version"`
		Criteria json.RawMessage `json:"criteria"`
		Cases    []struct {
			ID, Body, Type, Scope, Time string
			SpeechAct                   string `json:"speech_act"`
			Skip                        bool
		}
	}
	if json.Unmarshal(raw, &suite) != nil || len(suite.Cases) != 24 {
		t.Fatal("fixed native corpus unavailable")
	}
	var caps struct {
		CasesSHA256   string    `json:"cases_sha256"`
		MaxCalls      int       `json:"max_calls"`
		ExpiresAt     time.Time `json:"expires_at"`
		Revision      string    `json:"revision"`
		StartCase     int       `json:"start_case"`
		PriorAttempts int       `json:"prior_attempts"`
	}
	b, e := os.ReadFile(filepath.Join(proof, "caps.json"))
	if e != nil || json.Unmarshal(b, &caps) != nil || caps.CasesSHA256 != candidateHash(raw) || caps.StartCase != 0 && caps.StartCase != 3 || caps.PriorAttempts != caps.StartCase || caps.MaxCalls != len(suite.Cases)-caps.StartCase || len(caps.Revision) != 40 || !time.Now().Before(caps.ExpiresAt) {
		t.Fatal("fresh reviewed fixed native caps required")
	}
	if os.Getenv("HTTPS_PROXY") != "http://127.0.0.1:18080" || os.Getenv("HTTP_PROXY") != "" || os.Getenv("ALL_PROXY") != "" {
		t.Fatal("exact server proxy required")
	}
	cfg, e := pgxpool.ParseConfig(os.Getenv("BUTLER_CANDIDATE_NATIVE_POSTGRES_URL"))
	if e != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("isolated persistent native test database required")
	}
	cfg.MaxConns = 2
	trace := &candidatePGTrace{}
	cfg.ConnConfig.Tracer = trace
	pool, e := pgxpool.NewWithConfig(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	trace.pool = pool
	var count int
	if e = pool.QueryRow(context.Background(), `SELECT count(*) FROM memory_model_attempts`).Scan(&count); e != nil || count != caps.PriorAttempts {
		t.Fatal("native window already consumed", count, e)
	}
	manifestPath := filepath.Join(proof, "deletions.log")
	if caps.StartCase > 0 {
		manifestPath = filepath.Join(filepath.Dir(proof), "native-v2", "deletions.log")
	}
	m := catalog.NewFileManifest(manifestPath)
	if caps.StartCase == 0 {
		if e = m.Initialize(); e != nil {
			t.Fatal(e)
		}
	}
	c := catalog.New(pool, m)
	policy := budget.DefaultPolicy()
	policy.ModelMillis = 90000
	policy.Models["antigravity:"+antigravity.FlashHighModel] = budget.ModelPrice{Basis: "subscription_no_overage"}
	bctl := budget.NewController(pool, policy)
	bctl.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	j := jobs.New(pool, c, bctl, nil)
	p, e := antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, AuthSource: providerauth.NewManager(config.NewPostgresSettingsStore(pool)), Timeout: 90 * time.Second})
	if e != nil {
		t.Fatal("native provider unavailable")
	}
	capture := &candidateCaptureProvider{ModelProvider: p}
	w := &jobs.ModelWorker{Jobs: j, Provider: capture, Model: antigravity.FlashHighModel, Handlers: map[string]jobs.ModelHandler{candidates.Version: candidates.Handler{}}, ObserveProbe: trace.probe}
	srv := httptest.NewServer((&MemoryCatalogServer{Store: c, Auth: NewOwnerControlAuth(strings.Repeat("c", 40), "owner"), ModelJobs: j, ModelWorkerConfigured: true}).Handle())
	defer srv.Close()
	f := &candidateFixture{t: t, pool: pool, catalog: c, jobs: j, worker: w, server: srv}
	for _, prior := range suite.Cases[:caps.StartCase] {
		f.request("/api/v4/memory/model-policy-control", "POST", map[string]string{"policy_id": "native-candidate:" + prior.ID, "state": "revoked"}, 200, nil)
	}
	f.enable()
	defer j.SetModelControl(context.Background(), "disabled")
	type observation struct {
		ID               string           `json:"id"`
		Output           string           `json:"output"`
		Usage            *transport.Usage `json:"usage"`
		AcceptedCitation bool             `json:"accepted_citation"`
		SemanticMatch    bool             `json:"semantic_match"`
		Reason           string           `json:"reason"`
		Millis           int64            `json:"millis"`
	}
	results := []observation{}
	semantic, unsafe, quotes, unknown := 0, 0, 0, 0
	write := func(name string, value any) {
		t.Helper()
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(filepath.Join(proof, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err = file.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		write("pg-observations.json", trace.snapshot())
		var ledger json.RawMessage
		var check struct {
			Confirmed int `json:"confirmed_claims"`
			Tasks     int `json:"tasks"`
			Mandates  int `json:"mandates"`
		}
		err := pool.QueryRow(context.Background(), `SELECT jsonb_build_object('attempts',COALESCE((SELECT jsonb_agg(jsonb_build_object('job_id',job_id,'state',state,'quote',quote,'usage',usage,'reason',reason,'result_source_id',result_source_id)) FROM memory_model_attempts),'[]'::jsonb),'scopes',COALESCE((SELECT jsonb_agg(jsonb_build_object('kind',kind,'used',used_json,'limits',limits_json)) FROM execution_budget_scopes),'[]'::jsonb),'confirmed_claims',(SELECT count(*) FROM memory_claims WHERE status='confirmed'),'tasks',(SELECT count(*) FROM tasks),'mandates',(SELECT count(*) FROM action_mandates))`).Scan(&ledger)
		if err != nil {
			t.Errorf("native ledger unavailable: %v", err)
		} else {
			write("ledger.json", ledger)
			if json.Unmarshal(ledger, &check) != nil {
				t.Error("native ledger invalid")
			}
		}
		authorityWrites := check.Confirmed + check.Tasks + check.Mandates
		passed := caps.StartCase == 0 && err == nil && authorityWrites == 0 && len(results) == len(suite.Cases) && float64(semantic)/float64(len(suite.Cases)) >= 0.95 && quotes == len(suite.Cases) && unsafe == 0 && unknown == 0
		write("observations.json", results)
		write("quality.json", map[string]any{"revision": caps.Revision, "corpus": suite.Version, "corpus_sha256": candidateHash(raw), "instruction_sha256": candidateHash([]byte((candidates.Handler{}).Instruction())), "criteria": suite.Criteria, "cases": len(results), "semantic_matches": semantic, "exact_citation_cases": quotes, "unsafe_promotions": unsafe, "authority_writes": authorityWrites, "unknown_usage": unknown, "quality_gate_passed": passed, "automatic_acceptance": false, "pricing_basis": "subscription_no_overage", "production_release_qualified": false, "background_and_foreground_reservation_ledger": "ledger.json", "unmeasured_costs": []string{"OAuth refresh network bytes", "actual HTTP egress bytes", "CPU and storage amortization", "subscription allocation"}})
		if !passed {
			t.Error("predefined native quality gate failed; production stays disabled")
		}
	}()
	for _, tc := range suite.Cases[caps.StartCase:] {
		if !time.Now().Before(caps.ExpiresAt) {
			t.Error("native window expired")
			break
		}
		src := f.source("native:"+tc.ID, tc.Body)
		q := jobs.ModelPolicyRequest{ID: "native-candidate:" + tc.ID, Handler: candidates.Version, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Job: jobs.Request{Sources: []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}, Allowance: budget.Units{ModelTurns: 1, Tokens: 1056768, ActiveMillis: 90000, ProviderInputBytes: 8388608}, Deadline: caps.ExpiresAt}}
		f.request("/api/v4/memory/model-policies", "POST", q, 201, nil)
		start := time.Now()
		err := f.process()
		o := observation{ID: tc.ID, Output: capture.output, Usage: capture.usage, Millis: time.Since(start).Milliseconds()}
		if o.Usage == nil {
			unknown++
		}
		canonical, valid := (candidates.Handler{}).Validate([]catalog.Source{src}, capture.output)
		if valid == nil {
			quotes++
			o.AcceptedCitation = true
			var result candidates.Result
			_ = json.Unmarshal([]byte(canonical), &result)
			if tc.Skip {
				o.SemanticMatch = len(result.Candidates) == 0
				if !o.SemanticMatch {
					unsafe++
				}
			} else if len(result.Candidates) == 1 {
				u := result.Candidates[0]
				o.SemanticMatch = u.Type == tc.Type && u.SpeechAct == tc.SpeechAct && u.Quote == tc.Body && ((tc.Scope == "" && u.ScopeQuote == nil) || (u.ScopeQuote != nil && strings.Contains(*u.ScopeQuote, tc.Scope))) && ((tc.Time == "" && u.ValidTimeQuote == nil) || (u.ValidTimeQuote != nil && strings.Contains(*u.ValidTimeQuote, tc.Time)))
			}
		}
		if o.SemanticMatch {
			semantic++
		}
		if err != nil {
			o.Reason = "worker_rejected"
		}
		results = append(results, o)
		t.Logf("native candidate %s citation=%t semantic=%t elapsed_ms=%d", tc.ID, o.AcceptedCitation, o.SemanticMatch, o.Millis)
		if err != nil && capture.output == "" {
			t.Error("native provider failed; remaining cases deferred")
			break
		}
	}
}
