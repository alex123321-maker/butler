package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	"github.com/butler/butler/internal/owneridentity"
)

// DelegationDecisions controls already collected reports. It cannot submit an
// owner verification, create children or acquire a worker/dispatch lease.
type DelegationDecisions interface {
	Decide(context.Context, string, string, task.DelegationDecision) (task.DelegationRecord, error)
	AcceptVerified(context.Context, string, string, delegation.SourceRef, string, []delegation.SourceRef) (task.DelegationRecord, error)
}

const delegationDecisionsPrefix = "/api/v3/delegation-decisions/"

func DelegationOwnerDecisions(auth *OwnerControlAuth, decisions DelegationDecisions) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		actor, ok := auth.Authenticate(r)
		if !ok || len(r.Header.Values("Authorization")) != 1 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "owner control authentication required"})
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, delegationDecisionsPrefix), "/")
		if !strings.HasPrefix(r.URL.Path, delegationDecisionsPrefix) || len(parts) != 2 || !delegationDecisionID(parts[0]) || !delegationDecisionID(parts[1]) || r.URL.RawQuery != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "parent and child task references required"})
			return
		}
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32768))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bounded delegation decision required"})
			return
		}
		if _, err = actionauth.ArgumentsHash(string(raw)); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "unambiguous delegation decision required"})
			return
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "fixed delegation decision required"})
			return
		}
		var kind, resultID string
		var report delegation.SourceRef
		var refs []json.RawMessage
		valid := json.Unmarshal(fields["kind"], &kind) == nil && decodeDelegationDecisionSource(fields["report_source"], &report) && json.Unmarshal(fields["evidence_refs"], &refs) == nil && len(refs) >= 1 && len(refs) <= 32
		evidence := make([]delegation.SourceRef, len(refs))
		for i, ref := range refs {
			valid = decodeDelegationDecisionSource(ref, &evidence[i]) && valid
		}
		if kind == "accept_verified" {
			valid = valid && len(fields) == 4 && json.Unmarshal(fields["verified_result_id"], &resultID) == nil && delegationDecisionID(resultID)
		} else {
			valid = valid && kind == "reject" && len(fields) == 3
		}
		if !valid {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "exact collected report and evidence required"})
			return
		}
		if decisions == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "delegation owner controls unavailable"})
			return
		}
		ctx := owneridentity.WithActor(r.Context(), actor)
		var result task.DelegationRecord
		if kind == "accept_verified" {
			result, err = decisions.AcceptVerified(ctx, parts[0], parts[1], report, resultID, evidence)
		} else {
			result, err = decisions.Decide(ctx, parts[0], parts[1], task.DelegationDecision{Accept: false, Report: report, Evidence: evidence})
		}
		if err != nil {
			// Private store/source errors never become an owner-channel diagnostic.
			writeJSON(w, http.StatusConflict, map[string]string{"error": "current independently verified delegation evidence required"})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})
}

func delegationDecisionID(id string) bool {
	if len(id) < 1 || len(id) > 512 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == ':' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

func decodeDelegationDecisionSource(raw json.RawMessage, out *delegation.SourceRef) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 4 {
		return false
	}
	for _, key := range []string{"source_id", "revision", "access_revision", "content_hash"} {
		if _, ok := fields[key]; !ok {
			return false
		}
	}
	return json.Unmarshal(raw, out) == nil && delegation.ValidSource(*out)
}
