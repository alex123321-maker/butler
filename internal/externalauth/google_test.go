package externalauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func googleFixture(t *testing.T) (*Google, Challenge, *rsa.PrivateKey, map[string]any) {
	t.Helper()
	g, e := NewGoogle(GoogleConfig{ClientID: "123-test.apps.googleusercontent.com", ClientSecret: "synthetic-client-secret-canary", RedirectURL: "https://rp.example/auth/google/callback"})
	if e != nil {
		t.Fatal(e)
	}
	c, e := NewChallenge()
	if e != nil {
		t.Fatal(e)
	}
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal("test key unavailable")
	}
	now := time.Now().Truncate(time.Second)
	g.now = func() time.Time { return now }
	claims := map[string]any{"iss": "https://accounts.google.com", "aud": g.cfg.ClientID, "azp": g.cfg.ClientID, "sub": "private-subject-canary", "nonce": c.Nonce, "email": "synthetic@example.invalid", "email_verified": true, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}
	return g, c, key, claims
}
func signedToken(t *testing.T, key *rsa.PrivateKey, claims any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"test-key","typ":"JWT"}`))
	payload, e := json.Marshal(claims)
	if e != nil {
		t.Fatal("test claims")
	}
	body := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	h := sha256.Sum256([]byte(body))
	sig, e := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
	if e != nil {
		t.Fatal("test signing")
	}
	return body + "." + base64.RawURLEncoding.EncodeToString(sig)
}
func keySet(key *rsa.PrivateKey) string {
	b, _ := json.Marshal(map[string]any{"keys": []any{map[string]any{"kid": "test-key", "kty": "RSA", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
	return string(b)
}

func TestGoogleLaunchPinsMinimalIdentityAndPKCE(t *testing.T) {
	g, c, _, _ := googleFixture(t)
	launch, e := g.Launch(c)
	if e != nil {
		t.Fatal(e)
	}
	u, _ := url.Parse(launch.URL)
	q := u.Query()
	hash := sha256.Sum256([]byte(c.Verifier))
	if u.Scheme+"://"+u.Host+u.Path != googleAuthorize || q.Get("scope") != "openid email" || q.Get("client_id") != g.cfg.ClientID || q.Get("redirect_uri") != g.cfg.RedirectURL || q.Get("state") != c.State || q.Get("nonce") != c.Nonce || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(hash[:]) || q.Get("response_type") != "code" || q.Get("prompt") != "select_account" || len(q) != 9 {
		t.Fatal("launch scope or challenge changed")
	}
	for _, s := range []string{g.cfg.ClientSecret, c.Verifier, "access_type", "offline"} {
		if strings.Contains(launch.URL, s) {
			t.Fatal("launch exposed secrets or requested reusable access")
		}
	}
	for _, v := range []any{g, g.cfg, c, launch, VerifiedIdentity{Subject: "private-subject-canary", Email: "synthetic@example.invalid"}} {
		b, _ := json.Marshal(v)
		text := fmt.Sprintf("%v %#v %s", v, v, b)
		for _, s := range []string{g.cfg.ClientSecret, c.State, c.Nonce, c.Verifier, "private-subject-canary", "synthetic@example.invalid"} {
			if strings.Contains(text, s) {
				t.Fatal("private auth data formatted publicly")
			}
		}
	}
}

func TestGoogleExchangeAndStrictSignedIdentity(t *testing.T) {
	g, c, key, base := googleFixture(t)
	for _, mode := range []string{"valid", "wrong_audience", "wrong_issuer", "wrong_nonce", "unverified_email", "expired", "future_issue", "old_issue", "long_lived", "wrong_azp", "array_audience", "duplicate_claim", "bad_signature", "unknown_key", "duplicate_key", "bad_alg", "redirect", "missing_id_token", "large_body", "network_lost"} {
		t.Run(mode, func(t *testing.T) {
			claims := map[string]any{}
			for k, v := range base {
				claims[k] = v
			}
			switch mode {
			case "wrong_audience":
				claims["aud"] = "other-client"
			case "wrong_issuer":
				claims["iss"] = "https://foreign.example"
			case "wrong_nonce":
				claims["nonce"] = "other"
			case "unverified_email":
				claims["email_verified"] = false
			case "expired":
				claims["exp"] = g.now().Unix()
			case "future_issue":
				claims["iat"] = g.now().Add(2 * time.Minute).Unix()
			case "old_issue":
				claims["iat"] = g.now().Add(-time.Hour).Unix()
			case "long_lived":
				claims["exp"] = g.now().Add(3 * time.Hour).Unix()
			case "wrong_azp":
				claims["azp"] = "other-client"
			case "array_audience":
				claims["aud"] = []string{g.cfg.ClientID, "other"}
			}
			token := signedToken(t, key, claims)
			if mode == "bad_signature" {
				parts := strings.Split(token, ".")
				parts[2] = base64.RawURLEncoding.EncodeToString(make([]byte, key.Size()))
				token = strings.Join(parts, ".")
			}
			if mode == "duplicate_claim" {
				parts := strings.Split(token, ".")
				raw, _ := base64.RawURLEncoding.DecodeString(parts[1])
				parts[1] = base64.RawURLEncoding.EncodeToString(append([]byte(`{"aud":"other",`), raw[1:]...))
				h := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
				sig, _ := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
				parts[2] = base64.RawURLEncoding.EncodeToString(sig)
				token = strings.Join(parts, ".")
			}
			if mode == "bad_alg" {
				parts := strings.Split(token, ".")
				parts[0] = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","kid":"test-key"}`))
				token = strings.Join(parts, ".")
			}
			posts, gets := 0, 0
			g.http = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				status, body := 200, ""
				switch r.URL.String() {
				case googleToken:
					posts++
					if r.Method != "POST" {
						t.Fatal("exchange method")
					}
					raw, _ := io.ReadAll(r.Body)
					q, _ := url.ParseQuery(string(raw))
					if q.Get("client_secret") != g.cfg.ClientSecret || q.Get("code_verifier") != c.Verifier || q.Get("code") != "synthetic-code-canary" || q.Get("redirect_uri") != g.cfg.RedirectURL || q.Get("client_id") != g.cfg.ClientID || q.Get("grant_type") != "authorization_code" || len(q) != 6 {
						t.Fatal("private exchange binding changed")
					}
					if mode == "network_lost" {
						return nil, fmt.Errorf("synthetic transport failure")
					}
					b, _ := json.Marshal(map[string]string{"id_token": token, "access_token": "discarded-access-canary", "refresh_token": "discarded-refresh-canary"})
					body = string(b)
					if mode == "redirect" {
						status = 307
					}
					if mode == "missing_id_token" {
						body = `{"access_token":"discarded-access-canary"}`
					}
					if mode == "large_body" {
						body = strings.Repeat("a", 65537)
					}
				case googleKeys:
					gets++
					if r.Method != "GET" {
						t.Fatal("key request method")
					}
					body = keySet(key)
					if mode == "unknown_key" {
						body = strings.ReplaceAll(body, "test-key", "unknown")
					}
					if mode == "duplicate_key" {
						var set map[string][]any
						_ = json.Unmarshal([]byte(body), &set)
						set["keys"] = append(set["keys"], set["keys"][0])
						raw, _ := json.Marshal(set)
						body = string(raw)
					}
				default:
					t.Fatal("unexpected provider destination")
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://foreign.example/collect"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			}), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
			identity, e := g.Exchange(context.Background(), "synthetic-code-canary", c, g.now().Add(-time.Second))
			if (e == nil) != (mode == "valid") || posts != 1 || gets > 1 {
				t.Fatal("wrong provider verdict or replay")
			}
			if mode == "valid" && (identity.Subject != "private-subject-canary" || identity.Email != "synthetic@example.invalid") {
				t.Fatal("verified identity unavailable")
			}
			if e != nil && e.Error() != ErrProvider.Error() {
				t.Fatal("private provider diagnostic leaked")
			}
		})
	}
}

func TestGoogleRejectsUntrustedOperatorConfigAndExpiredChallenge(t *testing.T) {
	g, c, _, _ := googleFixture(t)
	for _, mutate := range []func(*GoogleConfig){func(c *GoogleConfig) { c.RedirectURL = "http://rp.example/auth/google/callback" }, func(c *GoogleConfig) { c.RedirectURL += "?code=private" }, func(c *GoogleConfig) { c.RedirectURL = "https://rp.example/other" }, func(c *GoogleConfig) { c.ClientID = ".apps.googleusercontent.com" }, func(c *GoogleConfig) { c.ClientSecret = "bad\nsecret" }} {
		cfg := g.cfg
		mutate(&cfg)
		if _, e := NewGoogle(cfg); e == nil {
			t.Fatal("untrusted provider configuration accepted")
		}
	}
	g.http = &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
		t.Fatal("stale challenge reached network")
		return nil, ErrProvider
	})}
	for _, created := range []time.Time{{}, g.now().Add(time.Second), g.now().Add(-11 * time.Minute)} {
		if _, e := g.Exchange(t.Context(), "code", c, created); e == nil {
			t.Fatal("stale challenge accepted")
		}
	}
}
