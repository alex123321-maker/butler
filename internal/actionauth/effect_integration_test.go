package actionauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

// This receiver implements an explicit key+fingerprint contract. Generic HTTP
// services do not acquire this guarantee merely by accepting an arbitrary header.
type receiptReceiver struct {
	mu      sync.Mutex
	created map[string]string
	effects atomic.Int32
	dropAck atomic.Bool
}

func (r *receiptReceiver) serve(w http.ResponseWriter, q *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := q.Header.Get("Idempotency-Key")
	fingerprint := q.Header.Get("Butler-Action-Fingerprint")
	state, artifact := string(StateConfirmed), "fixture-created-item"
	if q.Method == http.MethodGet {
		key = strings.TrimPrefix(q.URL.Path, "/effects/")
		fingerprint = r.created[key]
		if fingerprint == "" {
			state, artifact = string(StateAbsent), ""
			fingerprint = q.Header.Get("Expected-Fingerprint")
		}
	}
	if q.Method == http.MethodPost {
		if key == "" || fingerprint == "" {
			w.WriteHeader(400)
			return
		}
		if old, ok := r.created[key]; ok && old != fingerprint {
			w.WriteHeader(409)
			return
		}
		if _, ok := r.created[key]; !ok {
			r.created[key] = fingerprint
			r.effects.Add(1)
		}
	}
	if q.Method == http.MethodPost && r.dropAck.Swap(false) {
		connection, _, err := w.(http.Hijacker).Hijack()
		if err == nil {
			_ = connection.Close()
		}
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"butler_effect_receipt": map[string]string{"idempotency_key": key, "action_fingerprint": fingerprint, "effect_state": state, "artifact_id": artifact}})
}
func receiptResult(t *testing.T, call *toolbrokerv1.ToolCall, response *http.Response) *toolbrokerv1.ToolResult {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"status_code": response.StatusCode, "body": string(body), "truncated": false})
	return &toolbrokerv1.ToolResult{RunId: call.GetRunId(), ToolCallId: call.GetToolCallId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: string(raw)}
}
func receiptFixture(t *testing.T) (*Service, *PostgresStore, *contexts, *toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract, *receiptReceiver, *httptest.Server) {
	t.Helper()
	pool := actionDatabase(t)
	s, _, reader, _, call, contract := fixture(t)
	store := NewPostgresStore(pool)
	s.store = store
	s.now = func() time.Time { return time.Now().UTC() }
	receiver := &receiptReceiver{created: map[string]string{}}
	server := httptest.NewServer(http.HandlerFunc(receiver.serve))
	t.Cleanup(server.Close)
	call.ArgsJson = fmt.Sprintf(`{"method":"POST","url":%q}`, server.URL+"/create")
	contract.ContractRevision = "receipt-fixture-v1"
	contract.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "external_idempotency", VerificationMethod: "external_receipt_v1", IdempotencyHeader: "Idempotency-Key", ScopeOrigin: server.URL, VerificationUrl: server.URL + "/effects/{idempotency_key}"}
	return s, store, reader, call, contract, receiver, server
}
func logicalGrant(t *testing.T, s *Service, c *contexts, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, ref string) Grant {
	t.Helper()
	p, err := s.PrepareWithLogicalRef(context.Background(), call, contract, ref)
	if err != nil {
		t.Fatal(err)
	}
	c.approved = ApprovalSnapshot{ApprovalID: "approval", Status: "approved", Binding: p.Binding, ExpiresAt: s.now().Add(1000000000000), ResolvedAt: s.now()}
	g, err := s.Authorize(context.Background(), call, contract, p.Binding.ActionID, "approval", "")
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func postReceipt(t *testing.T, server *httptest.Server, call *toolbrokerv1.ToolCall, e EffectRecord) *toolbrokerv1.ToolResult {
	t.Helper()
	q, _ := http.NewRequest(http.MethodPost, server.URL+"/create", strings.NewReader("fixture"))
	q.Header.Set("Idempotency-Key", e.IdempotencyKey)
	q.Header.Set("Butler-Action-Fingerprint", e.Fingerprint)
	response, err := server.Client().Do(q)
	if err != nil {
		t.Fatal(err)
	}
	return receiptResult(t, call, response)
}
func observeEffect(t *testing.T, s *Service, store *PostgresStore, c *contexts, contract *toolbrokerv1.ToolContract, e EffectRecord, server *httptest.Server) (Grant, *toolbrokerv1.ToolResult) {
	t.Helper()
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "observation-" + randomID(), ToolName: "http.request", AutonomyMode: 3, ArgsJson: fmt.Sprintf(`{"method":"GET","url":%q}`, verificationResource(e))}
	p, err := s.Prepare(context.Background(), call, contract)
	if err != nil {
		t.Fatal(err)
	}
	g, err := s.Authorize(context.Background(), call, contract, p.Binding.ActionID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ClaimGrant(context.Background(), g.ID); err != nil {
		t.Fatal(err)
	}
	q, _ := http.NewRequest(http.MethodGet, verificationResource(e), nil)
	q.Header.Set("Expected-Fingerprint", e.Fingerprint)
	response, err := server.Client().Do(q)
	if err != nil {
		t.Fatal(err)
	}
	result := receiptResult(t, call, response)
	if record, err := s.Outcome(context.Background(), g, result, false); err != nil || record.State != StateConfirmed {
		t.Fatalf("observation failed: state=%s error=%v", record.State, err)
	}
	return g, result
}

func TestEffectJournalLostAcknowledgementReconcilesWithoutReplay(t *testing.T) {
	for _, fault := range []string{"after_accept_before_response", "after_response_before_persistence"} {
		t.Run(fault, func(t *testing.T) {
			s, store, c, call, contract, receiver, server := receiptFixture(t)
			ctx := context.Background()
			g := logicalGrant(t, s, c, call, contract, "create-slot-1")
			e, err := s.EffectForGrant(ctx, g)
			if err != nil {
				t.Fatal(err)
			}
			if err = store.ClaimGrant(ctx, g.ID); err != nil {
				t.Fatal(err)
			}
			var result *toolbrokerv1.ToolResult
			if fault == "after_accept_before_response" {
				receiver.dropAck.Store(true)
				q, _ := http.NewRequest(http.MethodPost, server.URL+"/create", strings.NewReader("fixture"))
				q.Header.Set("Idempotency-Key", e.IdempotencyKey)
				q.Header.Set("Butler-Action-Fingerprint", e.Fingerprint)
				response, requestErr := server.Client().Do(q)
				if requestErr == nil {
					response.Body.Close()
					t.Fatal("recipient intentionally lost its acknowledgement")
				}
				if receiver.effects.Load() != 1 {
					t.Fatal("external acceptance did not precede connection loss")
				}
			} else {
				result = postReceipt(t, server, call, e)
			}
			if fault == "after_response_before_persistence" {
				_, err = store.pool.Exec(ctx, `CREATE FUNCTION lose_outcome() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected outcome loss'; END $$; CREATE TRIGGER lose_outcome BEFORE INSERT ON action_effect_evidence FOR EACH ROW EXECUTE FUNCTION lose_outcome()`)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = s.Outcome(ctx, g, result, false); err == nil {
					t.Fatal("injected persistence failure was ignored")
				}
				if _, err = store.pool.Exec(ctx, `DROP TRIGGER lose_outcome ON action_effect_evidence; DROP FUNCTION lose_outcome()`); err != nil {
					t.Fatal(err)
				}
			}
			// Recreate the service as after process loss; executing is durable and spent.
			restarted := NewService(NewPostgresStore(store.pool), c, nil)
			if err = store.MarkInterruptedEffect(ctx, e.ActionID); err != nil {
				t.Fatal(err)
			}
			if _, err = restarted.Authorize(ctx, call, contract, g.Binding.ActionID, "approval", ""); err == nil {
				t.Fatal("uncertain effect reauthorized")
			}
			if err = store.ClaimGrant(ctx, g.ID); err == nil {
				t.Fatal("uncertain effect replayed")
			}
			observation, observed := observeEffect(t, restarted, store, c, contract, e, server)
			forged := *observed
			forged.ResultJson = strings.ReplaceAll(observed.ResultJson, "fixture-created-item", "invented-item")
			if _, err = restarted.Reconcile(ctx, e.ActionID, observation, &forged); err == nil {
				t.Fatal("verifier invented uncaptured external evidence")
			}
			reconciled, err := restarted.Reconcile(ctx, e.ActionID, observation, observed)
			if err != nil || reconciled.State != StateConfirmed {
				t.Fatalf("reconciliation: state=%s error=%v", reconciled.State, err)
			}
			if receiver.effects.Load() != 1 {
				t.Fatal("recipient received duplicate effect")
			}
			evidence, err := store.GetEffectEvidence(ctx, reconciled.EvidenceRefs[len(reconciled.EvidenceRefs)-1])
			if err != nil || evidence.Source != "independent_lookup" || evidence.SourceActionID != observation.Binding.ActionID || evidence.ObservedAt.IsZero() || evidence.ArtifactDigest == "" {
				t.Fatal("independent provenance was lost")
			}
			if _, err = store.pool.Exec(ctx, `UPDATE action_effect_evidence SET evidence='{}'::jsonb WHERE evidence_id=$1`, evidence.EvidenceID); err == nil {
				t.Fatal("append-only evidence was mutable")
			}
		})
	}
}
func TestEffectJournalBeforeDispatchRollbackAndAbsentObservation(t *testing.T) {
	s, store, c, call, contract, receiver, server := receiptFixture(t)
	ctx := context.Background()
	g := logicalGrant(t, s, c, call, contract, "prepare-slot")
	e, _ := s.EffectForGrant(ctx, g)
	_, err := store.pool.Exec(ctx, `CREATE FUNCTION refuse_dispatch() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.state='executing' THEN RAISE EXCEPTION 'injected before dispatch'; END IF; RETURN NEW; END $$; CREATE TRIGGER refuse_dispatch BEFORE UPDATE ON action_effects FOR EACH ROW EXECUTE FUNCTION refuse_dispatch()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ClaimGrant(ctx, g.ID); err == nil {
		t.Fatal("injected dispatch commit failure was ignored")
	}
	e, _ = store.GetEffect(ctx, e.ActionID)
	if e.State != StateAllowed || receiver.effects.Load() != 0 {
		t.Fatal("failed journal commit allowed an external call")
	}
	_, err = store.pool.Exec(ctx, `DROP TRIGGER refuse_dispatch ON action_effects; DROP FUNCTION refuse_dispatch()`)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.ClaimGrant(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.MarkInterruptedEffect(ctx, e.ActionID); err != nil {
		t.Fatal(err)
	}
	observation, result := observeEffect(t, s, store, c, contract, e, server)
	absent, err := s.Reconcile(ctx, e.ActionID, observation, result)
	if err != nil || absent.State != StateAbsent {
		t.Fatalf("absence: state=%s err=%v", absent.State, err)
	}
	// Observed absence records fact, not a renewed mutation permission.
	if _, err = s.Authorize(ctx, call, contract, g.Binding.ActionID, "approval", ""); err == nil {
		t.Fatal("absence silently created a replay grant")
	}
}
func TestEffectJournalLogicalIdentityAndRecipientKey(t *testing.T) {
	s, store, c, call, contract, receiver, server := receiptFixture(t)
	ctx := context.Background()
	g := logicalGrant(t, s, c, call, contract, "permanent-slot")
	e, _ := s.EffectForGrant(ctx, g)
	if err := store.ClaimGrant(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	result := postReceipt(t, server, call, e)
	confirmed, err := s.Outcome(ctx, g, result, false)
	if err != nil || confirmed.State != StateConfirmed {
		t.Fatal("valid receipt did not confirm effect")
	}
	// The independently documented recipient guarantee reuses one artifact.
	postReceipt(t, server, call, e)
	if receiver.effects.Load() != 1 {
		t.Fatal("receiver did not honor its key contract")
	}
	call.ToolCallId = "another-call"
	c.current.RunID = "another-run"
	call.RunId = "another-run"
	p, err := s.PrepareWithLogicalRef(ctx, call, contract, "permanent-slot")
	if err != nil || p.EffectID != e.ActionID {
		t.Fatal("logical task identity changed across executions")
	}
	c.approved = ApprovalSnapshot{ApprovalID: "approval", Status: "approved", Binding: p.Binding, ExpiresAt: s.now().Add(1000000000000), ResolvedAt: s.now()}
	if _, err = s.Authorize(ctx, call, contract, p.Binding.ActionID, "approval", ""); err == nil {
		t.Fatal("known effect dispatched from another run")
	}
	call.ToolCallId = "changed-call"
	call.ArgsJson = fmt.Sprintf(`{"method":"POST","url":%q,"body":"different"}`, server.URL+"/create")
	if _, err = s.PrepareWithLogicalRef(ctx, call, contract, "permanent-slot"); err == nil {
		t.Fatal("permanent identity accepted changed arguments")
	}
	// Completion/memory/delivery code is downstream of this immutable journal.
	evidence, err := store.GetRunEvidence(ctx, "run")
	if err != nil || len(evidence) != 1 || evidence[0].EffectState != StateConfirmed || len(evidence[0].EvidenceRefs) != 1 {
		t.Fatal("final-result evidence bridge lost confirmed effect")
	}
}
func TestEffectJournalGenericSuccessIsUnverifiedAndCancellationFencesClaim(t *testing.T) {
	s, store, c, call, contract, _, _ := receiptFixture(t)
	contract.EffectContract = nil
	ctx := context.Background()
	g := prepareApproved(t, s, c, call, contract)
	if _, err := store.pool.Exec(ctx, `UPDATE session_ownership SET fence_epoch=fence_epoch+1,lease_id=''`); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimGrant(ctx, g.ID); err == nil {
		t.Fatal("cancelled execution committed an irreversible dispatch")
	}
	if _, err := store.pool.Exec(ctx, `UPDATE session_ownership SET fence_epoch=4,lease_id='lease'`); err != nil {
		t.Fatal(err)
	}
	if err := store.ClaimGrant(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	result := &toolbrokerv1.ToolResult{RunId: call.GetRunId(), ToolCallId: call.GetToolCallId(), ToolName: call.GetToolName(), Status: "completed", ResultJson: `{"status_code":201,"body":"created"}`}
	e, err := s.Outcome(ctx, g, result, false)
	if err != nil || e.State != StateUnknown || e.DispatchOutcome != "dispatch_completed" {
		t.Fatal("generic success falsely proved external effect")
	}
}

func TestEffectJournalCancelAndPrepareOnlySerializeWithClaim(t *testing.T) {
	for _, mutation := range []string{"cancel", "prepare_only", "constraints", "owner"} {
		t.Run(mutation, func(t *testing.T) {
			s, store, c, call, contract, _, _ := receiptFixture(t)
			ctx := context.Background()
			g := prepareApproved(t, s, c, call, contract)
			controller, err := store.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer controller.Rollback(ctx)
			var key string
			if err = controller.QueryRow(ctx, `SELECT session_key FROM session_ownership WHERE run_id='run' FOR UPDATE`).Scan(&key); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				bounded, cancel := context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				done <- store.ClaimGrant(bounded, g.ID)
			}()
			select {
			case err := <-done:
				t.Fatalf("dispatch escaped ownership fence: %v", err)
			case <-time.After(30 * time.Millisecond):
			}
			switch mutation {
			case "cancel":
				_, err = controller.Exec(ctx, `UPDATE session_ownership SET lease_id='',fence_epoch=fence_epoch+1; UPDATE tasks SET status='cancelled'`)
			case "prepare_only":
				_, err = controller.Exec(ctx, `UPDATE tasks SET execution_policy='prepare_only'`)
			case "constraints":
				_, err = controller.Exec(ctx, `UPDATE tasks SET constraints_revision=constraints_revision+1`)
			case "owner":
				_, err = controller.Exec(ctx, `UPDATE tasks SET owner_id='another-owner'`)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = controller.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err == nil {
				t.Fatal("stale task authority committed an external dispatch")
			}
			effect, err := s.EffectForGrant(ctx, g)
			if err != nil || effect.State != StateAllowed {
				t.Fatal("denied claim consumed or started the action")
			}
		})
	}
}

func TestEffectCompensationIsASeparateApprovedIntent(t *testing.T) {
	s, store, c, call, contract, receiver, server := receiptFixture(t)
	ctx := context.Background()
	original := logicalGrant(t, s, c, call, contract, "original-intent")
	old, _ := s.EffectForGrant(ctx, original)
	if err := store.ClaimGrant(ctx, original.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Outcome(ctx, original, postReceipt(t, server, call, old), false); err != nil {
		t.Fatal(err)
	}
	call.ToolCallId = "compensation-call"
	call.ArgsJson = fmt.Sprintf(`{"method":"POST","url":%q,"body":"owner-requested-compensation"}`, server.URL+"/create")
	prepared, err := s.PrepareWithLogicalRef(ctx, call, contract, "compensation-intent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Authorize(ctx, call, contract, prepared.Binding.ActionID, "approval", ""); err == nil {
		t.Fatal("original approval authorized a compensation")
	}
	c.approved = ApprovalSnapshot{ApprovalID: "compensation-approval", Status: "approved", Binding: prepared.Binding, ResolvedAt: s.now(), ExpiresAt: s.now().Add(time.Hour)}
	g, err := s.Authorize(ctx, call, contract, prepared.Binding.ActionID, "compensation-approval", "")
	if err != nil {
		t.Fatal(err)
	}
	effect, _ := s.EffectForGrant(ctx, g)
	if effect.ActionID == old.ActionID || effect.IdempotencyKey == old.IdempotencyKey {
		t.Fatal("compensation reused original effect identity")
	}
	if receiver.effects.Load() != 1 {
		t.Fatal("preparing compensation executed it")
	}
}

func TestEffectJournalLegacyCompletedMigrationRemainsUnverified(t *testing.T) {
	pool := actionDatabase(t)
	ctx := context.Background()
	down, err := os.ReadFile(filepath.Join("..", "..", "migrations", "035_external_effect_journal.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	service, _, reader, _, call, contract := fixture(t)
	grant := prepareApproved(t, service, reader, call, contract)
	binding, _ := json.Marshal(grant.Binding)
	data, _ := json.Marshal(grant)
	_, err = pool.Exec(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at) VALUES($1,$2,$3,$4,$5::jsonb,true,clock_timestamp()+INTERVAL '1 hour')`, grant.Binding.ActionID, grant.Binding.RunID, grant.Binding.ToolCallID, grant.Binding.ActionHash, string(binding))
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO action_authorizations(authorization_id,action_id,action_hash,grant_data,expires_at,state,dispatched_at,finished_at) VALUES($1,$2,$3,$4::jsonb,clock_timestamp()+INTERVAL '1 minute','dispatch_completed',clock_timestamp(),clock_timestamp())`, grant.ID, grant.Binding.ActionID, grant.Binding.ActionHash, string(data))
	if err != nil {
		t.Fatal(err)
	}
	up, err := os.ReadFile(filepath.Join("..", "..", "migrations", "035_external_effect_journal.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	record, err := NewPostgresStore(pool).GetEffect(ctx, grant.Binding.ActionID)
	if err != nil || record.State != StateUnknown || len(record.EvidenceRefs) != 0 {
		t.Fatal("migration invented a confirmed external result")
	}
	service.store = NewPostgresStore(pool)
	if _, err = service.Authorize(ctx, call, contract, grant.Binding.ActionID, "approval", ""); err == nil {
		t.Fatal("legacy completed grant became replayable")
	}
	if _, err = pool.Exec(ctx, string(down)); err != nil {
		t.Fatal(err)
	}
	stored, err := service.store.GetGrant(ctx, grant.ID)
	if err != nil || stored.State != "dispatch_completed" {
		t.Fatal("rollback changed historical dispatch authority")
	}
}
