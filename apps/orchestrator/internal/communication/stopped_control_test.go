package communication_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/owneridentity"
)

func stopResumeFixture(t *testing.T, opt fixtureOptions, keepClaim, keepLease bool) (*fixture, communication.CloseFailedRequest) {
	t.Helper()
	f, _, live := answeredResume(t, opt)
	if !keepClaim {
		r, err := f.s.Inputs.Get(live, f.claim.InputID)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.s.Inputs.Finish(live, r, "blocked", "interaction_invalidated", "original stopped failure", 0); err != nil {
			t.Fatal(err)
		}
	}
	if !keepLease {
		if _, err := f.leases.ReleaseLease(live, f.lease.LeaseID); err != nil {
			t.Fatal(err)
		}
	}
	return f, communication.CloseFailedRequest{BindingID: f.bindings["web"], BindingVersion: 1, ExpectedCheckpoint: "interaction_ready", ExpectedErrorCode: "interaction_invalidated"}
}

func TestCommunicationCloseFailedKeepsEvidenceAndOneAuditIntegration(t *testing.T) {
	f, q := stopResumeFixture(t, fixtureOptions{}, false, false)
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", SessionKey: "web:" + f.p.OwnerID, DeliveryID: "close-failed"})
	if err := f.s.Bind(ctx, communication.Binding{ID: q.BindingID, Channel: "web", Recipient: "web:" + f.p.OwnerID, Enabled: true, Capabilities: []string{"message", "widget"}}, 1); err != nil {
		t.Fatal(err)
	}
	q.BindingVersion = 2
	_, err := f.pool.Exec(f.ctx, `INSERT INTO execution_budget_reservations(reservation_id,owner_id,operation_id,fingerprint,request_json) VALUES('retained-reservation',$1,'old-operation','fixture','{}')`, f.p.OwnerID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO execution_model_attempts(operation_id,run_id,reservation_id,provider,model,input_allowance,output_allowance) VALUES('old-operation',$1,'retained-reservation','fixture','fixture',1,1)`, f.p.RunID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO execution_loop_observations(effect_id,owner_id,task_root,state_key,call_key,category,result_digest) VALUES('old-loop',$1,$2,'s','c','read','digest')`, f.p.OwnerID, f.p.SessionKey)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := func() map[string]string {
		t.Helper()
		out := map[string]string{}
		for _, table := range []string{"execution_budget_scopes", "execution_budget_reservations", "execution_model_attempts", "execution_loop_observations", "communication_publications", "communication_interactions", "messages"} {
			var data string
			if err := f.pool.QueryRow(f.ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(x) ORDER BY to_jsonb(x)::text),'[]'::jsonb)::text FROM `+table+` x`).Scan(&data); err != nil {
				t.Fatal(err)
			}
			out[table] = data
		}
		return out
	}
	before := snapshot()
	var original map[string]any
	var data []byte
	if err = f.pool.QueryRow(f.ctx, `SELECT to_jsonb(x) FROM accepted_inputs x WHERE input_id=$1`, f.claim.InputID).Scan(&data); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &original); err != nil {
		t.Fatal(err)
	}
	delete(original, "status")
	delete(original, "updated_at")
	const proof = "synthetic-owner-close-proof-12345678901234567890"
	handler := api.CommunicationWeb(api.NewOwnerControlAuth(proof, f.p.OwnerID), &communication.WebReader{Core: f.s}, nil, nil)
	post := func(token, body string) int {
		request := httptest.NewRequest(http.MethodPost, "/api/v3/communications/inputs/"+f.claim.InputID+"/close-failed", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("X-Butler-Delivery-ID", "close-failed")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response.Code
	}
	raw, _ := json.Marshal(q)
	if got := post("wrong", string(raw)); got != 401 {
		t.Fatal("owner authentication", got)
	}
	if got := post(proof, `{"binding_id":"x","binding_version":2,"expected_checkpoint":"interaction_ready","expected_error_code":"interaction_invalidated","status":"completed"}`); got != 400 {
		t.Fatal("model/body cannot choose completion", got)
	}
	for i := 0; i < 2; i++ {
		if got := post(proof, string(raw)); got != 200 {
			t.Fatal("idempotent failed close", i, got)
		}
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("failure acknowledgement rewrote evidence or budgets")
	}
	if err = f.pool.QueryRow(f.ctx, `SELECT to_jsonb(x) FROM accepted_inputs x WHERE input_id=$1`, f.claim.InputID).Scan(&data); err != nil {
		t.Fatal(err)
	}
	var after map[string]any
	if err = json.Unmarshal(data, &after); err != nil {
		t.Fatal(err)
	}
	if after["status"] != "failed" {
		t.Fatal("failed attempt claimed success")
	}
	delete(after, "status")
	delete(after, "updated_at")
	if !reflect.DeepEqual(original, after) {
		t.Fatal("original error/checkpoint/input changed")
	}
	if f.count(t, "communication_events") < 1 {
		t.Fatal("missing owner audit")
	}
	var events, transitions int
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM communication_events WHERE aggregate_id=$1 AND event_type='input.closed_failed'`, f.claim.InputID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err = f.pool.QueryRow(f.ctx, `SELECT count(*) FROM run_state_transitions WHERE run_id=$1 AND to_state='failed' AND triggered_by='owner'`, f.p.RunID).Scan(&transitions); err != nil {
		t.Fatal(err)
	}
	if events != 1 || transitions != 1 {
		t.Fatal("duplicate or missing owning audit", events, transitions)
	}
}

func TestCommunicationCloseFailedRejectsLiveAndUncertainIntegration(t *testing.T) {
	for _, mode := range []string{"wrong_owner", "binding", "checkpoint", "claim", "lease", "task", "executing", "unknown_write"} {
		t.Run(mode, func(t *testing.T) {
			f, q := stopResumeFixture(t, fixtureOptions{Task: mode == "task"}, mode == "claim", mode == "lease")
			actor := owneridentity.Actor{OwnerID: f.p.OwnerID, Channel: "web", SessionKey: "web:verified", DeliveryID: "close"}
			if mode == "wrong_owner" {
				actor.OwnerID = "other"
			}
			if mode == "binding" {
				q.BindingVersion++
			}
			if mode == "checkpoint" {
				q.ExpectedCheckpoint = "other"
			}
			if mode == "executing" || mode == "unknown_write" {
				state := "executing"
				if mode == "unknown_write" {
					state = "effect_unknown"
				}
				if _, err := f.pool.Exec(f.ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state) VALUES('uncertain','fixture',jsonb_build_object('run_id',$1::text,'effect','write'),'{}',$2)`, f.p.RunID, state); err != nil {
					t.Fatal(err)
				}
			}
			before, err := f.s.Inputs.Get(f.ctx, f.claim.InputID)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.s.CloseFailed(owneridentity.WithActor(context.Background(), actor), f.claim.InputID, q); err == nil {
				t.Fatal("unsafe close admitted", mode)
			}
			after, err := f.s.Inputs.Get(f.ctx, f.claim.InputID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("denied close mutated input")
			}
		})
	}
}
