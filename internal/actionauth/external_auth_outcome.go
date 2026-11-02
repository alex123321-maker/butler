package actionauth

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/externalauth"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
)

// Reuse the existing protected factual-report UI only after all execution has
// settled. This changes the journal assessment, never the callback result,
// canonical Input, charged budget, ticket or consumed provider state.
func (s *OwnerOutcomes) externalAuthReviewTx(ctx context.Context, tx pgx.Tx, e EffectRecord) (OwnerOutcomeReview, error) {
	a, ok := owneridentity.FromContext(ctx)
	b := e.Binding
	if !ok || a.OwnerID != b.OwnerID || !validExternalScope(b) || b.ToolName != externalauth.RequestTool || b.Effect != EffectCommit || e.Policy.RetrySafety != "unsafe" {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	var session string
	var live bool
	if tx.QueryRow(ctx, `SELECT s.session_key,o.lease_id<>'' AND o.expires_at>clock_timestamp() FROM runs r JOIN sessions s ON s.session_key=r.session_key JOIN session_ownership o ON o.session_key=s.session_key WHERE r.run_id=$1 AND s.user_id=$2 AND r.current_state IN ('completed','failed','cancelled','timed_out') FOR UPDATE OF o`, b.RunID, b.OwnerID).Scan(&session, &live) != nil || live {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	var id, scopeRaw, scopeDigest string
	var settled bool
	if tx.QueryRow(ctx, `SELECT q.id,q.scope::text,q.scope_digest,
 x.claimed_at<clock_timestamp()-interval '75 seconds' AND bs.expires_at<clock_timestamp()-interval '35 seconds'
 AND NOT EXISTS(SELECT 1 FROM accepted_inputs i WHERE i.run_id=q.run_id AND i.status IN ('pending','accepted','running','waiting_for_user','blocked'))
 FROM browser_external_auth_requests q JOIN browser_external_auth_exchanges x ON x.request_id=q.id JOIN browser_external_auth_results r ON r.request_id=q.id AND r.outcome='unknown'
 JOIN server_browser_sessions bs ON bs.browser_session_id=q.browser_session_id AND bs.run_id=q.run_id AND bs.owner_id=q.owner_id AND bs.task_id=q.task_id
 JOIN action_effect_dispatches d ON d.action_id=x.effect_id AND d.authorization_id=$2 AND d.execution_action_id=x.effect_id AND d.outcome='unknown' AND d.finished_at IS NOT NULL
 WHERE x.effect_id=$1 AND q.owner_id=$3 AND q.run_id=$4 AND q.session_key=$5 AND q.task_id=$6 AND q.task_revision=$7 AND q.constraints_revision=$8`, e.ActionID, e.AuthorizationID, b.OwnerID, b.RunID, session, b.TaskID, b.TaskRevision, b.ConstraintsRevision).Scan(&id, &scopeRaw, &scopeDigest, &settled) != nil || !settled {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	var scope externalauth.Scope
	if json.Unmarshal([]byte(scopeRaw), &scope) != nil || scope != *b.ExternalAuth || b.ToolCallID != "_external_auth_"+id || b.ActionID != StableActionID(b.RunID, b.ToolCallID) || e.Fingerprint != digest(b.ActionHash+"\x00"+id+"\x00"+scopeDigest) {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	locked, err := effectIn(ctx, tx, e.ActionID, true)
	if err != nil || locked.State != StateUnknown || locked.Fingerprint != e.Fingerprint {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	if _, err = tx.Exec(ctx, `UPDATE browser_owner_outcome_reviews SET state='unresolved',decision_digest='expired',decided_at=clock_timestamp() WHERE effect_id=$1 AND state='pending' AND expires_at<=clock_timestamp()`, e.ActionID); err != nil {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	r := OwnerOutcomeReview{OwnerID: b.OwnerID, SessionKey: session, TaskID: b.TaskID, TaskRevision: b.TaskRevision, ConstraintsRevision: b.ConstraintsRevision, ID: "review_" + randomID(), EffectID: e.ActionID, Fingerprint: e.Fingerprint, RunID: b.RunID, ToolCallID: b.ToolCallID, SourceKind: "historical_expired", State: "pending", Revision: 1, ResourceHint: scope.RPURL + " [external Google; historical factual report]", ExpiresAt: time.Now().UTC().Add(20 * time.Minute)}
	_, err = tx.Exec(ctx, `INSERT INTO browser_owner_outcome_reviews(review_id,effect_id,fingerprint,execution_action_id,owner_id,run_id,session_key,tool_call_id,browser_session_id,task_id,task_revision,constraints_revision,source_kind,resource_hint,expires_at) VALUES($1,$2,$3,$2,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, r.ID, r.EffectID, r.Fingerprint, r.OwnerID, r.RunID, r.SessionKey, r.ToolCallID, scope.Browser, r.TaskID, r.TaskRevision, r.ConstraintsRevision, r.SourceKind, r.ResourceHint, r.ExpiresAt)
	if err != nil {
		return OwnerOutcomeReview{}, ErrOwnerOutcome
	}
	return r, nil
}
