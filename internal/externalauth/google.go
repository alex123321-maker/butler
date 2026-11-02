// Package externalauth implements private provider protocol operations. It does
// not grant browser, credential, task, callback, or continuation authority.
package externalauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/butler/butler/internal/browsersso"
	"github.com/butler/butler/internal/jsonstrict"
	"github.com/butler/butler/internal/security/egress"
)

const googleAuthorize = "https://accounts.google.com/o/oauth2/v2/auth"
const googleToken = "https://oauth2.googleapis.com/token"
const googleKeys = "https://www.googleapis.com/oauth2/v3/certs"

var ErrProvider = errors.New("external authentication unavailable or unverified")

// GoogleConfig is trusted operator configuration, never model/tool input.
type GoogleConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURL  string `json:"redirect_uri"`
}

func (GoogleConfig) String() string               { return "GoogleConfig([redacted])" }
func (GoogleConfig) GoString() string             { return "GoogleConfig([redacted])" }
func (GoogleConfig) MarshalJSON() ([]byte, error) { return []byte(`{"redacted":true}`), nil }

// Challenge belongs to one durable, owner-approved attempt. Persist it only
// through the owning encrypted store; no field is a public observation.
type Challenge struct {
	State    string `json:"-"`
	Nonce    string `json:"-"`
	Verifier string `json:"-"`
}

func (Challenge) String() string   { return "ExternalAuthChallenge([redacted])" }
func (Challenge) GoString() string { return "ExternalAuthChallenge([redacted])" }
func opaque() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", ErrProvider
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
func NewChallenge() (Challenge, error) {
	var c Challenge
	for _, dest := range []*string{&c.State, &c.Nonce, &c.Verifier} {
		s, err := opaque()
		if err != nil {
			return Challenge{}, err
		}
		*dest = s
	}
	return c, nil
}
func validOpaque(s string) bool {
	b, e := base64.RawURLEncoding.DecodeString(s)
	return e == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == s
}

type PrivateLaunch struct {
	URL string `json:"-"`
}

func (PrivateLaunch) String() string   { return "ExternalAuthLaunch([redacted])" }
func (PrivateLaunch) GoString() string { return "ExternalAuthLaunch([redacted])" }

// VerifiedIdentity is private protocol data, not an ActionEffect or permission
// to resume a Run. The owning kernel must match account and current authority.
type VerifiedIdentity struct {
	Subject   string    `json:"-"`
	Email     string    `json:"-"`
	ExpiresAt time.Time `json:"-"`
}

func (VerifiedIdentity) String() string   { return "VerifiedExternalIdentity([redacted])" }
func (VerifiedIdentity) GoString() string { return "VerifiedExternalIdentity([redacted])" }

type Google struct {
	cfg  GoogleConfig
	http *http.Client
	now  func() time.Time
}

func (*Google) String() string   { return "GoogleExternalAuth([redacted])" }
func (*Google) GoString() string { return "GoogleExternalAuth([redacted])" }

func NewGoogle(cfg GoogleConfig) (*Google, error) {
	u, e := url.Parse(cfg.RedirectURL)
	idPrefix := strings.TrimSuffix(cfg.ClientID, ".apps.googleusercontent.com")
	if e != nil || browsersso.Origin(cfg.RedirectURL) == "" || u.Path != "/auth/google/callback" || len(cfg.ClientID) > 512 || idPrefix == cfg.ClientID || idPrefix == "" || len(cfg.ClientSecret) < 8 || len(cfg.ClientSecret) > 4096 {
		return nil, ErrProvider
	}
	for _, r := range idPrefix {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return nil, ErrProvider
		}
	}
	for _, v := range []string{cfg.ClientID, cfg.ClientSecret} {
		for _, r := range v {
			if r <= 32 || r >= 127 {
				return nil, ErrProvider
			}
		}
	}
	return &Google{cfg: cfg, http: egress.NewClient(egress.Policy{}, 15*time.Second), now: time.Now}, nil
}

func (g *Google) Launch(c Challenge) (PrivateLaunch, error) {
	if g == nil || !validOpaque(c.State) || !validOpaque(c.Nonce) || !validOpaque(c.Verifier) {
		return PrivateLaunch{}, ErrProvider
	}
	h := sha256.Sum256([]byte(c.Verifier))
	q := url.Values{"client_id": {g.cfg.ClientID}, "redirect_uri": {g.cfg.RedirectURL}, "response_type": {"code"}, "scope": {"openid email"}, "state": {c.State}, "nonce": {c.Nonce}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(h[:])}, "prompt": {"select_account"}}
	return PrivateLaunch{URL: googleAuthorize + "?" + q.Encode()}, nil
}

// Exchange never retries or follows redirects. The owning store MUST consume
// the exact state/browser binding before invoking this one-shot operation.
// A lost HTTP ACK is unknown, not permission to call Exchange again.
func (g *Google) Exchange(ctx context.Context, code string, c Challenge, created time.Time) (VerifiedIdentity, error) {
	if g == nil || len(code) == 0 || len(code) > 4096 || !validOpaque(c.Nonce) || !validOpaque(c.Verifier) || !validOpaque(c.State) || created.IsZero() || created.After(g.now()) || g.now().Sub(created) > 10*time.Minute {
		return VerifiedIdentity{}, ErrProvider
	}
	raw, e := g.request(ctx, http.MethodPost, googleToken, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {g.cfg.RedirectURL}, "client_id": {g.cfg.ClientID}, "client_secret": {g.cfg.ClientSecret}, "code_verifier": {c.Verifier}})
	if e != nil {
		return VerifiedIdentity{}, ErrProvider
	}
	var body map[string]json.RawMessage
	if jsonstrict.Decode(raw, &body) != nil || body == nil {
		return VerifiedIdentity{}, ErrProvider
	}
	var token string
	if json.Unmarshal(body["id_token"], &token) != nil || len(token) > 16384 {
		return VerifiedIdentity{}, ErrProvider
	}
	return g.verify(ctx, token, c.Nonce, created)
}
func (g *Google) request(ctx context.Context, method, target string, form url.Values) ([]byte, error) {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r, e := http.NewRequestWithContext(ctx, method, target, body)
	if e != nil {
		return nil, ErrProvider
	}
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	r.Header.Set("Accept", "application/json")
	response, e := g.http.Do(r)
	if e != nil {
		return nil, ErrProvider
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrProvider
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, 65537))
	if e != nil || len(raw) > 65536 {
		return nil, ErrProvider
	}
	return raw, nil
}

func (g *Google) verify(ctx context.Context, token, nonce string, created time.Time) (VerifiedIdentity, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || len(token) > 16384 {
		return VerifiedIdentity{}, ErrProvider
	}
	decode := func(s string, out any) error {
		b, e := base64.RawURLEncoding.DecodeString(s)
		if e != nil || base64.RawURLEncoding.EncodeToString(b) != s {
			return ErrProvider
		}
		return jsonstrict.Decode(b, out)
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		Typ string `json:"typ"`
	}
	if decode(parts[0], &header) != nil || header.Alg != "RS256" || header.Kid == "" || len(header.Kid) > 256 || header.Typ != "" && header.Typ != "JWT" {
		return VerifiedIdentity{}, ErrProvider
	}
	var claims map[string]json.RawMessage
	if decode(parts[1], &claims) != nil || claims == nil {
		return VerifiedIdentity{}, ErrProvider
	}
	var iss, aud, azp, sub, gotNonce, email string
	var exp, iat int64
	var emailVerified bool
	for name, dest := range map[string]*string{"iss": &iss, "aud": &aud, "sub": &sub, "nonce": &gotNonce, "email": &email} {
		if json.Unmarshal(claims[name], dest) != nil {
			return VerifiedIdentity{}, ErrProvider
		}
	}
	if b, ok := claims["azp"]; ok && (json.Unmarshal(b, &azp) != nil || azp != g.cfg.ClientID) {
		return VerifiedIdentity{}, ErrProvider
	}
	if json.Unmarshal(claims["exp"], &exp) != nil || json.Unmarshal(claims["iat"], &iat) != nil || json.Unmarshal(claims["email_verified"], &emailVerified) != nil {
		return VerifiedIdentity{}, ErrProvider
	}
	now := g.now()
	if (iss != "https://accounts.google.com" && iss != "accounts.google.com") || aud != g.cfg.ClientID || sub == "" || len(sub) > 255 || !emailVerified || email == "" || len(email) > 320 || strings.TrimSpace(email) != email || !strings.Contains(email, "@") || subtle.ConstantTimeCompare([]byte(gotNonce), []byte(nonce)) != 1 || exp <= now.Unix() || iat < created.Add(-time.Minute).Unix() || iat > now.Add(time.Minute).Unix() || exp <= iat || exp-iat > 7200 {
		return VerifiedIdentity{}, ErrProvider
	}
	if b, ok := claims["nbf"]; ok {
		var nbf int64
		if json.Unmarshal(b, &nbf) != nil || nbf > now.Unix() {
			return VerifiedIdentity{}, ErrProvider
		}
	}
	raw, e := g.request(ctx, http.MethodGet, googleKeys, nil)
	if e != nil {
		return VerifiedIdentity{}, ErrProvider
	}
	var jwks struct {
		Keys []map[string]json.RawMessage `json:"keys"`
	}
	if jsonstrict.Decode(raw, &jwks) != nil || len(jwks.Keys) == 0 || len(jwks.Keys) > 16 {
		return VerifiedIdentity{}, ErrProvider
	}
	var key *rsa.PublicKey
	for _, j := range jwks.Keys {
		var kid, kty, alg, use, n, exponent string
		for name, dest := range map[string]*string{"kid": &kid, "kty": &kty, "alg": &alg, "use": &use, "n": &n, "e": &exponent} {
			if json.Unmarshal(j[name], dest) != nil {
				return VerifiedIdentity{}, ErrProvider
			}
		}
		if kid != header.Kid {
			continue
		}
		if key != nil || kty != "RSA" || alg != "RS256" || use != "sig" {
			return VerifiedIdentity{}, ErrProvider
		}
		nb, ne := base64.RawURLEncoding.DecodeString(n)
		eb, ee := base64.RawURLEncoding.DecodeString(exponent)
		if ne != nil || ee != nil || len(eb) > 4 || len(eb) == 0 || len(nb) == 0 || nb[0] == 0 || eb[0] == 0 || base64.RawURLEncoding.EncodeToString(nb) != n || base64.RawURLEncoding.EncodeToString(eb) != exponent {
			return VerifiedIdentity{}, ErrProvider
		}
		eValue := new(big.Int).SetBytes(eb).Int64()
		modulus := new(big.Int).SetBytes(nb)
		if modulus.BitLen() < 2048 || modulus.BitLen() > 4096 || eValue != 65537 {
			return VerifiedIdentity{}, ErrProvider
		}
		key = &rsa.PublicKey{N: modulus, E: int(eValue)}
	}
	signature, e := base64.RawURLEncoding.DecodeString(parts[2])
	h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if e != nil || key == nil || base64.RawURLEncoding.EncodeToString(signature) != parts[2] || rsa.VerifyPKCS1v15(key, crypto.SHA256, h[:], signature) != nil {
		return VerifiedIdentity{}, ErrProvider
	}
	return VerifiedIdentity{Subject: sub, Email: email, ExpiresAt: time.Unix(exp, 0).UTC()}, nil
}
