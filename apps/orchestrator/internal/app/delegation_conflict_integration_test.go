package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
)

// The two finals below are synthetic DATA, not inference or recipient evidence.
// Real stores retain each final/report separately. A fixed independent check
// reads the original Catalog witness and each saved artifact before the owner
// verification path can accept one result and reject its contradiction.
func TestDelegationConflictingReportsRequireOriginalWitnessIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	witness := f.source(t, "conflict-original", "user", f.parent.OwnerID, `{"observed_value":17,"observed_date":"2026-10-11"}`, nil)
	var children []task.Record
	var results []task.Result
	var reports []delegation.Report
	var originals [][]catalog.SourceRef
	var collected []delegation.SourceRef
	for i, value := range []int64{17, 23} {
		key := []string{"first-conflict", "second-conflict"}[i]
		q := f.proposal(t, key)
		q.Sources = []delegation.SourceRef{witness}
		q.ContractSource = f.numericResearchContract(t, key+"-numeric", witness, nil)
		admitted, err := f.manager.Admit(f.owner, q)
		if err != nil {
			t.Fatal(err)
		}
		child, err := f.store.Get(f.ctx, admitted.Contract.TaskID)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(delegation.NumericObservation{SourceID: witness.ID, SourceRevision: witness.Revision, SourceHash: witness.Hash, ObservedValue: value, ObservedDate: "2026-10-11", Uncertainty: "source_only"})
		result, report, parents := f.completeWithContent(t, admitted.Contract, child, false, false, string(body))
		children = append(children, child)
		results = append(results, result)
		reports = append(reports, report)
		originals = append(originals, parents)
	}
	// Without independently persisted verification, both apparent answers deny.
	for i, child := range children {
		ref := f.report(t, []string{"unverified-a", "unverified-b"}[i], child, reports[i], originals[i])
		if _, err := f.manager.Collect(f.ctx, f.parent.TaskID, child.TaskID, results[i].ResultID, ref); err != nil {
			t.Fatal(err)
		}
		collected = append(collected, ref)
		if _, err := f.manager.Decide(f.owner, f.parent.TaskID, child.TaskID, task.DelegationDecision{Accept: true, Report: ref, Evidence: []delegation.SourceRef{witness}}); err == nil {
			t.Fatal("conflicting model DATA became objective verification")
		}
	}
	original, err := f.sources.Get(f.owner, witness.ID)
	if err != nil || guardSource(original) != witness {
		t.Fatal("current original witness unavailable", err)
	}
	var accepted, rejected int
	ownerToken := strings.Repeat("fixture-owner-", 4)
	ownerAPI := api.DelegationOwnerDecisions(api.NewOwnerControlAuth(ownerToken, f.parent.OwnerID), f.manager)
	for i, child := range children {
		var body string
		if err := f.pool.QueryRow(f.ctx, `SELECT content_text FROM artifacts WHERE artifact_id=$1 AND NOT privacy_redacted`, reports[i].Evidence[1].ID).Scan(&body); err != nil {
			t.Fatal(err)
		}
		matched := delegation.VerifyNumericObservation([]byte(body), witness, []byte(original.Body)) == nil
		verified, err := f.store.SaveVerifiedResult(f.owner, task.Result{TaskID: child.TaskID, TaskRevision: child.Revision, RunID: results[i].RunID, Summary: "Synthetic owner verification for objective guard", EvidenceRefs: []string{DelegationSourceEvidence(witness), DelegationRecordEvidence(delegation.EvidenceRef{Kind: "action", ID: reports[i].Actions[0].ActionID, Hash: validationHash}), DelegationRecordEvidence(reports[i].Actions[0].Evidence[0])}, VerifiedCriteria: child.CompletionCriteria, Verifier: "owner"})
		if err != nil {
			t.Fatal(err)
		}
		if matched {
			results[i] = verified
			reports[i].ResultID, reports[i].Evidence[0].ID = verified.ResultID, verified.ResultID
			reports[i].Uncertainty = nil
			accepted++
		} else {
			// Even an actual owner-verified row cannot accept the contradictory
			// immutable final under this selected objective contract.
			if _, err := f.manager.AcceptVerified(f.owner, f.parent.TaskID, child.TaskID, collected[i], verified.ResultID, []delegation.SourceRef{witness}); err == nil {
				t.Fatal("owner verification overrode the original numeric witness")
			}
			var storedResult, state string
			if err := f.pool.QueryRow(f.ctx, `SELECT result_id,state FROM task_delegations WHERE task_id=$1`, child.TaskID).Scan(&storedResult, &state); err != nil || storedResult != results[i].ResultID || state != "result_ready" {
				t.Fatal("denied objective acceptance changed the collected observation", err)
			}
			rejected++
		}
		payload := map[string]any{"kind": "reject", "report_source": collected[i], "evidence_refs": []delegation.SourceRef{witness}}
		if matched {
			payload["kind"], payload["verified_result_id"] = "accept_verified", results[i].ResultID
		}
		encoded, _ := json.Marshal(payload)
		request := httptest.NewRequest(http.MethodPost, "/api/v3/delegation-decisions/"+f.parent.TaskID+"/"+child.TaskID, strings.NewReader(string(encoded)))
		request.Header.Set("Authorization", "Bearer "+ownerToken)
		response := httptest.NewRecorder()
		ownerAPI.ServeHTTP(response, request)
		var decision task.DelegationRecord
		if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &decision) != nil || decision.Decision == nil || decision.Decision.Accept != matched {
			t.Fatal("independent conflict decision was not retained through the owner API", response.Code, response.Body.String())
		}
	}
	var previousReportID, previousResultID string
	if f.pool.QueryRow(f.ctx, `SELECT decision_evidence_json->'previous_report_source'->>'source_id',decision_evidence_json->>'previous_result_id' FROM task_delegations WHERE task_id=$1`, children[0].TaskID).Scan(&previousReportID, &previousResultID) != nil || previousReportID != collected[0].ID || previousResultID == "" || previousResultID == results[0].ResultID {
		t.Fatal("promotion discarded original collected evidence")
	}
	if accepted != 1 || rejected != 1 {
		t.Fatal("original witness did not separate conflicting answers")
	}
	for _, child := range children {
		r, err := f.store.Get(f.ctx, child.TaskID)
		if err != nil || r.Status != "active" {
			t.Fatal("decision completed a child task", err)
		}
	}
	parent, err := f.store.Get(f.ctx, f.parent.TaskID)
	if err != nil || parent.Status != "active" {
		t.Fatal("conflict resolution completed the parent", err)
	}
}

func (f *delegationGuardFixture) numericResearchContract(t *testing.T, key string, witness delegation.SourceRef, override []byte) delegation.SourceRef {
	t.Helper()
	body, _ := json.Marshal(struct {
		Kind   string               `json:"kind"`
		Source delegation.SourceRef `json:"source_ref"`
	}{delegation.NumericObservationKind, witness})
	if override != nil {
		body = override
	}
	source, err := f.sources.Put(f.owner, catalog.PutSource{ExternalKey: key, Kind: "model_candidate", ScopeType: "user", ScopeID: f.parent.OwnerID, Body: string(body), Title: "Bounded numeric source observation"})
	if err != nil {
		t.Fatal(err)
	}
	return guardSource(source)
}

func TestDelegationNumericAcceptanceCannotUseAgreementOrMalformedContractIntegration(t *testing.T) {
	for _, scenario := range []string{"agreeing_wrong_finals", "duplicate_marker", "unknown_schema", "unselected_witness", "missing_observed_value", "null_observed_value", "missing_source_evidence"} {
		t.Run(scenario, func(t *testing.T) {
			f := newDelegationGuardFixture(t)
			witness := f.source(t, "numeric-original", "user", f.parent.OwnerID, `{"observed_value":0,"observed_date":"2026-10-11"}`, nil)
			selected := witness
			override := []byte(nil)
			switch scenario {
			case "duplicate_marker":
				override = []byte(`{"kind":"delegation_numeric_observation_v1","kind":"legacy"}`)
			case "unknown_schema":
				override = []byte(`{"kind":"delegation_numeric_observation_v2"}`)
			case "unselected_witness", "missing_source_evidence":
				selected = f.source(t, "numeric-other", "user", f.parent.OwnerID, `{"observed_value":0,"observed_date":"2026-10-11"}`, nil)
			}
			count := 1
			if scenario == "agreeing_wrong_finals" {
				count = 2
			}
			for i := 0; i < count; i++ {
				key := []string{"numeric-first", "numeric-second"}[i]
				q := f.proposal(t, key)
				q.Sources = []delegation.SourceRef{selected}
				if scenario == "missing_source_evidence" {
					q.Sources = append(q.Sources, witness)
				}
				q.ContractSource = f.numericResearchContract(t, key+"-contract", witness, override)
				admitted, err := f.manager.Admit(f.owner, q)
				if err != nil {
					t.Fatal(err)
				}
				child, err := f.store.Get(f.ctx, admitted.Contract.TaskID)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := json.Marshal(delegation.NumericObservation{SourceID: witness.ID, SourceRevision: witness.Revision, SourceHash: witness.Hash, ObservedValue: 0, ObservedDate: "2026-10-11", Uncertainty: "source_only"})
				raw := string(body)
				switch scenario {
				case "agreeing_wrong_finals":
					raw = strings.Replace(raw, `"observed_value":0`, `"observed_value":23`, 1)
				case "missing_observed_value":
					raw = strings.Replace(raw, `"observed_value":0,`, "", 1)
				case "null_observed_value":
					raw = strings.Replace(raw, `"observed_value":0`, `"observed_value":null`, 1)
				}
				result, report, parents := f.completeWithContent(t, admitted.Contract, child, false, false, raw)
				ref := f.report(t, key+"-report", child, report, parents)
				if _, err := f.manager.Collect(f.ctx, f.parent.TaskID, child.TaskID, result.ResultID, ref); err != nil {
					t.Fatal("unverified invalid observation must remain collectable", err)
				}
				verified, err := f.store.SaveVerifiedResult(f.owner, task.Result{TaskID: child.TaskID, TaskRevision: child.Revision, RunID: result.RunID, Summary: "Synthetic owner proof cannot override objective facts", EvidenceRefs: []string{DelegationSourceEvidence(selected), DelegationRecordEvidence(delegation.EvidenceRef{Kind: "action", ID: report.Actions[0].ActionID, Hash: validationHash}), DelegationRecordEvidence(report.Actions[0].Evidence[0])}, VerifiedCriteria: child.CompletionCriteria, Verifier: "owner"})
				if err != nil {
					t.Fatal(err)
				}
				var before string
				if err := f.pool.QueryRow(f.ctx, `SELECT decision_evidence_json::text FROM task_delegations WHERE task_id=$1`, child.TaskID).Scan(&before); err != nil {
					t.Fatal(err)
				}
				if _, err := f.manager.AcceptVerified(f.owner, f.parent.TaskID, child.TaskID, ref, verified.ResultID, []delegation.SourceRef{selected}); err == nil {
					t.Fatal("unsupported numeric final accepted", scenario)
				}
				var after, state, current string
				if err := f.pool.QueryRow(f.ctx, `SELECT decision_evidence_json::text,state,result_id FROM task_delegations WHERE task_id=$1`, child.TaskID).Scan(&after, &state, &current); err != nil || after != before || state != "result_ready" || current != result.ResultID {
					t.Fatal("denial rewrote retained observation", err)
				}
			}
		})
	}
}

func TestDelegationIndependentVerificationRejectsWorkerAndForeignProofIntegration(t *testing.T) {
	f := newDelegationGuardFixture(t)
	c, child := f.admit(t, "verification-proof")
	result, report, parents := f.complete(t, c, child, false, false)
	ref := delegation.SourceRef{ID: parents[0].ID, Revision: parents[0].Revision, AccessRevision: parents[0].AccessRevision, Hash: parents[0].Hash}
	claim := task.Result{TaskID: child.TaskID, TaskRevision: child.Revision, RunID: result.RunID, Summary: "owner is merely a claimed string", EvidenceRefs: []string{DelegationSourceEvidence(ref), DelegationRecordEvidence(report.Actions[0].Evidence[0])}, VerifiedCriteria: child.CompletionCriteria, Verifier: "owner"}
	worker := f.leasedWorker(t, f.parent, "verification-worker")
	for _, scenario := range []string{"anonymous", "foreign_owner", "foreign_session", "worker", "mixed_owner_worker"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := f.ctx
			switch scenario {
			case "foreign_owner":
				ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "another-owner", Channel: "web", DeliveryID: "foreign-proof"})
			case "foreign_session":
				ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: child.OwnerID, SessionKey: "another-session", Channel: "web", DeliveryID: "foreign-proof"})
			case "worker", "mixed_owner_worker":
				ctx = worker
				if scenario == "mixed_owner_worker" {
					ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: child.OwnerID, Channel: "web", DeliveryID: "mixed-proof"})
				}
			}
			if _, err := f.store.SaveVerifiedResult(ctx, claim); err != task.ErrUnauthorized {
				t.Fatal("untrusted verification became an owner verdict", err)
			}
		})
	}
	var count int
	if f.pool.QueryRow(f.ctx, `SELECT count(*) FROM task_results WHERE task_id=$1 AND verdict='verified'`, child.TaskID).Scan(&count) != nil || count != 0 {
		t.Fatal("denied verification wrote a result")
	}
}

func TestDelegationVerifiedPromotionDeniesChangedOrIncompleteEvidenceIntegration(t *testing.T) {
	for _, scenario := range []string{"unknown_effect", "partial_final", "stale_report", "deleted_original", "unfinished_child", "missing_action_evidence", "replayed_decision"} {
		t.Run(scenario, func(t *testing.T) {
			f := newDelegationGuardFixture(t)
			c, child := f.admit(t, "promotion-"+scenario)
			result, report, parents := f.complete(t, c, child, scenario == "unknown_effect", scenario == "partial_final")
			ref := f.report(t, "collected-"+scenario, child, report, parents)
			if _, err := f.manager.Collect(f.ctx, f.parent.TaskID, child.TaskID, result.ResultID, ref); err != nil {
				t.Fatal(err)
			}
			original := delegation.SourceRef{ID: parents[0].ID, Revision: parents[0].Revision, AccessRevision: parents[0].AccessRevision, Hash: parents[0].Hash}
			evidence := []string{DelegationSourceEvidence(original)}
			if scenario != "missing_action_evidence" {
				evidence = append(evidence, DelegationRecordEvidence(delegation.EvidenceRef{Kind: "action", ID: report.Actions[0].ActionID, Hash: validationHash}), DelegationRecordEvidence(report.Actions[0].Evidence[0]))
			}
			verified, err := f.store.SaveVerifiedResult(f.owner, task.Result{TaskID: child.TaskID, TaskRevision: child.Revision, RunID: result.RunID, Summary: "Synthetic owner proof for denial test", EvidenceRefs: evidence, VerifiedCriteria: child.CompletionCriteria, Verifier: "owner"})
			if err != nil {
				t.Fatal(err)
			}
			expectedReport, expectedState := ref, "result_ready"
			switch scenario {
			case "stale_report":
				expectedReport.Revision++
			case "deleted_original":
				if _, err := f.sources.Delete(f.owner, original.ID, "delete", original.Revision); err != nil {
					t.Fatal(err)
				}
			case "unfinished_child":
				if _, err := f.pool.Exec(f.ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES('unfinished-promotion',$1,'fixture','running','model_running','mode_1','fixture',clock_timestamp(),clock_timestamp(),$2,$3)`, child.SessionKey, child.TaskID, child.Revision); err != nil {
					t.Fatal(err)
				}
			case "replayed_decision":
				if _, err := f.manager.AcceptVerified(f.owner, f.parent.TaskID, child.TaskID, ref, verified.ResultID, []delegation.SourceRef{original}); err != nil {
					t.Fatal("first complete promotion denied", err)
				}
				expectedState = "accepted"
			}
			var before string
			if err := f.pool.QueryRow(f.ctx, `SELECT decision_evidence_json::text FROM task_delegations WHERE task_id=$1`, child.TaskID).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, err := f.manager.AcceptVerified(f.owner, f.parent.TaskID, child.TaskID, expectedReport, verified.ResultID, []delegation.SourceRef{original}); err == nil {
				t.Fatal("invalid promotion accepted", scenario)
			}
			var state, after string
			if err := f.pool.QueryRow(f.ctx, `SELECT state,decision_evidence_json::text FROM task_delegations WHERE task_id=$1`, child.TaskID).Scan(&state, &after); err != nil || state != expectedState || before != after {
				t.Fatal("denied promotion changed the collected decision", err)
			}
		})
	}
}
