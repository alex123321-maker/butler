package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"errors"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgconn"
)

type authorityFaultAuth struct {
	authSource
	after func()
}

func (a authorityFaultAuth) ResolveAntigravity(ctx context.Context) (providerauth.AntigravityAuth, error) {
	a.after()
	return a.authSource.ResolveAntigravity(ctx)
}

func TestMemoryJobActualNativeAdapterHTTPIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	m := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if e := m.Initialize(); e != nil {
		t.Fatal(e)
	}
	c := catalog.New(pool, m)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "fixture"})
	src, e := c.Put(ctx, catalog.PutSource{ExternalKey: "adapter-source", Kind: "document", ScopeType: "user", ScopeID: "owner", Title: "source", Body: "exact HTTP fixture"})
	if e != nil {
		t.Fatal(e)
	}
	var wire, checks, dispatch atomic.Int32
	output, _ := json.Marshal(map[string]string{"source_id": src.ID, "quote": src.Body})
	part, _ := json.Marshal(map[string]string{"text": string(output)})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wire.Add(1)
		if r.URL.Path != "/v1:streamGenerateContent" {
			t.Error("unexpected native path")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, frame(string(part), "STOP"))
	}))
	defer server.Close()
	target, _ := url.Parse(server.URL)
	p := newFixture(t, func(r *http.Request) (*http.Response, error) {
		copy := r.Clone(r.Context())
		u := *r.URL
		u.Scheme = target.Scheme
		u.Host = target.Host
		copy.URL = &u
		return http.DefaultTransport.RoundTrip(copy)
	})
	policy := budget.DefaultPolicy()
	policy.Models[Name+":"+DefaultModel] = budget.ModelPrice{Basis: "subscription_no_overage"}
	b := budget.NewController(pool, policy)
	s := jobs.New(pool, c, b, nil)
	if e = s.SetModelControl(ctx, "enabled"); e != nil {
		t.Fatal(e)
	}
	q := jobs.ModelPolicyRequest{ID: "native-adapter-policy", Handler: "source-quote-v1", Trigger: "source_changed", IntervalSeconds: 60, MaxJobs: 1, Privacy: "cloud_allowed", Job: jobs.Request{Sources: []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}, Allowance: budget.Units{ModelTurns: 2, Tokens: 4 * 1048576, ActiveMillis: 240000, ProviderInputBytes: 2 * 8388608}, Deadline: time.Now().Add(time.Hour)}}
	if _, e = s.CreateModelPolicy(ctx, q); e != nil {
		t.Fatal(e)
	}
	if e = s.PollModelPolicies(ctx, func(string) bool { return true }); e != nil {
		t.Fatal(e)
	}
	worker := &jobs.ModelWorker{Jobs: s, Provider: p, Model: DefaultModel, Handlers: map[string]jobs.ModelHandler{"source-quote-v1": jobs.QuoteProbe{}}}
	worked, e := worker.ProcessOne(ctx)
	if e != nil || !worked {
		t.Fatal(worked, e)
	}
	var state string
	var usage []byte
	if e = pool.QueryRow(ctx, `SELECT state,usage FROM memory_model_attempts`).Scan(&state, &usage); e != nil || state != "published" || len(usage) == 0 || wire.Load() != 1 {
		t.Fatal(state, string(usage), wire.Load(), e)
	}
	// Direct adapter regression: authority is probed twice; dispatch is called
	// exactly once at the real HTTP boundary, and denial sends no second request.
	calls := transport.WithInferenceAuthority(ctx, func(context.Context) error { checks.Add(1); return nil })
	calls = transport.WithInferenceDispatch(calls, func(context.Context) error { dispatch.Add(1); return fmt.Errorf("private test denial") })
	req := request("dispatch-denied")
	req.ToolDefinitions = nil
	stream, e := p.StartRun(calls, req)
	if e != nil {
		t.Fatal(e)
	}
	failed := false
	for _, event := range collect(t, stream) {
		if event.TransportError != nil {
			failed = true
		}
	}
	if !failed || wire.Load() != 1 || checks.Load() != 2 || dispatch.Load() != 1 {
		t.Fatal(failed, wire.Load(), checks.Load(), dispatch.Load())
	}
	// Inject one SQL40001 specifically between the early authority check and
	// actual native HTTP. Transport's public source error normalization must not
	// erase the owning worker's private transient-persistence cause.
	q.ID = "native-read-fault-policy"
	if _, e = s.CreateModelPolicy(ctx, q); e != nil {
		t.Fatal(e)
	}
	if e = s.PollModelPolicies(ctx, func(string) bool { return true }); e != nil {
		t.Fatal(e)
	}
	var injected atomic.Bool
	p.config.AuthSource = authorityFaultAuth{after: func() {
		if !injected.CompareAndSwap(false, true) {
			return
		}
		_, err := pool.Exec(ctx, `CREATE SEQUENCE issue82_read_fault;CREATE FUNCTION issue82_read_policy(p jsonb) RETURNS jsonb LANGUAGE plpgsql AS $$ BEGIN IF nextval('issue82_read_fault')=1 THEN RAISE EXCEPTION USING ERRCODE='40001',MESSAGE='injected read rollback'; END IF; RETURN p; END $$;ALTER TABLE memory_model_policies RENAME TO issue82_policy_backing;CREATE VIEW memory_model_policies AS SELECT policy_id,owner_id,issue82_read_policy(policy) AS policy,state,jobs_created,last_event,next_event_at,reason,created_at FROM issue82_policy_backing`)
		if err != nil {
			t.Error(err)
		}
	}}
	worked, e = worker.ProcessOne(ctx)
	var transient *pgconn.PgError
	if !worked || !errors.As(e, &transient) || transient.Code != "40001" || wire.Load() != 1 {
		t.Fatal("native guard lost SQL cause or sent HTTP", worked, e, wire.Load())
	}
	if _, e = pool.Exec(ctx, `UPDATE memory_aux_jobs SET next_attempt_at=clock_timestamp() WHERE status='pending'`); e != nil {
		t.Fatal(e)
	}
	if worked, e = worker.ProcessOne(ctx); e != nil || !worked || wire.Load() != 2 {
		t.Fatal("native transient recovery", worked, e, wire.Load())
	}
}
