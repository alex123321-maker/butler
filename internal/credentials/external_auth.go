package credentials

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/externalauth"
	"github.com/butler/butler/internal/jsonstrict"
	"github.com/butler/butler/internal/security/egress"
	"github.com/jackc/pgx/v5"
)

// ExternalAuth is private Core composition, not a runtime/secret resolver.
// Broker can create requests against the retained non-secret active profile;
// only Core receives the operator client configuration and provider object.
type ExternalAuth struct {
	profile externalauth.Profile
	google  externalProvider
	probe   *http.Client
	budgets *budget.Controller
}

type externalProvider interface {
	Launch(externalauth.Challenge) (externalauth.PrivateLaunch, error)
	Exchange(context.Context, string, externalauth.Challenge, time.Time) (externalauth.VerifiedIdentity, error)
}

func (v *Vault) ConfigureExternalAuth(ctx context.Context, cfg externalauth.GoogleConfig, rp, verify string, budgets *budget.Controller) error {
	g, e := externalauth.NewGoogle(cfg)
	p, pe := externalauth.ProfileFor(cfg, rp, verify)
	if e != nil || pe != nil || budgets == nil {
		return ErrVault
	}
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return ErrVault
	}
	defer tx.Rollback(ctx)
	// Operator-only startup serialization, never a model-selected profile.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(637163111)`); e != nil {
		return ErrVault
	}
	raw, _ := json.Marshal(p)
	if _, e = tx.Exec(ctx, `UPDATE browser_external_auth_profiles SET active=FALSE WHERE active AND fingerprint<>$1`, p.ClientFingerprint); e != nil {
		return ErrVault
	}
	var same bool
	if tx.QueryRow(ctx, `INSERT INTO browser_external_auth_profiles(fingerprint,profile) VALUES($1,$2::jsonb) ON CONFLICT(fingerprint) DO UPDATE SET active=TRUE RETURNING profile=$2::jsonb`, p.ClientFingerprint, string(raw)).Scan(&same) != nil || !same || tx.Commit(ctx) != nil {
		return ErrVault
	}
	v.external = &ExternalAuth{profile: p, google: g, probe: egress.NewClient(egress.Policy{}, 15*time.Second), budgets: budgets}
	return nil
}

func (v *Vault) externalProfileTx(ctx context.Context, tx pgx.Tx) (externalauth.Profile, error) {
	var raw string
	var p externalauth.Profile
	if tx.QueryRow(ctx, `SELECT profile::text FROM browser_external_auth_profiles WHERE active FOR SHARE`).Scan(&raw) != nil || jsonstrict.Decode([]byte(raw), &p) != nil {
		return p, ErrVault
	}
	return p, nil
}

type externalRequest struct {
	p                                                                Principal
	id, input, call, source, record, profile, purpose, digest, state string
	revision, epoch                                                  int64
	scope                                                            externalauth.Scope
	created, expires                                                 time.Time
	executionDeadline                                                time.Time // local monotonic bound; never serialized or retained
}

func externalDigest(q externalRequest) string {
	raw, _ := json.Marshal([]any{externalauth.Consent, q.p, q.id, q.input, q.call, q.source, q.record, q.revision, q.profile, q.purpose, q.epoch, q.scope})
	return digestString(string(raw))
}

// CreateExternalRequest is only invoked by the already-dispatched Broker
// capability. Exact retained arguments/source and live Input are mandatory.
func (v *Vault) CreateExternalRequest(ctx context.Context, p Principal, callID string, dispatch actionauth.DispatchIdentity, args string) (AccessRequest, error) {
	a, e := externalauth.ParseRequest(args)
	if e != nil || p.TaskID == "" || callID == "" || dispatch.ActionID == "" || dispatch.Fingerprint == "" {
		return AccessRequest{}, ErrVault
	}
	// DispatchIdentity.ActionID is the permanent effect, not this execution's
	// intent ID. Bind both identities and the trusted fingerprint explicitly.
	actionID := actionauth.StableActionID(p.RunID, callID)
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return AccessRequest{}, ErrVault
	}
	defer tx.Rollback(ctx)
	if currentTx(ctx, tx, p) != nil {
		return AccessRequest{}, ErrVault
	}
	q := externalRequest{p: p, id: randomID("extq_"), call: callID, source: actionID, record: a.RecordID, purpose: a.Purpose, scope: a.Scope}
	profile, e := v.externalProfileTx(ctx, tx)
	if e != nil || !profile.Matches(a.Scope) {
		return AccessRequest{}, ErrVault
	}
	q.profile = profile.ClientFingerprint
	var binding string
	var b actionauth.ActionBinding
	if tx.QueryRow(ctx, `SELECT i.binding::text FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id JOIN action_effect_dispatches d ON d.execution_action_id=i.action_id AND d.authorization_id=e.authorization_id WHERE i.action_id=$1 AND i.run_id=$2 AND i.tool_call_id=$3 AND e.action_id=$4 AND e.fingerprint=$5 AND d.action_id=e.action_id AND e.state='executing' AND d.outcome='executing' FOR SHARE OF i,e,d`, actionID, p.RunID, callID, dispatch.ActionID, dispatch.Fingerprint).Scan(&binding) != nil || jsonstrict.Decode([]byte(binding), &b) != nil {
		return AccessRequest{}, ErrVault
	}
	h, e := actionauth.ArgumentsHash(args)
	if e != nil || b.ArgsHash != h || b.ToolName != externalauth.RequestTool || b.GoalID == "" || b.MandateID == "" || b.MandateRevision <= 0 || b.ExternalAuth == nil || *b.ExternalAuth != a.Scope || b.Effect != actionauth.EffectPrepare || b.OwnerID != p.OwnerID || b.TaskID != p.TaskID || b.TaskRevision != p.TaskRevision || b.ConstraintsRevision != p.ConstraintsRevision || len(b.CredentialRefs) != 0 || actionauth.CheckTaskActionTx(ctx, tx, b, time.Now()) != nil {
		return AccessRequest{}, ErrVault
	}
	commitBinding := b
	commitBinding.Effect = actionauth.EffectCommit
	if actionauth.CheckTaskActionTx(ctx, tx, commitBinding, time.Now()) != nil {
		return AccessRequest{}, ErrVault
	}
	if tx.QueryRow(ctx, `SELECT input_id FROM accepted_inputs WHERE run_id=$1 AND session_key=$2 AND owner_id=$3 AND task_id=$4 AND status='running' AND expires_at>clock_timestamp() AND claim_id<>'' AND claim_expires_at>clock_timestamp() FOR SHARE`, p.RunID, p.SessionKey, p.OwnerID, p.TaskID).Scan(&q.input) != nil {
		return AccessRequest{}, ErrVault
	}
	if tx.QueryRow(ctx, `SELECT revision FROM credential_vault WHERE id=$1 AND owner_id=$2 AND status='active' FOR SHARE`, q.record, p.OwnerID).Scan(&q.revision) != nil {
		return AccessRequest{}, ErrVault
	}
	var expiry time.Time
	if tx.QueryRow(ctx, `SELECT b.epoch,b.expires_at FROM server_browser_sessions b JOIN server_browser_pages p ON p.browser_session_id=b.browser_session_id WHERE b.browser_session_id=$1 AND b.owner_id=$2 AND b.run_id=$3 AND b.task_id=$4 AND b.page_mode AND b.mode='agent' AND b.expires_at>clock_timestamp() AND p.page_id=$5 AND p.document_id=$6 AND p.observation_version=$7 AND p.observed_epoch=b.epoch AND p.observed_url=$8 AND p.state='ready' AND p.runtime_version>0 FOR SHARE OF b,p`, a.Scope.Browser, q.p.OwnerID, q.p.RunID, q.p.TaskID, a.Scope.RPPage.ID, a.Scope.RPPage.DocumentID, a.Scope.RPPage.Version, a.Scope.RPURL).Scan(&q.epoch, &expiry) != nil {
		return AccessRequest{}, ErrVault
	}
	q.digest = externalDigest(q)
	raw, _ := json.Marshal(q.scope)
	e = tx.QueryRow(ctx, `INSERT INTO browser_external_auth_requests(id,owner_id,session_key,run_id,input_id,task_id,task_revision,constraints_revision,tool_call_id,source_action_id,record_id,record_revision,browser_session_id,rp_page_id,context_epoch,profile_fingerprint,scope,scope_digest,purpose,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17::jsonb,$18,$19,LEAST($20,clock_timestamp()+interval '10 minutes')) ON CONFLICT(run_id,tool_call_id) DO NOTHING RETURNING created_at,expires_at,state`, q.id, p.OwnerID, p.SessionKey, p.RunID, q.input, p.TaskID, p.TaskRevision, p.ConstraintsRevision, callID, actionID, q.record, q.revision, q.scope.Browser, q.scope.RPPage.ID, q.epoch, q.profile, string(raw), q.digest, q.purpose, expiry).Scan(&q.created, &q.expires, &q.state)
	if e == pgx.ErrNoRows {
		e = tx.QueryRow(ctx, `SELECT id,scope_digest,created_at,expires_at,state FROM browser_external_auth_requests WHERE run_id=$1 AND tool_call_id=$2 AND owner_id=$3 AND input_id=$4 AND task_id=$5 AND task_revision=$6 AND constraints_revision=$7 AND source_action_id=$8 AND record_id=$9 AND record_revision=$10 AND profile_fingerprint=$11 AND scope=$12::jsonb AND purpose=$13 AND context_epoch=$14`, p.RunID, callID, p.OwnerID, q.input, p.TaskID, p.TaskRevision, p.ConstraintsRevision, actionID, q.record, q.revision, q.profile, string(raw), q.purpose, q.epoch).Scan(&q.id, &q.digest, &q.created, &q.expires, &q.state)
	}
	if e != nil || q.digest != externalDigest(q) {
		return AccessRequest{}, ErrVault
	}
	if _, e = tx.Exec(ctx, `INSERT INTO browser_external_auth_notices(request_id) VALUES($1) ON CONFLICT DO NOTHING`, q.id); e != nil {
		return AccessRequest{}, ErrVault
	}
	if v.audit(ctx, tx, p.OwnerID, q.record, q.id, "external_auth_requested") != nil || tx.Commit(ctx) != nil {
		return AccessRequest{}, ErrVault
	}
	return q.metadata(), nil
}

func (q externalRequest) metadata() AccessRequest {
	s := q.scope
	return AccessRequest{ID: q.id, Kind: "external_login", RecordID: q.record, TargetOrigin: "https://accounts.google.com", BrowserSessionID: q.scope.Browser, Purpose: q.purpose, Fields: []string{}, State: q.state, ExpiresAt: q.expires, ScopeDigest: q.digest, ContextEpoch: q.epoch, ExternalScope: &s}
}

func validExternalID(id string) bool {
	if len(id) != 69 || !strings.HasPrefix(id, "extq_") || strings.ToLower(id) != id {
		return false
	}
	b, e := hex.DecodeString(id[5:])
	return e == nil && len(b) == 32
}
