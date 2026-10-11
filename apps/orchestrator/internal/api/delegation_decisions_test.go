package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/owneridentity"
)

type decisionBoundary struct{ calls int }

func (d *decisionBoundary) Decide(ctx context.Context, parent, child string, q task.DelegationDecision) (task.DelegationRecord, error) {
	actor, ok := owneridentity.FromContext(ctx)
	if !ok || actor.OwnerID != "decision-owner" || parent != "parent" || child != "child" || q.Accept {
		return task.DelegationRecord{}, task.ErrUnauthorized
	}
	d.calls++
	return task.DelegationRecord{}, errors.New("private source and account error must not be returned")
}
func (d *decisionBoundary) AcceptVerified(ctx context.Context, parent, child string, ref delegation.SourceRef, result string, evidence []delegation.SourceRef) (task.DelegationRecord, error) {
	d.calls++
	return task.DelegationRecord{}, task.ErrUnauthorized
}

func TestDelegationOwnerDecisionRejectsAuthorityAndAmbiguousBodies(t *testing.T) {
	ref := delegation.SourceRef{ID: "report", Revision: 1, AccessRevision: 1, Hash: strings.Repeat("a", 64)}
	raw, _ := json.Marshal(map[string]any{"kind": "reject", "report_source": ref, "evidence_refs": []delegation.SourceRef{ref}})
	base := string(raw)
	for _, scenario := range []string{"anonymous", "duplicate_auth", "query", "unknown_authority", "duplicate_kind", "case_alias", "null_source", "source_alias", "unverified_accept", "reject_with_result", "oversize", "valid_reject"} {
		t.Run(scenario, func(t *testing.T) {
			token := strings.Repeat("owner-proof-", 4)
			boundary := &decisionBoundary{}
			handler := DelegationOwnerDecisions(NewOwnerControlAuth(token, "decision-owner"), boundary)
			body, target, want := base, "/api/v3/delegation-decisions/parent/child", http.StatusBadRequest
			switch scenario {
			case "anonymous", "duplicate_auth":
				want = http.StatusUnauthorized
			case "query":
				target += "?owner_id=decision-owner"
			case "unknown_authority":
				body = strings.TrimSuffix(body, "}") + `,"owner_id":"decision-owner"}`
			case "duplicate_kind":
				body = strings.Replace(body, `"kind":"reject"`, `"kind":"accept_verified","kind":"reject"`, 1)
			case "case_alias":
				body = strings.Replace(body, `"kind"`, `"Kind"`, 1)
			case "null_source":
				var fields map[string]any
				_ = json.Unmarshal(raw, &fields)
				fields["report_source"] = nil
				b, _ := json.Marshal(fields)
				body = string(b)
			case "source_alias":
				body = strings.Replace(body, `"source_id"`, `"Source_id"`, 1)
			case "unverified_accept":
				body = strings.Replace(body, `"reject"`, `"accept_verified"`, 1)
			case "reject_with_result":
				body = strings.TrimSuffix(body, "}") + `,"verified_result_id":"forged"}`
			case "oversize":
				body += strings.Repeat(" ", 32768)
			case "valid_reject":
				want = http.StatusConflict
			}
			r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
			if scenario != "anonymous" {
				r.Header.Set("Authorization", "Bearer "+token)
			}
			if scenario == "duplicate_auth" {
				r.Header.Add("Authorization", "Bearer "+token)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			calls := 0
			if scenario == "valid_reject" {
				calls = 1
			}
			if w.Code != want || boundary.calls != calls || strings.Contains(w.Body.String(), "private source") || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("protected decision boundary failed", w.Code, boundary.calls, w.Body.String())
			}
		})
	}
}
