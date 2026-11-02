package actionauth

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/externalauth"
	"github.com/butler/butler/internal/jsonstrict"
	"github.com/jackc/pgx/v5"
)

const ExternalAuthVerification = "external_google_rp_v1"

// BeginExternalAuthTx owns the private authentication suboperation's journal
// and separate finite budget reservation. A model cannot obtain a generic
// dispatch grant for this commit binding: the public request contract remains
// prepare-only. The owning credential kernel must validate state/cookie and
// current source authority before calling this on the same transaction.
func BeginExternalAuthTx(ctx context.Context, tx pgx.Tx, id string, c *budget.Controller) (string, error) {
	if c == nil || len(id) != 69 || !strings.HasPrefix(id, "extq_") {
		return "", externalauth.ErrProvider
	}
	var raw, digestScope string
	if tx.QueryRow(ctx, `SELECT i.binding::text,q.scope_digest FROM browser_external_auth_requests q JOIN action_intents i ON i.action_id=q.source_action_id JOIN browser_external_auth_launches l ON l.request_id=q.id WHERE q.id=$1 AND q.state='approved' AND q.expires_at>clock_timestamp()`, id).Scan(&raw, &digestScope) != nil {
		return "", externalauth.ErrProvider
	}
	var b ActionBinding
	if jsonstrict.Decode([]byte(raw), &b) != nil || !validExternalScope(b) || b.GoalID == "" || b.MandateID == "" || b.Effect != EffectPrepare {
		return "", externalauth.ErrProvider
	}
	b.Effect = EffectCommit
	if CheckTaskActionTx(ctx, tx, b, time.Now()) != nil {
		return "", externalauth.ErrProvider
	}
	b.ToolCallID = "_external_auth_" + id
	b.ActionID = StableActionID(b.RunID, b.ToolCallID)
	b.ActionHash = ""
	encoded, _ := json.Marshal(b)
	b.ActionHash = digest(string(encoded))
	encoded, _ = json.Marshal(b)
	fingerprint := digest(b.ActionHash + "\x00" + id + "\x00" + digestScope)
	identity := budget.ToolIdentity{RunID: b.RunID, MandateID: b.MandateID, EffectID: b.ActionID, ToolName: b.ToolName, ArgsHash: b.ArgsHash, ContractHash: b.ContractHash, Effect: string(b.Effect), TaskRevision: b.TaskRevision, ConstraintsRevision: b.ConstraintsRevision}
	if c.ReserveToolTx(ctx, tx, identity) != nil {
		return "", externalauth.ErrProvider
	}
	policy := EffectPolicy{RetrySafety: "unsafe", VerificationMethod: ExternalAuthVerification, ScopeOrigin: b.ExternalAuth.RPURL, VerificationURL: b.ExternalAuth.VerifyURL}
	praw, _ := json.Marshal(policy)
	claim := "external_auth_claim_" + id[5:]
	if _, e := tx.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state,authorization_id,dispatch_outcome) VALUES($1,$2,$3::jsonb,$4::jsonb,'executing',$5,'executing')`, b.ActionID, fingerprint, string(encoded), string(praw), claim); e != nil {
		return "", externalauth.ErrProvider
	}
	if _, e := tx.Exec(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id,effect_fingerprint,effect_policy) SELECT $1,$2,$3,$4,$5::jsonb,TRUE,q.expires_at,$1,$6,$7::jsonb FROM browser_external_auth_requests q WHERE q.id=$8`, b.ActionID, b.RunID, b.ToolCallID, b.ActionHash, string(encoded), fingerprint, string(praw), id); e != nil {
		return "", externalauth.ErrProvider
	}
	if _, e := tx.Exec(ctx, `INSERT INTO action_effect_dispatches(authorization_id,action_id,execution_action_id,outcome) VALUES($1,$2,$2,'executing')`, claim, b.ActionID); e != nil {
		return "", externalauth.ErrProvider
	}
	return b.ActionID, nil
}

// FinishExternalAuthTx accepts only the owning immutable RP result, never a
// boolean from a browser script/model. It preserves spending and the claim.
func FinishExternalAuthTx(ctx context.Context, tx pgx.Tx, id string) error {
	var effect, outcome, scopeDigest string
	var auth, anonymous bool
	var at time.Time
	if tx.QueryRow(ctx, `SELECT x.effect_id,r.outcome,r.rp_authenticated_read,r.rp_anonymous_challenge,q.scope_digest,r.finished_at FROM browser_external_auth_exchanges x JOIN browser_external_auth_results r USING(request_id) JOIN browser_external_auth_requests q ON q.id=x.request_id WHERE q.id=$1 AND q.state='approved'`, id).Scan(&effect, &outcome, &auth, &anonymous, &scopeDigest, &at) != nil {
		return externalauth.ErrProvider
	}
	r, e := effectIn(ctx, tx, effect, true)
	if e != nil || r.State != StateExecuting || r.Policy.VerificationMethod != ExternalAuthVerification || !validExternalScope(r.Binding) {
		return externalauth.ErrProvider
	}
	verdict := StateUnknown
	dispatch := "unknown"
	if outcome == "verified" {
		var identity bool
		if !auth || !anonymous || tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM browser_external_auth_verified_identities WHERE request_id=$1)`, id).Scan(&identity) != nil || !identity {
			return externalauth.ErrProvider
		}
		verdict = StateConfirmed
		dispatch = "dispatch_completed"
	} else if outcome == "unverified" {
		var identity bool
		if tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM browser_external_auth_verified_identities WHERE request_id=$1)`, id).Scan(&identity) != nil || identity {
			return externalauth.ErrProvider
		}
		verdict = StateAbsent
		dispatch = "dispatch_completed"
	} else if outcome != "unknown" {
		return externalauth.ErrProvider
	}
	var recorded time.Time
	if tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&recorded) != nil {
		return externalauth.ErrProvider
	}
	observed := EffectEvidence{EvidenceID: "evidence_" + randomID(), ActionID: effect, SourceActionID: r.Binding.ActionID, SourceRunID: r.Binding.RunID, SourceToolCallID: r.Binding.ToolCallID, Source: "credential_kernel", Method: ExternalAuthVerification, Verdict: verdict, ResultDigest: digest(id + "\x00" + scopeDigest + "\x00" + outcome), ObservedAt: at, RecordedAt: recorded}
	if insertEvidence(ctx, tx, observed) != nil {
		return externalauth.ErrProvider
	}
	if _, e = tx.Exec(ctx, `UPDATE action_effects SET state=$2,dispatch_outcome=$3,updated_at=clock_timestamp() WHERE action_id=$1`, effect, string(verdict), dispatch); e != nil {
		return externalauth.ErrProvider
	}
	if _, e = tx.Exec(ctx, `UPDATE action_effect_dispatches SET outcome=$2,finished_at=clock_timestamp() WHERE authorization_id=$1 AND outcome='executing'`, r.AuthorizationID, dispatch); e != nil {
		return externalauth.ErrProvider
	}
	return nil
}
