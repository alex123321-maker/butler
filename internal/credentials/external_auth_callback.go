package credentials

import (
	"context"
	"crypto/subtle"
	"io"
	"net/http"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/jsonstrict"
)

// ExternalCallbackCookieName identifies the expected per-attempt browser
// cookie. It grants nothing and returns no private cookie/state material.
func (v *Vault) ExternalCallbackCookieName(ctx context.Context, state string) (string, error) {
	if !externalOpaque(state) {
		return "", ErrVault
	}
	var id string
	if v.Pool.QueryRow(ctx, `SELECT request_id FROM browser_external_auth_uses WHERE state_hash=$1`, digestString(state)).Scan(&id) != nil || !validExternalID(id) {
		return "", ErrVault
	}
	return externalCookieName(externalRequest{id: id}), nil
}

func (v *Vault) claimExternalExchange(ctx context.Context, state, cookie string) (externalRequest, externalPrivate, error) {
	if !externalOpaque(state) || !externalOpaque(cookie) || v.external == nil {
		return externalRequest{}, externalPrivate{}, ErrVault
	}
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return externalRequest{}, externalPrivate{}, ErrVault
	}
	defer tx.Rollback(ctx)
	var id string
	if tx.QueryRow(ctx, `SELECT request_id FROM browser_external_auth_uses WHERE state_hash=$1`, digestString(state)).Scan(&id) != nil {
		return externalRequest{}, externalPrivate{}, ErrVault
	}
	// Serialize competing callbacks before acquiring shared Task/Input locks.
	// A losing replay must not hold those locks while the winner reserves its
	// budget, which needs the Task row exclusively. The claim remains durable.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,637163112))`, id); e != nil {
		return externalRequest{}, externalPrivate{}, ErrVault
	}
	q, e := v.loadExternalTx(ctx, tx, id)
	var spent bool
	if e == nil && tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM browser_external_auth_exchanges WHERE request_id=$1)`, id).Scan(&spent) != nil {
		return externalRequest{}, externalPrivate{}, ErrVault
	}
	if spent {
		return externalRequest{}, externalPrivate{}, ErrVault
	}
	if e != nil || q.state != "approved" || v.externalCurrentTx(ctx, tx, q, true) != nil || v.external.profile.ClientFingerprint != q.profile {
		return externalRequest{}, externalPrivate{}, ErrVault
	}
	var cipher, cookieHash string
	if tx.QueryRow(ctx, `SELECT u.private_cipher,u.cookie_hash FROM browser_external_auth_uses u JOIN browser_external_auth_launches l USING(request_id) WHERE u.request_id=$1`, id).Scan(&cipher, &cookieHash) != nil || subtle.ConstantTimeCompare([]byte(cookieHash), []byte(digestString(cookie))) != 1 {
		return externalRequest{}, externalPrivate{}, ErrVault
	}
	p, e := v.openExternal(q, cipher)
	if e != nil || subtle.ConstantTimeCompare([]byte(state), []byte(p.State)) != 1 || subtle.ConstantTimeCompare([]byte(cookie), []byte(p.Cookie)) != 1 {
		return externalRequest{}, externalPrivate{}, ErrVault
	}
	effect, e := actionauth.BeginExternalAuthTx(ctx, tx, id, v.external.budgets)
	if e != nil {
		// No provider operation has started and this transaction owns no claim.
		// Close this attempt rather than leave an unfinishable browser wait.
		_ = tx.Rollback(ctx)
		_ = v.closeExternal(ctx, q.p.OwnerID, id, q.digest, "rejected")
		return externalRequest{}, externalPrivate{}, ErrVault
	}
	deadline, e := v.external.budgets.ToolDeadlineTx(ctx, tx, q.p.OwnerID, effect)
	anchor := time.Now()
	var dbNow time.Time
	if e != nil || tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&dbNow) != nil || !deadline.After(dbNow) {
		return externalRequest{}, externalPrivate{}, ErrVault
	}
	// PostgreSQL owns budget time. Convert its remaining allowance to a local
	// monotonic deadline, conservatively including query/commit latency.
	q.executionDeadline = anchor.Add(deadline.Sub(dbNow))
	if _, e = tx.Exec(ctx, `INSERT INTO browser_external_auth_exchanges(request_id,effect_id) VALUES($1,$2)`, id, effect); e != nil || tx.Commit(ctx) != nil {
		return externalRequest{}, externalPrivate{}, ErrVault
	}
	return q, p, nil
}

// CompleteExternalAuth consumes state before any provider exchange and never
// retries it. Identity verification precedes an independently requested exact
// RP authenticated/anonymous differential. No token/identity enters a result.
func (v *Vault) CompleteExternalAuth(ctx context.Context, state, code, cookie string, denied bool) error {
	limit := 65 * time.Second
	if v.external == nil || v.external.budgets == nil {
		return ErrVault
	}
	if tool := time.Duration(v.external.budgets.Policy.ToolMillis) * time.Millisecond; tool < limit {
		limit = tool
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	q, p, e := v.claimExternalExchange(ctx, state, cookie)
	if e != nil {
		return ErrVault
	}
	bounded, stop := context.WithDeadline(ctx, q.executionDeadline)
	defer stop()
	ctx = bounded
	unknown := func() error { _ = v.finishExternal(ctx, q, "unknown", false, false, ""); return ErrVault }
	if denied {
		return v.finishExternal(ctx, q, "unverified", false, false, "")
	}
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return unknown()
	}
	defer tx.Rollback(ctx)
	if v.externalCurrentTx(ctx, tx, q, true) != nil {
		_ = tx.Rollback(ctx)
		return unknown()
	}
	// Authority is serialized at dispatch; network waiting must not hold Task,
	// owner-policy or Input row locks. Revocation can commit while Google is
	// in flight, and the result is revalidated before establishing RP access.
	if tx.Commit(ctx) != nil {
		return unknown()
	}
	identity, e := v.external.google.Exchange(ctx, code, p.challenge(), q.created)
	if e != nil {
		return unknown()
	}
	tx, e = v.Pool.Begin(ctx)
	if e != nil {
		return unknown()
	}
	defer tx.Rollback(ctx)
	if v.externalCurrentTx(ctx, tx, q, true) != nil {
		_ = tx.Rollback(ctx)
		return unknown()
	}
	if v.matchExternalIdentityTx(ctx, tx, q, identity) != nil {
		_ = tx.Rollback(ctx)
		_ = v.finishExternal(ctx, q, "unverified", false, false, "")
		return ErrVault
	}
	reference := randomID("google-test-")
	if _, e = tx.Exec(ctx, `INSERT INTO browser_external_auth_verified_identities(request_id,private_reference,expires_at) VALUES($1,$2,LEAST($3::timestamptz,$4::timestamptz))`, q.id, reference, q.expires, identity.ExpiresAt); e != nil || tx.Commit(ctx) != nil {
		_ = tx.Rollback(ctx)
		return unknown()
	}
	reference = ""
	auth, anonymous, ref, e := v.probeExternalRP(ctx, q, p.Cookie)
	if e != nil || !auth || !anonymous {
		return unknown()
	}
	if v.finishExternal(ctx, q, "verified", auth, anonymous, ref) != nil {
		return unknown()
	}
	return nil
}

func (v *Vault) finishExternal(ctx context.Context, q externalRequest, outcome string, auth, anonymous bool, reference string) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return ErrVault
	}
	defer tx.Rollback(ctx)
	if outcome == "verified" && v.externalCurrentTx(ctx, tx, q, true) != nil {
		return ErrVault
	}
	tag, e := tx.Exec(ctx, `INSERT INTO browser_external_auth_results(request_id,outcome,rp_authenticated_read,rp_anonymous_challenge,rp_reference) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`, q.id, outcome, auth, anonymous, reference)
	if e != nil || tag.RowsAffected() != 1 {
		return ErrVault
	}
	if actionauth.FinishExternalAuthTx(ctx, tx, q.id) != nil {
		return ErrVault
	}
	if v.audit(ctx, tx, q.p.OwnerID, q.record, q.id, "external_auth_"+outcome) != nil || v.AfterResponse != nil && v.AfterResponse(ctx, tx, q.p.RunID, q.id) != nil || tx.Commit(ctx) != nil {
		return ErrVault
	}
	return nil
}

// ExternalRPAccount is the diagnostic RP's protected content, not identity or
// credential metadata. Only an exact cookie for a verified current flow can
// read it. Cookie possession alone cannot establish provider authentication.
type ExternalRPAccount struct {
	Authenticated bool   `json:"authenticated"`
	Reference     string `json:"reference"`
}

func (v *Vault) ExternalAccount(ctx context.Context, cookieName, cookie string) (ExternalRPAccount, error) {
	if !externalOpaque(cookie) || v.external == nil {
		return ExternalRPAccount{}, ErrVault
	}
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return ExternalRPAccount{}, ErrVault
	}
	defer tx.Rollback(ctx)
	var id string
	if tx.QueryRow(ctx, `SELECT request_id FROM browser_external_auth_uses WHERE cookie_hash=$1`, digestString(cookie)).Scan(&id) != nil {
		return ExternalRPAccount{}, ErrVault
	}
	q, e := v.loadExternalTx(ctx, tx, id)
	if e != nil || cookieName != externalCookieName(q) || q.state != "approved" || v.externalCurrentTx(ctx, tx, q, true) != nil {
		return ExternalRPAccount{}, ErrVault
	}
	var reference string
	if tx.QueryRow(ctx, `SELECT private_reference FROM browser_external_auth_verified_identities WHERE request_id=$1 AND expires_at>clock_timestamp()`, id).Scan(&reference) != nil || tx.Commit(ctx) != nil {
		return ExternalRPAccount{}, ErrVault
	}
	return ExternalRPAccount{Authenticated: true, Reference: reference}, nil
}

func (v *Vault) probeExternalRP(ctx context.Context, q externalRequest, cookie string) (bool, bool, string, error) {
	if v.external == nil || v.external.probe == nil || !v.external.profile.Matches(q.scope) || v.external.profile.ClientFingerprint != q.profile {
		return false, false, "", ErrVault
	}
	get := func(auth bool) (int, []byte, error) {
		r, e := http.NewRequestWithContext(ctx, http.MethodGet, q.scope.VerifyURL, nil)
		if e != nil {
			return 0, nil, ErrVault
		}
		r.Header.Set("Accept", "application/json")
		if auth {
			r.AddCookie(&http.Cookie{Name: externalCookieName(q), Value: cookie})
		}
		response, e := v.external.probe.Do(r)
		if e != nil {
			return 0, nil, ErrVault
		}
		defer response.Body.Close()
		body, e := io.ReadAll(io.LimitReader(response.Body, 4097))
		if e != nil || len(body) > 4096 {
			return 0, nil, ErrVault
		}
		return response.StatusCode, body, nil
	}
	status, body, e := get(true)
	var account ExternalRPAccount
	if e != nil || status != http.StatusOK || jsonstrict.Decode(body, &account) != nil || !account.Authenticated || account.Reference == "" || len(account.Reference) > 256 {
		return false, false, "", ErrVault
	}
	status, body, e = get(false)
	var negative struct {
		Error string `json:"error"`
	}
	if e != nil || status != http.StatusUnauthorized || jsonstrict.Decode(body, &negative) != nil || negative.Error != "authentication_required" {
		return true, false, "", ErrVault
	}
	return true, true, account.Reference, nil
}
