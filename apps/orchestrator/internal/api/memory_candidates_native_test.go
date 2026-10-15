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
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

type candidateCaptureProvider struct {
	transport.ModelProvider
	mu   sync.Mutex
	last candidateNativeTransport
}

type candidateNativeTransport struct {
	Output       string                               `json:"output"`
	Usage        *transport.Usage                     `json:"usage"`
	Stages       []transport.ProviderStage            `json:"provider_stages"`
	Auth         []providerauth.AntigravityDiagnostic `json:"auth_diagnostics"`
	ErrorCode    string                               `json:"provider_error_code,omitempty"`
	HTTPStatus   int                                  `json:"provider_http_status,omitempty"`
	SafeReason   string                               `json:"provider_safe_reason,omitempty"`
	FinishReason string                               `json:"finish_reason,omitempty"`
}

func (p *candidateCaptureProvider) snapshot() candidateNativeTransport {
	p.mu.Lock()
	defer p.mu.Unlock()
	r := p.last
	r.Stages = append([]transport.ProviderStage(nil), r.Stages...)
	r.Auth = append([]providerauth.AntigravityDiagnostic(nil), r.Auth...)
	if r.Usage != nil {
		u := *r.Usage
		r.Usage = &u
	}
	return r
}

// This separate owner-controlled stage consumes an actual native result only
// after the extraction screen has finished. It makes no model/provider request.
func TestCandidateNativeOwnerLifecycle(t *testing.T) {
	proof := os.Getenv("BUTLER_CANDIDATE_NATIVE_EVIDENCE")
	if proof == "" {
		t.Skip("completed native quality window required")
	}
	var quality struct {
		Passed bool `json:"quality_gate_passed"`
	}
	raw, err := os.ReadFile(filepath.Join(proof, "quality.json"))
	if err != nil || json.Unmarshal(raw, &quality) != nil || !quality.Passed {
		t.Fatal("native owner stage requires completed quality PASS")
	}
	started, err := os.OpenFile(filepath.Join(proof, "owner-lifecycle.started"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("one-use owner lifecycle stage required")
	}
	started.Close()
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_CANDIDATE_NATIVE_POSTGRES_URL"))
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("separate persistent native database required")
	}
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	c := catalog.New(pool, catalog.NewFileManifest(filepath.Join(filepath.Dir(proof), "native-v2", "deletions.log")))
	srv := httptest.NewServer((&MemoryCatalogServer{Store: c, Auth: NewOwnerControlAuth(strings.Repeat("c", 40), "owner")}).Handle())
	defer srv.Close()
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "native-owner-proof"})
	f := &candidateFixture{t: t, pool: pool, catalog: c, server: srv, owner: ctx}
	var sourceID string
	if err = pool.QueryRow(ctx, `SELECT source_id FROM memory_sources WHERE external_key=$1 AND owner_id='owner' AND status='active'`, filepath.Base(proof)+":explicit-detail").Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	src, err := c.Get(ctx, sourceID)
	if err != nil {
		t.Fatal(err)
	}
	var claim catalog.Claim
	for _, got := range f.claims() {
		if got.SourceID == src.ID && got.Status == "candidate" {
			claim = got
		}
	}
	if claim.ID == "" {
		t.Fatal("actual native candidate unavailable")
	}
	evidence := candidateOwnerLifecycle(t, f, src, claim)
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil || os.WriteFile(filepath.Join(proof, "owner-lifecycle.json"), data, 0600) != nil {
		t.Fatal("owner lifecycle evidence unavailable")
	}
}

func candidateOwnerLifecycle(t *testing.T, f *candidateFixture, src catalog.Source, claim catalog.Claim) map[string]any {
	t.Helper()
	ctx := context.Background()
	var before, after int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM memory_model_attempts`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	f.request("/api/v4/memory/decisions", "POST", catalog.Decision{ClaimID: claim.ID}, 200, &claim)
	if claim.Status != "confirmed" {
		t.Fatal("owner acceptance did not confirm native candidate")
	}
	var profileID int64
	if err := f.pool.QueryRow(ctx, `SELECT profile_id FROM memory_claims WHERE claim_id=$1`, claim.ID).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	refs := []catalog.ObjectRef{{Type: "profile", ID: profileID}}
	if err := f.catalog.CheckObjects(f.owner, refs, "memory_read"); err != nil {
		t.Fatal("owner-confirmed native evidence unavailable", err)
	}
	var corrected catalog.Source
	f.request("/api/v4/memory/correct", "POST", catalog.Correction{SourceID: src.ID, ExpectedRevision: src.Revision, Body: "Исправление владельца: прежнее предпочтение отменено."}, 200, &corrected)
	if err := f.catalog.CheckObjects(f.owner, refs, "memory_read"); err == nil {
		t.Fatal("corrected native evidence still eligible")
	}
	f.request("/api/v4/memory/delete", "POST", map[string]any{"source_id": src.ID, "action": "revoke", "expected_revision": corrected.Revision}, 200, nil)
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM memory_model_attempts`).Scan(&after); err != nil || after != before {
		t.Fatal("owner lifecycle changed paid history", before, after, err)
	}
	return map[string]any{"native_claim_id": claim.ID, "source_id": src.ID, "explicit_owner_acceptance": true, "accepted_memory_eligible": true, "correction_blocks_old_memory": true, "explicit_owner_source_revocation": true, "additional_model_calls": 0, "paid_attempts_before": before, "paid_attempts_after": after, "automatic_acceptance": false}
}

func (p *candidateCaptureProvider) InferenceLimits() (int64, int64) {
	return p.ModelProvider.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
}
func (p *candidateCaptureProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	p.mu.Lock()
	p.last = candidateNativeTransport{}
	p.mu.Unlock()
	ctx = transport.WithProviderStageObserver(ctx, func(stage transport.ProviderStage) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if len(p.last.Stages) < 32 {
			p.last.Stages = append(p.last.Stages, stage)
		}
	})
	ctx = providerauth.WithAntigravityDiagnostics(ctx, func(d providerauth.AntigravityDiagnostic) {
		p.mu.Lock()
		defer p.mu.Unlock()
		if len(p.last.Auth) < 32 {
			p.last.Auth = append(p.last.Auth, d)
		}
	})
	recordError := func(err error) {
		p.last.ErrorCode = transport.DiagnosticCode(err)
		p.last.HTTPStatus = transport.DiagnosticHTTPStatus(err)
		p.last.SafeReason = transport.DiagnosticReason(err)
	}
	s, e := p.ModelProvider.StartRun(ctx, r)
	if e != nil {
		p.mu.Lock()
		recordError(e)
		p.mu.Unlock()
		return nil, e
	}
	ch := make(chan transport.TransportEvent)
	go func() {
		defer close(ch)
		for event := range s {
			p.mu.Lock()
			if event.AssistantFinal != nil {
				p.last.Output = event.AssistantFinal.Content
				switch reason := strings.ToUpper(event.AssistantFinal.FinishReason); reason {
				case "STOP", "MAX_TOKENS", "SAFETY", "RECITATION", "OTHER":
					p.last.FinishReason = reason
				default:
					p.last.FinishReason = "unknown"
				}
			}
			if event.Usage != nil {
				copy := *event.Usage
				p.last.Usage = &copy
			}
			if event.TransportError != nil {
				recordError(event.TransportError)
			}
			p.mu.Unlock()
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

type candidateDiagnosticProvider struct {
	transport.ModelProvider
	final bool
}

func (p candidateDiagnosticProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	transport.ObserveProviderStage(ctx, transport.ProviderStage("PRIVATE_STAGE"))
	transport.ObserveProviderStage(ctx, transport.ProviderStageAuthentication)
	ch := make(chan transport.TransportEvent, 1)
	if p.final {
		ch <- transport.NewAssistantFinalEvent(r.Context.RunID, "antigravity", transport.AssistantFinal{Content: "{}", FinishReason: "PRIVATE_FINISH", Usage: &transport.Usage{InputTokens: 1, OutputTokens: 1, TotalTokens: 2}})
	} else {
		ch <- transport.NewTransportErrorEvent(r.Context.RunID, "antigravity", &transport.Error{Type: transport.ErrorTypeProviderUnavailable, HTTPStatus: 403, Message: "PRIVATE_MESSAGE", ProviderCode: "PRIVATE_CODE", SafeReason: "PRIVATE_REASON", ProviderDetails: map[string]any{"token": "PRIVATE_TOKEN"}})
	}
	close(ch)
	return ch, nil
}

func TestCandidateNativeDiagnosticCaptureExcludesPrivateProviderDetails(t *testing.T) {
	for _, final := range []bool{false, true} {
		p := &candidateCaptureProvider{ModelProvider: candidateDiagnosticProvider{final: final}}
		stream, err := p.StartRun(context.Background(), transport.StartRunRequest{})
		if err != nil {
			t.Fatal(err)
		}
		for range stream {
		}
		snapshot := p.snapshot()
		b, err := json.Marshal(snapshot)
		if err != nil || strings.Contains(string(b), "PRIVATE") || len(snapshot.Stages) != 1 || snapshot.Stages[0] != transport.ProviderStageAuthentication {
			t.Fatal("private diagnostic detail escaped", err)
		}
		if final && snapshot.FinishReason != "unknown" || !final && (snapshot.ErrorCode != "provider_unavailable" || snapshot.HTTPStatus != 403 || snapshot.SafeReason != "") {
			t.Fatal("fixed diagnostic projection unavailable")
		}
	}
}

func TestCandidateHigh32768ProfileReservesCompleteFiniteQuoteIntegration(t *testing.T) {
	f := newCandidateFixture(t)
	f.budgets.Policy.ModelMillis = 300000
	f.budgets.Policy.OutputTokens = 32768
	f.budgets.Policy.Models["antigravity:"+antigravity.FlashHighModel] = budget.ModelPrice{Basis: "subscription_no_overage"}
	p, err := antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, MaximumOutputTokens: 32768, Timeout: 300 * time.Second, AuthSource: providerauth.NewManager(config.NewPostgresSettingsStore(f.pool))})
	if err != nil {
		t.Fatal(err)
	}
	f.worker.Provider, f.worker.Model = p, antigravity.FlashHighModel
	src := f.source("high-profile-preflight", "Предпочитаю ответы на русском языке.")
	q := jobs.ModelPolicyRequest{ID: "high-profile-preflight", Handler: candidates.Version, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Job: jobs.Request{Sources: []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}, Allowance: budget.Units{ModelTurns: 1, Tokens: 1081344, ActiveMillis: 300000, ProviderInputBytes: 8388608}, Deadline: time.Now().Add(time.Hour)}}
	f.request("/api/v4/memory/model-policies", "POST", q, 201, nil)
	f.enable()
	if err = f.jobs.PollModelPolicies(context.Background(), func(string) bool { return true }); err != nil {
		t.Fatal(err)
	}
	j, ok, err := f.jobs.ClaimPurpose(context.Background(), "model", candidates.Version)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	a, _, err := f.worker.Admit(context.Background(), j, candidates.Handler{})
	if err != nil || a.Quote.Tokens != 1081344 || a.Quote.ActiveMillis != 300000 || a.Quote.ProviderInputBytes != 8388608 || time.Until(a.Deadline) < 290*time.Second || time.Until(*j.ClaimUntil) < 350*time.Second {
		t.Fatal("native profile does not fit its durable quote/deadline", a, err)
	}
	// This is admission only. No StartRun, auth resolution or HTTP is executed.
}

func candidateRevokePastPolicies(f *candidateFixture) {
	f.t.Helper()
	rows, err := f.pool.Query(context.Background(), `SELECT policy_id FROM memory_model_policies WHERE state<>'revoked' ORDER BY policy_id LIMIT 256`)
	if err != nil {
		f.t.Fatal(err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			f.t.Fatal(err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		f.t.Fatal(err)
	}
	for _, id := range ids {
		f.request("/api/v4/memory/model-policy-control", "POST", map[string]string{"policy_id": id, "state": "revoked"}, 200, nil)
	}
}

func TestCandidatePriorPolicyRevocationRetainsAttemptAndPreventsReplayIntegration(t *testing.T) {
	f := newCandidateFixture(t)
	src := f.source("prior-window", "Предпочитаю ответы на русском языке.")
	p := f.worker.Provider.(*candidateProvider)
	p.source = src
	f.policy("prior-window", src, "cloud_allowed")
	f.enable()
	if err := f.process(); err != nil {
		t.Fatal(err)
	}
	candidateRevokePastPolicies(f)
	if err := f.jobs.PollModelPolicies(context.Background(), func(string) bool { return true }); err != nil {
		t.Fatal(err)
	}
	worked, err := f.worker.ProcessOne(context.Background())
	var attempts int
	var state string
	if qerr := f.pool.QueryRow(context.Background(), `SELECT state,(SELECT count(*) FROM memory_model_attempts) FROM memory_model_policies WHERE policy_id='prior-window'`).Scan(&state, &attempts); qerr != nil {
		t.Fatal(qerr)
	}
	if err != nil || worked || p.calls.Load() != 1 || attempts != 1 || state != "revoked" {
		t.Fatal("prior window replayed or paid history changed", worked, err, p.calls.Load(), attempts, state)
	}
}

type candidateNativeCase struct {
	ID, Body, Type, Scope, Time string
	SpeechAct                   string `json:"speech_act"`
	Skip                        bool
}

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
		Cases    []candidateNativeCase
	}
	if json.Unmarshal(raw, &suite) != nil || len(suite.Cases) != 24 {
		t.Fatal("fixed native corpus unavailable")
	}
	holdoutRaw, e := os.ReadFile("../../../../testdata/acceptance/ru-memory-candidates-holdout-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	var holdout struct {
		Version string `json:"version"`
		Cases   []candidateNativeCase
	}
	if json.Unmarshal(holdoutRaw, &holdout) != nil || len(holdout.Cases) != 8 {
		t.Fatal("fixed metadata holdout unavailable")
	}
	cases := append(append([]candidateNativeCase{}, suite.Cases...), holdout.Cases...)
	var caps struct {
		CasesSHA256   string    `json:"cases_sha256"`
		MaxCalls      int       `json:"max_calls"`
		ExpiresAt     time.Time `json:"expires_at"`
		Revision      string    `json:"revision"`
		StartCase     int       `json:"start_case"`
		PriorAttempts int       `json:"prior_attempts"`
		HoldoutSHA256 string    `json:"holdout_sha256"`
		Handler       string    `json:"handler"`
		OutputTokens  int64     `json:"maximum_output_tokens"`
		ModelMillis   int64     `json:"model_timeout_millis"`
	}
	b, e := os.ReadFile(filepath.Join(proof, "caps.json"))
	if e != nil || json.Unmarshal(b, &caps) != nil || caps.CasesSHA256 != candidateHash(raw) || caps.HoldoutSHA256 != candidateHash(holdoutRaw) || caps.Handler != candidates.Version || caps.StartCase != 0 || caps.PriorAttempts < 50 || caps.PriorAttempts > 128 || caps.MaxCalls != len(cases) || caps.OutputTokens != 32768 || caps.ModelMillis != 300000 || len(caps.Revision) != 40 || !time.Now().Before(caps.ExpiresAt) {
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
	var room bool
	e = pool.QueryRow(context.Background(), `SELECT deadline>$1
 AND (limits_json->>'model_turns')::bigint-(used_json->>'model_turns')::bigint >= $2
 AND (limits_json->>'tokens')::bigint-(used_json->>'tokens')::bigint >= $2*(1048576+$3::bigint)
 AND (limits_json->>'active_millis')::bigint-(used_json->>'active_millis')::bigint >= $2*$4::bigint
 AND (limits_json->>'provider_input_bytes')::bigint-(used_json->>'provider_input_bytes')::bigint >= $2*8388608::bigint
 FROM execution_budget_scopes WHERE kind='owner_day' AND owner_id='owner' AND subject_id=to_char(clock_timestamp() AT TIME ZONE 'UTC','YYYY-MM-DD')`, caps.ExpiresAt, int64(caps.MaxCalls), caps.OutputTokens, caps.ModelMillis).Scan(&room)
	if e != nil || !room {
		t.Fatal("prospective full window does not fit retained owner-day allowance")
	}
	manifestPath := filepath.Join(filepath.Dir(proof), "native-v2", "deletions.log")
	m := catalog.NewFileManifest(manifestPath)
	c := catalog.New(pool, m)
	policy := budget.DefaultPolicy()
	policy.ModelMillis = caps.ModelMillis
	policy.OutputTokens = caps.OutputTokens
	policy.Models["antigravity:"+antigravity.FlashHighModel] = budget.ModelPrice{Basis: "subscription_no_overage"}
	bctl := budget.NewController(pool, policy)
	bctl.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	j := jobs.New(pool, c, bctl, nil)
	p, e := antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, AuthSource: providerauth.NewManager(config.NewPostgresSettingsStore(pool)), Timeout: time.Duration(caps.ModelMillis) * time.Millisecond, MaximumOutputTokens: int(caps.OutputTokens)})
	if e != nil {
		t.Fatal("native provider unavailable")
	}
	capture := &candidateCaptureProvider{ModelProvider: p}
	w := &jobs.ModelWorker{Jobs: j, Provider: capture, Model: antigravity.FlashHighModel, Handlers: map[string]jobs.ModelHandler{candidates.Version: candidates.Handler{}}, ObserveProbe: trace.probe}
	srv := httptest.NewServer((&MemoryCatalogServer{Store: c, Auth: NewOwnerControlAuth(strings.Repeat("c", 40), "owner"), ModelJobs: j, ModelWorkerConfigured: true}).Handle())
	defer srv.Close()
	f := &candidateFixture{t: t, pool: pool, catalog: c, jobs: j, worker: w, server: srv}
	// Past identities are never automatically retried by a later window. Use the
	// existing owner controls; their original spend/attempts stay untouched.
	candidateRevokePastPolicies(f)
	f.enable()
	defer j.SetModelControl(context.Background(), "disabled")
	type observation struct {
		candidateNativeTransport
		ID               string `json:"id"`
		AcceptedCitation bool   `json:"accepted_citation"`
		SemanticMatch    bool   `json:"semantic_match"`
		Reason           string `json:"reason"`
		Millis           int64  `json:"millis"`
	}
	results := []observation{}
	semantic, unsafe, quotes, unknown := 0, 0, 0, 0
	coreSemantic, holdoutSemantic := 0, 0
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
		passed := err == nil && authorityWrites == 0 && len(results) == len(cases) && float64(coreSemantic)/float64(len(suite.Cases)) >= 0.95 && holdoutSemantic == len(holdout.Cases) && quotes == len(cases) && unsafe == 0 && unknown == 0
		write("observations.json", results)
		write("quality.json", map[string]any{"revision": caps.Revision, "handler": candidates.Version, "retained_prior_attempts": caps.PriorAttempts, "corpus": suite.Version, "corpus_sha256": candidateHash(raw), "holdout": holdout.Version, "holdout_sha256": candidateHash(holdoutRaw), "holdout_semantic_matches": holdoutSemantic, "holdout_semantic_criterion": 1, "core_semantic_matches": coreSemantic, "instruction_sha256": candidateHash([]byte((candidates.Handler{}).Instruction())), "criteria": suite.Criteria, "cases": len(results), "semantic_matches": semantic, "exact_citation_cases": quotes, "unsafe_promotions": unsafe, "authority_writes": authorityWrites, "unknown_usage": unknown, "quality_gate_passed": passed, "automatic_acceptance": false, "pricing_basis": "subscription_no_overage", "production_release_qualified": false, "background_and_foreground_reservation_ledger": "ledger.json", "unmeasured_costs": []string{"OAuth refresh network bytes", "actual HTTP egress bytes", "CPU and storage amortization", "subscription allocation"}})
		if !passed {
			t.Error("predefined native quality gate failed; production stays disabled")
		}
	}()
	for index, tc := range cases {
		if !time.Now().Before(caps.ExpiresAt) {
			t.Error("native window expired")
			break
		}
		window := filepath.Base(proof)
		src := f.source(window+":"+tc.ID, tc.Body)
		q := jobs.ModelPolicyRequest{ID: window + ":" + tc.ID, Handler: candidates.Version, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Job: jobs.Request{Sources: []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}, Allowance: budget.Units{ModelTurns: 1, Tokens: 1048576 + caps.OutputTokens, ActiveMillis: caps.ModelMillis, ProviderInputBytes: 8388608}, Deadline: caps.ExpiresAt}}
		f.request("/api/v4/memory/model-policies", "POST", q, 201, nil)
		start := time.Now()
		err := f.process()
		o := observation{candidateNativeTransport: capture.snapshot(), ID: tc.ID, Millis: time.Since(start).Milliseconds()}
		if o.Usage == nil {
			unknown++
		}
		canonical, valid := (candidates.Handler{}).Validate([]catalog.Source{src}, o.Output)
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
			if index < len(suite.Cases) {
				coreSemantic++
			} else {
				holdoutSemantic++
			}
		}
		if err != nil {
			o.Reason = "worker_rejected"
		}
		results = append(results, o)
		t.Logf("native candidate %s citation=%t semantic=%t elapsed_ms=%d", tc.ID, o.AcceptedCitation, o.SemanticMatch, o.Millis)
		if err != nil && o.Output == "" || providerauth.UnsafeAntigravityRefresh(o.Auth) {
			t.Error("native provider failed; remaining cases deferred")
			break
		}
	}
}
