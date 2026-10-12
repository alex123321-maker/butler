package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/delegation"
)

type fullHighWidthAcceptance struct {
	Stage                          string                           `json:"stage"`
	ResolutionPolicy               string                           `json:"resolution_policy"`
	ParentFinalAvailable           bool                             `json:"parent_final_available"`
	ParentFinalUnverified          bool                             `json:"parent_final_unverified"`
	ParentFinalComplete            bool                             `json:"parent_final_complete"`
	ParentFinalPrivacyRedacted     bool                             `json:"parent_final_privacy_redacted"`
	ParentReceipt                  *nativeReceiptProjectionEvidence `json:"parent_receipt,omitempty"`
	StructuredOwningReportsChecked bool                             `json:"structured_owning_reports_checked"`
	IndependentOriginalChecks      bool                             `json:"independent_original_checks"`
	ConflictingOriginalsRetained   bool                             `json:"conflicting_originals_retained"`
	AcceptedChild                  string                           `json:"accepted_child,omitempty"`
	RejectedChild                  string                           `json:"rejected_child,omitempty"`
	PreviousResultAndReportKept    bool                             `json:"previous_result_and_report_kept"`
	OwnerAPIAuthenticated          bool                             `json:"owner_api_authenticated"`
	ChildFinalsStayUnverified      bool                             `json:"child_finals_stay_unverified"`
	ChildTasksStayActive           bool                             `json:"child_tasks_stay_active"`
}

func fullHighParentPrompt(args []delegation.AdmissionArgs) string {
	return "Создай ровно два независимых поручения: по одному последовательному вызову task.delegate для каждого JSON ниже. Аргументы неизменяемы: не меняй ссылки, бюджет, срок и не создавай третье поручение. Не отвечай на исследовательский вопрос сам. После двух успешных результатов инструмента верни только один JSON-объект с единственным ключом admitted_task_ids и массивом ровно из двух фактических task_id, полученных в result.task_id этих результатов. Используй реальные идентификаторы, не имена источников, run_id или input_id. Не добавляй Markdown, обратные кавычки, вводный текст, комментарии или другие поля. Формат конечного ответа: {\"admitted_task_ids\":[\"первый фактический task_id\",\"второй фактический task_id\"]}.\n" + string(nativeJSON(args[0])) + "\n" + string(nativeJSON(args[1]))
}

func checkFullHighParentReceipts(t *testing.T, f *nativeScenarioFixture, parent, run string, evidence *fullHighWidthAcceptance) {
	t.Helper()
	rows, err := f.pool.Query(f.ctx, `SELECT task_id FROM task_delegations WHERE parent_task_id=$1 ORDER BY task_id`, parent)
	if err != nil {
		t.Fatal("actual parent child identities unavailable")
	}
	var children []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			rows.Close()
			t.Fatal("actual parent child identities unavailable")
		}
		children = append(children, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		t.Fatal("actual parent child identities unavailable")
	}
	final, err := completion.New(f.pool).GetByRun(f.ctx, run)
	evidence.ParentFinalAvailable = err == nil
	if err != nil {
		t.Fatal("parent final lookup failed; no model retry")
	}
	evidence.ParentFinalUnverified = final.Verdict == "unverified"
	evidence.ParentFinalComplete = final.AnswerState == "final"
	evidence.ParentFinalPrivacyRedacted = final.PrivacyRedacted
	check := nativeReceiptProjection(final.Content, children)
	evidence.ParentReceipt = &check
	if !evidence.ParentFinalUnverified || !evidence.ParentFinalComplete || final.PrivacyRedacted || check.Reason != "passed" {
		t.Fatal("parent exact receipt gate failed; content-free reasons retained; no model retry")
	}
}

// This verifier reads owning immutable finals/current Catalog witnesses and the
// ordinary report guard. The model cannot supply a verdict or owner authority.
// A is selected by the fixture's predeclared owner policy; B remains a correct
// source-relative observation, not a disproved universal fact.
func verifyFullHighOwnerDecisions(t *testing.T, f *nativeScenarioFixture, admission *DelegationAdmission, width nativeWidthEvidence, evidence *fullHighWidthAcceptance) {
	t.Helper()
	owner := nativeOwner(f.ctx, "independent-native-verification", "")
	parent, err := f.tasks.Get(f.ctx, width.TaskID)
	if err != nil {
		t.Fatal("parent verification metadata unavailable")
	}
	guard, err := NewDelegationGuards(f.sources, admission.reader)
	if err != nil {
		t.Fatal("owning report verifier unavailable")
	}
	const ownerToken = "isolated-high-owner-decision-authentication-token"
	server := httptest.NewServer(api.DelegationOwnerDecisions(api.NewOwnerControlAuth(ownerToken, nativeFixtureOwner), admission.Manager))
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var accepted, rejected int
	for i, childID := range width.ChildTaskIDs {
		child, err := f.tasks.Get(f.ctx, childID)
		if err != nil {
			t.Fatal("child verification metadata unavailable")
		}
		record, err := admission.Manager.Get(f.ctx, childID)
		if err != nil || record.Contract.State != "result_ready" || record.Report == nil || *record.Report != width.ReportSourceRefs[i] || record.ResultID != width.ChildResultIDs[i] || len(record.Contract.Sources) != 1 || len(record.Contract.Permissions) != 0 {
			t.Fatal("current collected child identity unavailable")
		}
		results, err := f.tasks.Results(f.ctx, childID)
		if err != nil {
			t.Fatal("owning child results unavailable")
		}
		var original task.Result
		for _, result := range results {
			if result.ResultID == record.ResultID {
				original = result
			}
		}
		if original.ResultID == "" || original.Verdict != "unverified" || original.RunID != width.ChildRunIDs[i] {
			t.Fatal("original collected result changed")
		}
		tx, err := f.pool.Begin(owner)
		if err != nil {
			t.Fatal("independent report transaction unavailable")
		}
		actual, err := guard.reportTx(owner, tx, parent, child, original, *record.Report, record.Contract.Sources)
		_ = tx.Rollback(context.Background())
		if err != nil || len(actual.actions) != 0 || len(actual.report.Actions) != 0 || actual.report.ResultID != original.ResultID || actual.final.RunID != original.RunID || actual.final.TaskID != childID || actual.final.AnswerState != "final" || actual.final.PrivacyRedacted || actual.used.ModelTurns != 1 || len(actual.reservations) != 1 {
			t.Fatal("actual report/action/budget verification failed")
		}
		for reason := range actual.reasons {
			if reason != "verification_missing" {
				t.Fatal("unsupported child uncertainty blocks independent acceptance")
			}
		}
		contractBody, ok := actual.sources[record.Contract.ContractSource.ID]
		if !ok {
			t.Fatal("current typed numeric contract unavailable")
		}
		ref, applicable, err := delegation.NumericResearchSource([]byte(contractBody.Body))
		if err != nil || !applicable || ref != record.Contract.Sources[0] {
			t.Fatal("current selected numeric contract mismatch")
		}
		witness, ok := actual.sources[ref.ID]
		if !ok || guardSource(witness) != ref || delegation.VerifyNumericObservation([]byte(actual.finalText), ref, []byte(witness.Body)) != nil {
			t.Fatal("independent original numeric verification failed")
		}
		evidenceRefs := []string{DelegationSourceEvidence(ref)}
		for _, r := range actual.report.Evidence {
			evidenceRefs = append(evidenceRefs, DelegationRecordEvidence(r))
		}
		verified, err := f.tasks.SaveVerifiedResult(owner, task.Result{TaskID: childID, TaskRevision: child.Revision, RunID: original.RunID, Summary: "Source-relative numeric observation independently checked against current immutable original", EvidenceRefs: evidenceRefs, VerifiedCriteria: child.CompletionCriteria, Verifier: "owner"})
		if err != nil {
			t.Fatal("independent owner result persistence failed")
		}
		chooseA := ref == f.args[0].Sources[0]
		payload := map[string]any{"kind": "reject", "report_source": *record.Report, "evidence_refs": []delegation.SourceRef{ref}}
		if chooseA {
			payload["kind"], payload["verified_result_id"] = "accept_verified", verified.ResultID
		}
		request, err := http.NewRequestWithContext(f.ctx, http.MethodPost, server.URL+"/api/v3/delegation-decisions/"+width.TaskID+"/"+childID, bytes.NewReader(nativeJSON(payload)))
		if err != nil {
			t.Fatal("bounded owner decision request unavailable")
		}
		request.Header.Set("Authorization", "Bearer "+ownerToken)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal("actual owner decision request failed")
		}
		var decided task.DelegationRecord
		decodeErr := json.NewDecoder(response.Body).Decode(&decided)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || decodeErr != nil || decided.Decision == nil || decided.Decision.Accept != chooseA {
			t.Fatal("actual independently checked owner decision failed")
		}
		if chooseA {
			if decided.Contract.State != "accepted" || decided.ResultID != verified.ResultID || decided.Report == nil || *decided.Report == *record.Report {
				t.Fatal("owner acceptance did not preserve independent result identity")
			}
			var previousReport, previousResult string
			if f.pool.QueryRow(f.ctx, `SELECT decision_evidence_json->'previous_report_source'->>'source_id',decision_evidence_json->>'previous_result_id' FROM task_delegations WHERE task_id=$1`, childID).Scan(&previousReport, &previousResult) != nil || previousReport != record.Report.ID || previousResult != original.ResultID {
				t.Fatal("owner acceptance discarded original result/report")
			}
			accepted++
			evidence.AcceptedChild = childID
		} else {
			if decided.Contract.State != "rejected" || decided.ResultID != original.ResultID || decided.Report == nil || *decided.Report != *record.Report {
				t.Fatal("source-selection rejection replaced original result/report")
			}
			rejected++
			evidence.RejectedChild = childID
		}
		kept, err := completion.New(f.pool).GetByRun(f.ctx, original.RunID)
		if err != nil || kept.Verdict != "unverified" || kept.ContentHash != actual.final.ContentHash || kept.Content != actual.finalText {
			t.Fatal("owner decision rewrote immutable model final")
		}
		currentChild, err := f.tasks.Get(f.ctx, childID)
		if err != nil || currentChild.Status != "active" {
			t.Fatal("source-relative decision completed child task")
		}
	}
	if accepted != 1 || rejected != 1 || f.corpus.Sources[0].ExpectedDate != f.corpus.Sources[1].ExpectedDate || f.corpus.Sources[0].ExpectedValue == f.corpus.Sources[1].ExpectedValue {
		t.Fatal("bounded conflicting-original selection incomplete")
	}
	evidence.StructuredOwningReportsChecked = true
	evidence.IndependentOriginalChecks = true
	evidence.ConflictingOriginalsRetained = true
	evidence.PreviousResultAndReportKept = true
	evidence.OwnerAPIAuthenticated = true
	evidence.ChildFinalsStayUnverified = true
	evidence.ChildTasksStayActive = true
}
