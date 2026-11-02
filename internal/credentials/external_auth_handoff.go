package credentials

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"time"

	"github.com/butler/butler/internal/externalauth"
	"github.com/butler/butler/internal/jsonstrict"
)

// Only serialized directly into AEAD, never returned as JSON/metadata.
type externalPrivate struct{ State, Nonce, Verifier, Ticket, Cookie string }

func (externalPrivate) String() string   { return "ExternalAuthPrivate([redacted])" }
func (externalPrivate) GoString() string { return "ExternalAuthPrivate([redacted])" }
func (p externalPrivate) challenge() externalauth.Challenge {
	return externalauth.Challenge{State: p.State, Nonce: p.Nonce, Verifier: p.Verifier}
}
func externalOpaque(s string) bool {
	b, e := base64.RawURLEncoding.DecodeString(s)
	return e == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == s
}

func (v *Vault) openExternal(q externalRequest, cipher string) (externalPrivate, error) {
	var p externalPrivate
	raw, e := v.open(q.p.OwnerID, q.id, "external_auth_v1", cipher)
	if e != nil || jsonstrict.Decode([]byte(raw), &p) != nil || !externalOpaque(p.State) || !externalOpaque(p.Nonce) || !externalOpaque(p.Verifier) || !externalOpaque(p.Ticket) || !externalOpaque(p.Cookie) {
		return externalPrivate{}, ErrVault
	}
	return p, nil
}
func (v *Vault) externalLaunch(q externalRequest, p externalPrivate) (externalauth.PrivateLaunch, error) {
	if v.external == nil || v.external.profile.ClientFingerprint != q.profile || !v.external.profile.Matches(q.scope) {
		return externalauth.PrivateLaunch{}, ErrVault
	}
	u, e := url.Parse(v.external.profile.RedirectURL)
	if e != nil {
		return externalauth.PrivateLaunch{}, ErrVault
	}
	u.Path = "/auth/google/start"
	u.RawQuery = url.Values{"ticket": {p.Ticket}}.Encode()
	return externalauth.PrivateLaunch{URL: u.String()}, nil
}

// RespondExternal performs one protected owner decision. Approval permits a
// private launch, not an Input wake or an authenticated-RP/model success claim.
func (v *Vault) RespondExternal(ctx context.Context, owner, id, record string, allow bool, consent, digest string) (externalauth.PrivateLaunch, error) {
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return externalauth.PrivateLaunch{}, ErrVault
	}
	defer tx.Rollback(ctx)
	q, e := v.loadExternalTx(ctx, tx, id)
	if e != nil || q.p.OwnerID != owner || allow && (record != q.record || consent != externalauth.Consent || subtle.ConstantTimeCompare([]byte(digest), []byte(q.digest)) != 1) || v.externalCurrentTx(ctx, tx, q, allow) != nil {
		return externalauth.PrivateLaunch{}, ErrVault
	}
	var state string
	if tx.QueryRow(ctx, `SELECT state FROM browser_external_auth_requests WHERE id=$1`, q.id).Scan(&state) != nil {
		return externalauth.PrivateLaunch{}, ErrVault
	}
	if state != "pending" {
		if !allow && state == "denied" {
			if tx.Commit(ctx) == nil {
				return externalauth.PrivateLaunch{}, nil
			}
			return externalauth.PrivateLaunch{}, ErrVault
		}
		if !allow || state != "approved" {
			return externalauth.PrivateLaunch{}, ErrVault
		}
		var cipher string
		if tx.QueryRow(ctx, `SELECT private_cipher FROM browser_external_auth_uses WHERE request_id=$1`, id).Scan(&cipher) != nil {
			return externalauth.PrivateLaunch{}, ErrVault
		}
		p, e := v.openExternal(q, cipher)
		if e != nil {
			return externalauth.PrivateLaunch{}, ErrVault
		}
		launch, e := v.externalLaunch(q, p)
		if e != nil || tx.Commit(ctx) != nil {
			return externalauth.PrivateLaunch{}, ErrVault
		}
		return launch, nil
	}
	target := "denied"
	var launch externalauth.PrivateLaunch
	if allow {
		if v.external == nil || v.external.profile.ClientFingerprint != q.profile {
			return launch, ErrVault
		}
		c, e := externalauth.NewChallenge()
		if e != nil {
			return launch, ErrVault
		}
		other, e := externalauth.NewChallenge()
		if e != nil {
			return launch, ErrVault
		}
		p := externalPrivate{State: c.State, Nonce: c.Nonce, Verifier: c.Verifier, Ticket: other.State, Cookie: other.Nonce}
		raw, _ := json.Marshal(p)
		cipher, e := v.seal(owner, id, "external_auth_v1", string(raw))
		if e != nil {
			return launch, ErrVault
		}
		launch, e = v.externalLaunch(q, p)
		if e != nil {
			return launch, ErrVault
		}
		if _, e = tx.Exec(ctx, `INSERT INTO browser_external_auth_uses(request_id,state_hash,ticket_hash,cookie_hash,private_cipher) SELECT id,$2,$3,$4,$5 FROM browser_external_auth_requests WHERE id=$1 AND expires_at>clock_timestamp()`, id, digestString(p.State), digestString(p.Ticket), digestString(p.Cookie), cipher); e != nil {
			return externalauth.PrivateLaunch{}, ErrVault
		}
		target = "approved"
	}
	tag, e := tx.Exec(ctx, `UPDATE browser_external_auth_requests SET state=$2 WHERE id=$1 AND state='pending' AND expires_at>clock_timestamp()`, id, target)
	if e != nil || tag.RowsAffected() != 1 || v.audit(ctx, tx, owner, q.record, id, "external_auth_"+target) != nil {
		return externalauth.PrivateLaunch{}, ErrVault
	}
	if !allow && v.AfterResponse != nil && v.AfterResponse(ctx, tx, q.p.RunID, id) != nil {
		return externalauth.PrivateLaunch{}, ErrVault
	}
	if tx.Commit(ctx) != nil {
		return externalauth.PrivateLaunch{}, ErrVault
	}
	return launch, nil
}

type ExternalStart struct {
	URL         string    `json:"-"`
	CookieName  string    `json:"-"`
	CookieValue string    `json:"-"`
	ExpiresAt   time.Time `json:"-"`
}

func (ExternalStart) String() string              { return "ExternalAuthStart([redacted])" }
func (ExternalStart) GoString() string            { return "ExternalAuthStart([redacted])" }
func externalCookieName(q externalRequest) string { return "__Host-butler-external-" + q.id[5:21] }

// StartExternal consumes a launch ticket before emitting Google's private URL.
// Retrying a consumed ticket cannot mint another auth attempt or browser cookie.
func (v *Vault) StartExternal(ctx context.Context, ticket string) (ExternalStart, error) {
	if !externalOpaque(ticket) || v.external == nil {
		return ExternalStart{}, ErrVault
	}
	tx, e := v.Pool.Begin(ctx)
	if e != nil {
		return ExternalStart{}, ErrVault
	}
	defer tx.Rollback(ctx)
	var id string
	if tx.QueryRow(ctx, `SELECT request_id FROM browser_external_auth_uses WHERE ticket_hash=$1`, digestString(ticket)).Scan(&id) != nil {
		return ExternalStart{}, ErrVault
	}
	q, e := v.loadExternalTx(ctx, tx, id)
	if e != nil || q.state != "approved" || v.externalCurrentTx(ctx, tx, q, true) != nil || v.external.profile.ClientFingerprint != q.profile {
		return ExternalStart{}, ErrVault
	}
	var cipher string
	if tx.QueryRow(ctx, `SELECT private_cipher FROM browser_external_auth_uses WHERE request_id=$1`, id).Scan(&cipher) != nil {
		return ExternalStart{}, ErrVault
	}
	p, e := v.openExternal(q, cipher)
	if e != nil || subtle.ConstantTimeCompare([]byte(p.Ticket), []byte(ticket)) != 1 {
		return ExternalStart{}, ErrVault
	}
	launch, e := v.external.google.Launch(p.challenge())
	if e != nil {
		return ExternalStart{}, ErrVault
	}
	if _, e = tx.Exec(ctx, `INSERT INTO browser_external_auth_launches(request_id) VALUES($1)`, id); e != nil || tx.Commit(ctx) != nil {
		return ExternalStart{}, ErrVault
	}
	return ExternalStart{URL: launch.URL, CookieName: externalCookieName(q), CookieValue: p.Cookie, ExpiresAt: q.expires}, nil
}
