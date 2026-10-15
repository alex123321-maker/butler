package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/candidates"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/profile"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/transport"
)

// This provider records the actual ordinary request after memory preparation.
// It proves context composition and storage boundaries, not native answer quality.
type scopedCandidateProvider struct {
	transport.ModelProvider
	source catalog.Source
	last   transport.StartRunRequest
}

func (*scopedCandidateProvider) Name() string                    { return "antigravity" }
func (*scopedCandidateProvider) InferenceLimits() (int64, int64) { return 65536, 512 }
func (*scopedCandidateProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsStreaming: true}, nil
}
func (*scopedCandidateProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *scopedCandidateProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	p.last = r
	output := "Наблюдение завершено."
	if strings.HasPrefix(r.Context.SessionKey, "memory:") {
		b, _ := json.Marshal(candidates.Output{Units: []candidates.Unit{{SourceID: p.source.ID, Type: "explicit_preference", SpeechAct: "assertion", Key: "preference.language", Quote: p.source.Body}}, Skips: []candidates.Skip{}})
		output = string(b)
	}
	ch := make(chan transport.TransportEvent, 1)
	ch <- transport.NewAssistantFinalEvent(r.Context.RunID, p.Name(), transport.AssistantFinal{Content: output, Usage: &transport.Usage{InputTokens: 20, OutputTokens: 12, TotalTokens: 32}})
	close(ch)
	return ch, nil
}

func TestCandidateAcceptedPreferenceScopedOrdinaryContextIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	anchor := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := anchor.Initialize(); err != nil {
		t.Fatal(err)
	}
	c := catalog.New(pool, anchor)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "scoped-owner"})
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES('scope-a','owner','web'),('scope-b','owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('scope-seed','scope-a','seed','completed','mode_1','completed','fixture',clock_timestamp(),clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	b := budget.NewController(pool, budget.DefaultPolicy())
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	b.Policy.Models["antigravity:scope-fixture"] = budget.ModelPrice{Basis: "subscription_no_overage"}
	b.Policy.ModelMillis = 10000
	j := jobs.New(pool, c, b, nil)
	p := &scopedCandidateProvider{}
	w := &jobs.ModelWorker{Jobs: j, Provider: p, Model: "scope-fixture", Handlers: map[string]jobs.ModelHandler{candidates.Version: candidates.Handler{}}}
	server := httptest.NewServer((&api.MemoryCatalogServer{Store: c, Auth: api.NewOwnerControlAuth(strings.Repeat("s", 40), "owner"), ModelJobs: j, ModelWorkerConfigured: true}).Handle())
	defer server.Close()
	serial := 0
	request := func(path string, value, out any) {
		t.Helper()
		raw, _ := json.Marshal(value)
		req, err := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		serial++
		req.Header.Set("Authorization", "Bearer "+strings.Repeat("s", 40))
		req.Header.Set("X-Butler-Delivery-ID", fmt.Sprintf("scoped-%d", serial))
		resp, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			t.Fatalf("%s: status %d", path, resp.StatusCode)
		}
		if out != nil && json.Unmarshal(body, out) != nil {
			t.Fatal("invalid owner response")
		}
	}
	var src catalog.Source
	const preference = "Предпочитаю ответы на русском языке в этой беседе."
	request("/api/v4/memory/sources", catalog.PutSource{ExternalKey: "scoped-preference", Kind: "owner_statement", ScopeType: "session", ScopeID: "scope-a", Body: preference}, &src)
	p.source = src
	q := jobs.ModelPolicyRequest{ID: "scoped-extraction", Handler: candidates.Version, Trigger: "source_changed", Privacy: "cloud_allowed", IntervalSeconds: 60, MaxJobs: 1, Job: jobs.Request{OriginalRunID: "scope-seed", Sources: []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}, Allowance: budget.Units{ModelTurns: 1, Tokens: 100000, ActiveMillis: 10000, ProviderInputBytes: 8388608}, Deadline: time.Now().Add(time.Hour)}}
	request("/api/v4/memory/model-policies", q, nil)
	request("/api/v4/memory/model-worker-control", map[string]string{"state": "enabled"}, nil)
	if err := j.PollModelPolicies(ctx, func(string) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if worked, err := w.ProcessOne(ctx); !worked || err != nil {
		t.Fatal(worked, err)
	}
	claims, err := c.Claims(ctx)
	if err != nil || len(claims) != 1 || claims[0].Status != "candidate" {
		t.Fatal(claims, err)
	}
	continuation := func(scope string, want bool) {
		t.Helper()
		serial++
		id := fmt.Sprintf("scoped-foreground-%d", serial)
		executor := flow.NewService(session.NewPostgresRepository(pool), session.NewPostgresLeaseManager(pool), runservice.NewService(runservice.NewPostgresRepository(pool), nil), transcript.NewStore(pool), p, flow.Config{ProviderName: p.Name(), ModelName: "scope-fixture", OwnerID: "owner", LeaseTTL: 120, Budgets: b, MemorySources: c, ProfileStore: profileStoreAdapter{store: profile.NewStore(pool), sources: c}, Completion: completion.New(pool)}, slog.New(slog.NewTextHandler(io.Discard, nil)))
		payload, _ := json.Marshal(map[string]string{"text": "Продолжи работу с учётом принятых предпочтений.", "user_id": "owner"})
		if _, err := executor.Execute(ctx, ingress.InputEvent{EventID: id, IdempotencyKey: id, SessionKey: scope, Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: string(payload), CreatedAt: time.Now()}); err != nil {
			t.Fatal("ordinary continuation", err)
		}
		input, _ := json.Marshal(p.last.InputItems)
		if strings.Contains(string(input), preference) != want {
			t.Fatalf("scope=%s expected accepted preference=%t", scope, want)
		}
	}
	continuation("scope-a", false)
	request("/api/v4/memory/decisions", catalog.Decision{ClaimID: claims[0].ID}, nil)
	continuation("scope-a", true)
	continuation("scope-b", false)
	var corrected catalog.Source
	request("/api/v4/memory/correct", catalog.Correction{SourceID: src.ID, ExpectedRevision: src.Revision, Body: "Прежнее предпочтение отменено."}, &corrected)
	continuation("scope-a", false)
	request("/api/v4/memory/delete", map[string]any{"source_id": src.ID, "expected_revision": corrected.Revision, "action": "revoke"}, nil)
	continuation("scope-a", false)
}
