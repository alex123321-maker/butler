package broker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/egress/egresstest"
)

func TestTemporalRevalidationUsesRealAuthorizedBudgetedBrokerReadIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	s := catalog.New(pool, manifest)
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "temporal-read"})
	if _, err := pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:owner','owner','web');INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('run','web:owner','input','running','mode_3','tool_pending','fixture',clock_timestamp(),clock_timestamp());INSERT INTO tasks(task_id,session_key,owner_id,goal) VALUES('task','web:owner','owner','check technical fact');UPDATE runs SET task_id='task',task_revision=1 WHERE run_id='run';INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('web:owner','run','fixture-lease',1,clock_timestamp()+interval '1 hour')`); err != nil {
		t.Fatal(err)
	}
	src, err := s.Put(owner, catalog.PutSource{ExternalKey: "technical", Kind: "owner_statement", ScopeType: "user", ScopeID: "owner", Body: "Technical fixture service exists."})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Propose(owner, catalog.ClaimRequest{SourceID: src.ID, SourceRevision: 1, Key: "service.exists", Value: json.RawMessage(`true`), Summary: "service exists"})
	if err != nil {
		t.Fatal(err)
	}
	c, err = s.Decide(owner, catalog.Decision{ClaimID: c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetTemporal(owner, catalog.TemporalUpdate{ClaimID: c.ID, ExpectedHeadRevision: 1, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessTechnicalFact}); err != nil {
		t.Fatal(err)
	}
	live := ownership.WithToken(context.Background(), ownership.NewAuthority(pool), ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "fixture-lease", FenceEpoch: 1})
	if _, err = s.ReadAcceptedFacts(live, []string{c.ID}, catalog.TemporalQuery{}); err != nil {
		t.Fatal(err)
	}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer site.Close()
	executor := &checkedReadExecutor{client: egresstest.Client(site)}
	server := NewServer(loadTestRegistry(t), executor, nil, nil)
	authority := newAuthFixture()
	store := actionauth.NewPostgresStore(pool)
	b := budget.NewController(pool, budget.DefaultPolicy())
	store.SetBudgets(b)
	store.SetRunDataGuard(s.CheckRunSourcesTx)
	server.SetBudgets(b)
	server.SetActionAuthorizer(actionauth.NewService(store, authority, nil))
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "temporal-read", ToolName: "http.request", AutonomyMode: 3, ArgsJson: `{"method":"GET","url":"` + egresstest.Origin + `"}`}
	prepared, err := server.PrepareToolCall(live, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = server.AuthorizeToolCall(live, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: prepared.GetAction().GetActionId()}); err == nil {
		t.Fatal("external revalidation bypassed approval")
	}
	if executor.calls.Load() != 0 {
		t.Fatal("denial dispatched HTTP")
	}
	i, err := store.GetIntent(live, prepared.GetAction().GetActionId())
	if err != nil {
		t.Fatal(err)
	}
	authority.approvals["exact-read"] = actionauth.ApprovalSnapshot{ApprovalID: "exact-read", Status: "approved", Binding: i.Binding, ResolvedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
	g, err := server.AuthorizeToolCall(live, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: i.Binding.ActionID, ApprovalId: "exact-read"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := server.ExecuteToolCall(live, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: g.GetAuthorizationId()})
	if err != nil || result.GetResult().GetStatus() != "completed" || len(result.GetResult().GetEvidenceRefs()) != 1 {
		t.Fatal("actual Broker read failed", err, result)
	}
	// Core persists the returned sanitized tool result; no fixture journal receipt
	// or budget row is fabricated. The real Broker produced both above.
	if _, err = pool.Exec(owner, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at,finished_at,result) VALUES($1,'run','http.request',$2,'completed','http',clock_timestamp(),clock_timestamp(),$3)`, call.ToolCallId, call.ArgsJson, result.GetResult().GetResultJson()); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(i.Binding.Resource))
	q := catalog.BrokerTemporalUpdate{ClaimID: c.ID, ExpectedRevision: 1, ExpectedHeadRevision: 1, EvidenceID: result.GetResult().GetEvidenceRefs()[0], ExpectedResourceHash: hex.EncodeToString(sum[:]), ConfirmedSameFact: true}
	if _, err = s.SetTemporalFromBrokerRead(live, q); err == nil {
		t.Fatal("model/worker receipt acquired owner review authority")
	}
	bad := q
	bad.ConfirmedSameFact = false
	if _, err = s.SetTemporalFromBrokerRead(owner, bad); err == nil {
		t.Fatal("read automatically became semantic truth")
	}
	bad = q
	bad.ExpectedResourceHash = string(make([]byte, 64))
	if _, err = s.SetTemporalFromBrokerRead(owner, bad); err == nil {
		t.Fatal("unrelated resource accepted")
	}
	if _, err = pool.Exec(owner, `UPDATE tool_calls SET result='{"status_code":503}' WHERE tool_call_id=$1`, call.ToolCallId); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SetTemporalFromBrokerRead(owner, q); err == nil {
		t.Fatal("unavailable/changed read gave false confidence")
	}
	if _, err = pool.Exec(owner, `UPDATE tool_calls SET result=$2 WHERE tool_call_id=$1`, call.ToolCallId, result.GetResult().GetResultJson()); err != nil {
		t.Fatal(err)
	}
	m, err := s.SetTemporalFromBrokerRead(owner, q)
	if err != nil || m.VerificationBasis != "broker_read" || m.VerifiedAt == nil || m.VerificationEvidenceID != q.EvidenceID {
		t.Fatal("reviewed exact observation rejected", m, err)
	}
	if _, err = s.SetTemporalFromBrokerRead(owner, q); err == nil {
		t.Fatal("receipt replay refreshed metadata CAS")
	}
	page, err := s.ReadAcceptedFacts(owner, []string{c.ID}, catalog.TemporalQuery{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Freshness.Status != "fresh" || page.Items[0].Claim.ID != c.ID {
		t.Fatal("revalidation rewrote claim/authority", page, err)
	}
	var before, after int
	if err = pool.QueryRow(owner, `SELECT count(*) FROM execution_budget_reservations`).Scan(&before); err != nil || before != 1 {
		t.Fatal(before, err)
	}
	if _, err = s.Delete(owner, src.ID, "delete", 1); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(owner, `SELECT count(*) FROM execution_budget_reservations`).Scan(&after); err != nil || before != after || executor.calls.Load() != 1 {
		t.Fatal("memory deletion/refusal refunded or repeated read", before, after, err)
	}
}
