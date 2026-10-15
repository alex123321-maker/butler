package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type candidateProvider struct {
	transport.ModelProvider
	calls    atomic.Int32
	source   catalog.Source
	callback func()
	output   string
	inputs   []transport.InputItem
}

func (*candidateProvider) Name() string                    { return "antigravity" }
func (*candidateProvider) InferenceLimits() (int64, int64) { return 128, 64 }
func (*candidateProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *candidateProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	p.inputs = r.InputItems
	if e := transport.CheckInferenceAuthority(ctx); e != nil {
		return nil, e
	}
	if e := transport.CheckInferenceDispatch(ctx); e != nil {
		return nil, e
	}
	p.calls.Add(1)
	if len(r.ToolDefinitions) != 0 || r.Context.ProviderSessionRef != nil {
		return nil, jobs.ErrClaim
	}
	if p.callback != nil {
		p.callback()
	}
	out := p.output
	if out == "" {
		b, _ := json.Marshal(candidates.Output{Units: []candidates.Unit{{SourceID: p.source.ID, Type: "explicit_preference", SpeechAct: "assertion", Key: "preference.language", Quote: p.source.Body}}, Skips: []candidates.Skip{}})
		out = string(b)
	}
	u := &transport.Usage{InputTokens: 20, OutputTokens: 12, TotalTokens: 32}
	ch := make(chan transport.TransportEvent, 1)
	ch <- transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: out, Usage: u})
	close(ch)
	return ch, nil
}

type candidateFixture struct {
	t       *testing.T
	pool    *pgxpool.Pool
	catalog *catalog.Store
	jobs    *jobs.Store
	budgets *budget.Controller
	worker  *jobs.ModelWorker
	server  *httptest.Server
	owner   context.Context
	serial  int
}

func newCandidateFixture(t *testing.T) *candidateFixture {
	t.Helper()
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if e := m.Initialize(); e != nil {
		t.Fatal(e)
	}
	c := catalog.New(pool, m)
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	b.Policy.Models["antigravity:fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	b.Policy.ModelMillis = 1000
	j := jobs.New(pool, c, b, nil)
	w := &jobs.ModelWorker{Jobs: j, Provider: &candidateProvider{}, Model: "fixture", Handlers: map[string]jobs.ModelHandler{candidates.Version: candidates.Handler{}}}
	s := httptest.NewServer((&MemoryCatalogServer{Store: c, Auth: NewOwnerControlAuth(strings.Repeat("c", 40), "owner"), ModelJobs: j, ModelWorkerConfigured: true}).Handle())
	t.Cleanup(s.Close)
	return &candidateFixture{t: t, pool: pool, catalog: c, jobs: j, budgets: b, worker: w, server: s, owner: owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "fixture-owner"})}
}
func (f *candidateFixture) request(path, method string, value any, status int, out any) {
	f.t.Helper()
	var body []byte
	if value != nil {
		body, _ = json.Marshal(value)
	}
	r, e := http.NewRequest(method, f.server.URL+path, bytes.NewReader(body))
	if e != nil {
		f.t.Fatal(e)
	}
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("c", 40))
	f.serial++
	r.Header.Set("X-Butler-Delivery-ID", fmt.Sprintf("candidate-%d", f.serial))
	resp, e := f.server.Client().Do(r)
	if e != nil {
		f.t.Fatal(e)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != status {
		f.t.Fatalf("%s %d: %s", path, resp.StatusCode, b)
	}
	if out != nil && json.Unmarshal(b, out) != nil {
		f.t.Fatalf("invalid response: %s", b)
	}
}
func (f *candidateFixture) source(key, body string) catalog.Source {
	var s catalog.Source
	f.request("/api/v4/memory/sources", "POST", catalog.PutSource{ExternalKey: key, Kind: "owner_statement", ScopeType: "user", ScopeID: "owner", Body: body}, 200, &s)
	return s
}
func (f *candidateFixture) policy(id string, s catalog.Source, privacy string) jobs.ModelPolicyRequest {
	return f.policyWithRun(id, s, privacy, "")
}
func (f *candidateFixture) policyWithRun(id string, s catalog.Source, privacy, run string) jobs.ModelPolicyRequest {
	q := jobs.ModelPolicyRequest{ID: id, Handler: candidates.Version, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 2, Privacy: privacy, Job: jobs.Request{Sources: []catalog.SourceRef{{ID: s.ID, Revision: s.Revision, AccessRevision: s.AccessRevision, Hash: s.Hash}}, Allowance: budget.Units{ModelTurns: 4, Tokens: 10000, ActiveMillis: 10000, ProviderInputBytes: 4 * 8388608}, Deadline: time.Now().Add(time.Hour).UTC().Truncate(time.Microsecond)}}
	q.Job.OriginalRunID = run
	f.request("/api/v4/memory/model-policies", "POST", q, 201, nil)
	return q
}

func TestCandidateTranscriptAndParsedSourcePreserveAncestorPrivacyIntegration(t *testing.T) {
	for _, kind := range []string{"transcript", "parsed_document"} {
		t.Run(kind, func(t *testing.T) {
			f := newCandidateFixture(t)
			ctx := context.Background()
			_, err := f.pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('candidate-session','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('candidate-run','candidate-session','event','completed','mode_1','completed','fixture',clock_timestamp(),clock_timestamp()); INSERT INTO messages(message_id,session_key,run_id,role,content) VALUES('earlier-user','candidate-session','candidate-run','user','EARLIER_PRIVATE_INPUT'),('earlier-answer','candidate-session','candidate-run','assistant','EARLIER_PRIVATE_ANSWER'),('preference-user','candidate-session','candidate-run','user','Предпочитаю ответы на русском языке.')`)
			if err != nil {
				t.Fatal(err)
			}
			// Older migrated transcript sources legitimately retain previous-event
			// privacy parents. Keep that graph rather than treating it as model input.
			_, err = f.pool.Exec(ctx, `INSERT INTO memory_source_dependencies(child_source_id,parent_source_id,parent_revision) VALUES('message:preference-user','message:earlier-answer',1)`)
			if err != nil {
				t.Fatal(err)
			}
			src, err := f.catalog.Get(f.owner, "message:preference-user")
			if err != nil {
				t.Fatal(err)
			}
			if kind == "parsed_document" {
				src, err = f.catalog.Put(f.owner, catalog.PutSource{ExternalKey: "parsed-original", Kind: "document", ScopeType: "session", ScopeID: "candidate-session", Body: src.Body, Parents: []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}})
				if err != nil {
					t.Fatal(err)
				}
			}
			p := f.worker.Provider.(*candidateProvider)
			p.source = src
			f.policyWithRun("candidate-transcript-policy", src, "cloud_allowed", "candidate-run")
			f.enable()
			if err = f.process(); err != nil {
				t.Fatal(err)
			}
			cs := f.claims()
			if len(cs) != 1 || cs[0].SourceID != src.ID || cs[0].ScopeType != "session" || cs[0].ScopeID != "candidate-session" {
				t.Fatal(cs)
			}
			if len(p.inputs) != 2 || strings.Contains(p.inputs[1].Content, "EARLIER_PRIVATE") {
				t.Fatal("ancestor text was released as extraction input", p.inputs)
			}
			var dependencies int
			if err = f.pool.QueryRow(ctx, `SELECT count(*) FROM memory_source_dependencies WHERE child_source_id=(SELECT result_source_id FROM memory_model_attempts WHERE state='published')`).Scan(&dependencies); err != nil || dependencies < 3 {
				t.Fatal("privacy manifest lost", dependencies, err)
			}
			f.request("/api/v4/memory/decisions", "POST", catalog.Decision{ClaimID: cs[0].ID}, 200, nil)
			var profileID int64
			_ = f.pool.QueryRow(ctx, `SELECT profile_id FROM memory_claims WHERE claim_id=$1`, cs[0].ID).Scan(&profileID)
			f.request("/api/v4/memory/correct", "POST", catalog.Correction{SourceID: "message:earlier-user", ExpectedRevision: 1, Body: "EARLIER_CORRECTED"}, 200, nil)
			if err = f.catalog.CheckObjects(f.owner, []catalog.ObjectRef{{Type: "profile", ID: profileID}}, "memory_read"); err == nil {
				t.Fatal("ancestor correction failed to invalidate accepted claim")
			}
		})
	}
}

func TestCandidateNoModelSupervisorTimelineIntegration(t *testing.T) {
	if os.Getenv("BUTLER_CANDIDATE_SUPERVISOR_DIAG") != "true" {
		t.Skip("explicit bounded no-model supervisor diagnosis")
	}
	f := newCandidateFixture(t)
	f.budgets.Policy.ModelMillis = 90000
	f.enable()
	p := f.worker.Provider.(*candidateProvider)
	for i, delay := range []time.Duration{11 * time.Second, 16 * time.Second, 10 * time.Second} {
		src := f.source(fmt.Sprintf("supervisor-%d", i), "Предпочитаю ответы на русском языке.")
		p.source = src
		p.callback = func() { time.Sleep(delay) }
		q := jobs.ModelPolicyRequest{ID: fmt.Sprintf("candidate-supervisor-policy-%d", i), Handler: candidates.Version, Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Job: jobs.Request{Sources: []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}, Allowance: budget.Units{ModelTurns: 1, Tokens: 10000, ActiveMillis: 90000, ProviderInputBytes: 8388608}, Deadline: time.Now().Add(time.Hour)}}
		f.request("/api/v4/memory/model-policies", "POST", q, 201, nil)
		// Match the native90s quote while keeping this a controlled provider with
		// zero internet calls and an independently bounded persistent job.
		start := time.Now()
		e := f.process()
		stats := f.pool.Stat()
		t.Logf("probe timeline i=%d elapsed_ms=%d error_type=%T acquired=%d idle=%d total=%d", i, time.Since(start).Milliseconds(), e, stats.AcquiredConns(), stats.IdleConns(), stats.TotalConns())
		if e != nil {
			t.Fatal("no-model supervisor rejected a current source")
		}
	}
}
func (f *candidateFixture) process() error {
	f.t.Helper()
	if e := f.jobs.PollModelPolicies(context.Background(), func(s string) bool { return f.worker.Handlers[s] != nil }); e != nil {
		f.t.Fatal(e)
	}
	worked, e := f.worker.ProcessOne(context.Background())
	if !worked && e == nil {
		f.t.Fatal("no composed worker work")
	}
	return e
}
func (f *candidateFixture) claims() []catalog.Claim {
	var v struct {
		Claims []catalog.Claim `json:"claims"`
	}
	f.request("/api/v4/memory", "GET", nil, 200, &v)
	return v.Claims
}
func (f *candidateFixture) enable() {
	f.request("/api/v4/memory/model-worker-control", "POST", map[string]string{"state": "enabled"}, 200, nil)
}

func TestCandidateHTTPWorkerOwnerDecisionCorrectionIntegration(t *testing.T) {
	f := newCandidateFixture(t)
	s := f.source("language", "Предпочитаю ответы на русском языке.")
	p := f.worker.Provider.(*candidateProvider)
	p.source = s
	q := f.policy("candidate-language-policy", s, "cloud_allowed")
	f.enable()
	if e := f.process(); e != nil {
		t.Fatal(e)
	}
	claims := f.claims()
	if len(claims) != 1 || claims[0].Status != "candidate" || claims[0].Confidence != 0 || claims[0].ExtractedAt.IsZero() {
		t.Fatal(claims)
	}
	c := claims[0]
	var evidence candidates.Evidence
	if json.Unmarshal(c.Value, &evidence) != nil || evidence.SourceHash != s.Hash || evidence.SourceRevision != s.Revision || evidence.ScopeType != "user" || evidence.ScopeID != "owner" || evidence.EventTime != nil {
		t.Fatal(string(c.Value))
	}
	var profiles int
	if e := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM memory_profile WHERE confirmation_state='confirmed'`).Scan(&profiles); e != nil || profiles != 0 {
		t.Fatal(profiles, e)
	}
	f.request("/api/v4/memory/decisions", "POST", catalog.Decision{ClaimID: c.ID}, 200, &c)
	if c.Status != "confirmed" {
		t.Fatal(c)
	}
	var profileID int64
	if e := f.pool.QueryRow(context.Background(), `SELECT profile_id FROM memory_claims WHERE claim_id=$1`, c.ID).Scan(&profileID); e != nil {
		t.Fatal(e)
	}
	if e := f.catalog.CheckObjects(f.owner, []catalog.ObjectRef{{Type: "profile", ID: profileID}}, "memory_read"); e != nil {
		t.Fatal("confirmed source unavailable", e)
	}
	// A repeated owner request/event and a restarted worker retain the decision.
	f.request("/api/v4/memory/model-policies", "POST", q, 201, nil)
	f.worker = &jobs.ModelWorker{Jobs: f.jobs, Provider: p, Model: "fixture", Handlers: map[string]jobs.ModelHandler{candidates.Version: candidates.Handler{}}}
	if e := f.jobs.PollModelPolicies(context.Background(), func(string) bool { return true }); e != nil {
		t.Fatal(e)
	}
	worked, e := f.worker.ProcessOne(context.Background())
	if e != nil || worked || p.calls.Load() != 1 || len(f.claims()) != 1 || f.claims()[0].Status != "confirmed" {
		t.Fatal(worked, e, p.calls.Load(), f.claims())
	}
	// A distinct policy cannot reset acceptance or create a duplicate proposal.
	f.policy("candidate-repeat-policy", s, "cloud_allowed")
	if e := f.process(); e != nil {
		t.Fatal(e)
	}
	if got := f.claims(); len(got) != 1 || got[0].ID != c.ID || got[0].Status != "confirmed" {
		t.Fatal(got)
	}
	// Model wording/topic variation on the exact same evidence also cannot mint
	// another candidate or reset the already accepted owner decision.
	b, _ := json.Marshal(candidates.Output{Units: []candidates.Unit{{SourceID: s.ID, Type: "explicit_preference", SpeechAct: "assertion", Key: "preference.response_language", Quote: s.Body}}, Skips: []candidates.Skip{}})
	p.output = string(b)
	f.policy("candidate-key-change-policy", s, "cloud_allowed")
	if e := f.process(); e == nil {
		t.Fatal("changed exact-evidence retry accepted")
	}
	if got := f.claims(); len(got) != 1 || got[0].ID != c.ID || got[0].Status != "confirmed" {
		t.Fatal(got)
	}
	f.request("/api/v4/memory/correct", "POST", catalog.Correction{SourceID: s.ID, ExpectedRevision: s.Revision, Body: "Предпочитаю ответы на английском языке."}, 200, nil)
	if e := f.catalog.CheckObjects(f.owner, []catalog.ObjectRef{{Type: "profile", ID: profileID}}, "memory_read"); e == nil {
		t.Fatal("corrected source remains context eligible")
	}
	var observed, reserved int
	if e := f.pool.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE usage IS NOT NULL),(SELECT count(*) FROM execution_budget_reservations) FROM memory_model_attempts`).Scan(&observed, &reserved); e != nil || observed != 3 || reserved != 3 {
		t.Fatal(observed, reserved, e)
	}
}

func TestCandidateSeparateOwnerLifecycleStageIntegration(t *testing.T) {
	f := newCandidateFixture(t)
	src := f.source("owner-lifecycle", "Предпочитаю подробные объяснения технических решений.")
	f.worker.Provider.(*candidateProvider).source = src
	f.policy("owner-lifecycle-policy", src, "cloud_allowed")
	f.enable()
	if err := f.process(); err != nil {
		t.Fatal(err)
	}
	claims := f.claims()
	if len(claims) != 1 {
		t.Fatal(claims)
	}
	candidateOwnerLifecycle(t, f, src, claims[0])
}
func TestCandidateLateSourceChangeAndLocalOnlyIntegration(t *testing.T) {
	for _, change := range []string{"correct", "delete", "local_only", "invented", "incomplete"} {
		t.Run(change, func(t *testing.T) {
			f := newCandidateFixture(t)
			s := f.source("source", "Не сокращай ответы, кроме статусов.")
			p := f.worker.Provider.(*candidateProvider)
			p.source = s
			privacy := "cloud_allowed"
			if change == "local_only" {
				privacy = "local_only"
			}
			f.policy("candidate-change-policy", s, privacy)
			f.enable()
			if change == "correct" || change == "delete" {
				p.callback = func() {
					if change == "correct" {
						f.request("/api/v4/memory/correct", "POST", catalog.Correction{SourceID: s.ID, ExpectedRevision: 1, Body: "Исправлено."}, 200, nil)
					} else {
						f.request("/api/v4/memory/delete", "POST", map[string]any{"source_id": s.ID, "expected_revision": 1, "action": "delete"}, 200, nil)
					}
				}
			}
			if change == "invented" || change == "incomplete" {
				quote := "Сокращай ответы."
				if change == "incomplete" {
					quote = "Не сокращай ответы"
				}
				b, _ := json.Marshal(candidates.Output{Units: []candidates.Unit{{SourceID: s.ID, Type: "explicit_preference", SpeechAct: "assertion", Key: "preference.detail", Quote: quote}}, Skips: []candidates.Skip{}})
				p.output = string(b)
			}
			if e := f.process(); e == nil {
				t.Fatal("unsafe publication succeeded")
			}
			if len(f.claims()) != 0 {
				t.Fatal("late or invented candidate")
			}
			if change == "local_only" {
				var count int
				if e := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM memory_model_attempts`).Scan(&count); e != nil || count != 0 || p.calls.Load() != 0 {
					t.Fatal("cloud fallback", count, p.calls.Load(), e)
				}
				var reason string
				_ = f.pool.QueryRow(context.Background(), `SELECT error_code FROM memory_aux_jobs`).Scan(&reason)
				if reason != "local_backend_unavailable" {
					t.Fatal(reason)
				}
			}
		})
	}
}

func TestCandidateEpisodeOwnerProjectionAndRevokeIntegration(t *testing.T) {
	f := newCandidateFixture(t)
	src := f.source("episode", "После отката тестовый сервис снова заработал.")
	p := f.worker.Provider.(*candidateProvider)
	p.source = src
	b, _ := json.Marshal(candidates.Output{Units: []candidates.Unit{{SourceID: src.ID, Type: "outcome_episode", SpeechAct: "assertion", Key: "episode.rollback", Quote: src.Body}}, Skips: []candidates.Skip{}})
	p.output = string(b)
	f.policy("candidate-episode-policy", src, "cloud_allowed")
	f.enable()
	if e := f.process(); e != nil {
		t.Fatal(e)
	}
	cs := f.claims()
	if len(cs) != 1 || cs[0].MemoryType != "episodic" || cs[0].Status != "candidate" {
		t.Fatal(cs)
	}
	f.request("/api/v4/memory/decisions", "POST", catalog.Decision{ClaimID: cs[0].ID}, 200, nil)
	var episodeID int64
	var noProfile, noVector bool
	if e := f.pool.QueryRow(context.Background(), `SELECT c.episode_id,c.profile_id IS NULL,e.embedding IS NULL FROM memory_claims c JOIN memory_episodes e ON e.id=c.episode_id WHERE c.claim_id=$1 AND e.confirmation_state='confirmed' AND e.scope_type='user' AND e.scope_id='owner'`, cs[0].ID).Scan(&episodeID, &noProfile, &noVector); e != nil || !noProfile || !noVector {
		t.Fatal(episodeID, noProfile, noVector, e)
	}
	if e := f.catalog.CheckObjects(f.owner, []catalog.ObjectRef{{Type: "episodic", ID: episodeID}}, "memory_read"); e != nil {
		t.Fatal(e)
	}
	f.request("/api/v4/memory/delete", "POST", map[string]any{"source_id": src.ID, "expected_revision": 1, "action": "revoke"}, 200, nil)
	if e := f.catalog.CheckObjects(f.owner, []catalog.ObjectRef{{Type: "episodic", ID: episodeID}}, "memory_read"); e == nil {
		t.Fatal("revoked episode remains eligible")
	}
}

type failingCandidatePublisher struct{ candidates.Handler }

func (failingCandidatePublisher) PublishTx(ctx context.Context, tx pgx.Tx, c *catalog.Store, j jobs.Job, a jobs.ModelAttempt, sources []catalog.Source, result catalog.Source, body string) error {
	if e := (candidates.Handler{}).PublishTx(ctx, tx, c, j, a, sources, result, body); e != nil {
		return e
	}
	return jobs.ErrClaim
}
func TestCandidateAtomicConsumerRollbackIntegration(t *testing.T) {
	f := newCandidateFixture(t)
	src := f.source("rollback", "Предпочитаю ответы на русском языке.")
	f.worker.Provider.(*candidateProvider).source = src
	f.worker.Handlers[candidates.Version] = failingCandidatePublisher{}
	f.policy("candidate-rollback-policy", src, "cloud_allowed")
	f.enable()
	if e := f.process(); e == nil {
		t.Fatal("publication hook failure ignored")
	}
	var count int
	if e := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM memory_claims)+(SELECT count(*) FROM memory_sources WHERE source_kind='model_candidate')`).Scan(&count); e != nil || count != 0 {
		t.Fatal("partial materialization", count, e)
	}
	if e := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM memory_model_attempts WHERE usage IS NOT NULL AND state='failed'`).Scan(&count); e != nil || count != 1 {
		t.Fatal("paid observation erased", count, e)
	}
}

func TestCandidateLifecyclePoisonRemainsDataAndDeletionSelectiveIntegration(t *testing.T) {
	for _, attack := range memorytest.LifecycleAttacks(t) {
		t.Run(attack.ID, func(t *testing.T) {
			f := newCandidateFixture(t)
			good := f.source("legitimate", "Предпочитаю ответы на русском языке.")
			p := f.worker.Provider.(*candidateProvider)
			p.source = good
			f.policy("candidate-legitimate-policy", good, "cloud_allowed")
			f.enable()
			if e := f.process(); e != nil {
				t.Fatal(e)
			}
			accepted := f.claims()[0]
			f.request("/api/v4/memory/decisions", "POST", catalog.Decision{ClaimID: accepted.ID}, 200, nil)
			poison := f.source("poison", attack.Body)
			p.source = poison
			f.policy("candidate-poisoned-policy", poison, "cloud_allowed")
			if e := f.process(); e != nil {
				t.Fatal(e)
			}
			cs := f.claims()
			var malicious catalog.Claim
			for _, c := range cs {
				if c.SourceID == poison.ID {
					malicious = c
				}
			}
			if malicious.ID == "" || malicious.Status != "candidate" || malicious.Confidence != 0 || malicious.ScopeID != "owner" {
				t.Fatal("poison promoted", cs)
			}
			var authorities int
			if e := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM tasks)+(SELECT count(*) FROM action_mandates)+(SELECT count(*) FROM approvals)`).Scan(&authorities); e != nil || authorities != 0 {
				t.Fatal("DATA granted authority", authorities, e)
			}
			f.request("/api/v4/memory/delete", "POST", map[string]any{"source_id": poison.ID, "expected_revision": 1, "action": "delete"}, 200, nil)
			var v struct {
				Claims []catalog.Claim `json:"claims"`
			}
			f.request("/api/v4/memory", "GET", nil, 200, &v)
			for _, c := range v.Claims {
				if c.ID == accepted.ID && c.Status != "confirmed" {
					t.Fatal("unrelated owner decision removed")
				}
				if c.ID == malicious.ID && (c.Status != "deleted" || c.Summary != "" || string(c.Value) != "{}") {
					t.Fatal("poison text survived", c)
				}
			}
		})
	}
}
