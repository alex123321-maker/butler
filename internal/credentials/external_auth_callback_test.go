package credentials

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/externalauth"
	"github.com/jackc/pgx/v5"
)

type externalProviderFixture struct {
	original                         externalProvider
	calls                            atomic.Int32
	wrongAccount, wrongSubject, fail bool
	untrustedEmail, subjectCase      bool
	before                           func()
}

func (f *externalProviderFixture) Launch(c externalauth.Challenge) (externalauth.PrivateLaunch, error) {
	return f.original.Launch(c)
}
func (f *externalProviderFixture) Exchange(_ context.Context, code string, c externalauth.Challenge, _ time.Time) (externalauth.VerifiedIdentity, error) {
	f.calls.Add(1)
	if f.before != nil {
		f.before()
	}
	if code != "synthetic-code-canary" || !externalOpaque(c.State) || f.fail {
		return externalauth.VerifiedIdentity{}, errors.New("private-provider-diagnostic-canary")
	}
	email, subject := "synthetic@example.invalid", "synthetic-provider-subject"
	if f.wrongAccount {
		email = "other@example.invalid"
	}
	if f.wrongSubject {
		subject = "replaced-provider-subject"
	}
	if f.subjectCase {
		subject = strings.ToUpper(subject)
	}
	return externalauth.VerifiedIdentity{Email: email, Subject: subject, AuthoritativeEmail: !f.untrustedEmail, ExpiresAt: time.Now().Add(time.Hour)}, nil
}

type externalProbeTransport func(*http.Request) (*http.Response, error)

func (f externalProbeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestVaultExternalAuthCallbackOneUseAndRPAccess(t *testing.T) {
	for _, mode := range []string{"verified", "wrong_browser_cookie", "wrong_account", "wrong_subject", "subject_case", "untrusted_email", "lost_token_ack", "provider_denial", "anonymous_leak", "lost_rp_probe", "concurrent_callbacks", "cancelled_inflight", "revoked_inflight"} {
		t.Run(mode, func(t *testing.T) {
			v, p, entry, _, _, args := externalFixture(t)
			q := createExternalFixtureRequest(t, v, p, args)
			provider := &externalProviderFixture{original: v.external.google, wrongAccount: mode == "wrong_account", wrongSubject: mode == "wrong_subject", fail: mode == "lost_token_ack"}
			v.external.google = provider
			provider.untrustedEmail = mode == "untrusted_email"
			provider.subjectCase = mode == "subject_case"
			if mode == "wrong_subject" || mode == "subject_case" {
				index := v.externalSubjectIndex(p.OwnerID, v.external.profile.ClientFingerprint, entry.ID, "synthetic-provider-subject")
				if _, e := v.Pool.Exec(t.Context(), `INSERT INTO browser_external_auth_identities(owner_id,record_id,profile_fingerprint,subject_index) VALUES($1,$2,$3,$4)`, p.OwnerID, entry.ID, v.external.profile.ClientFingerprint, index); e != nil {
					t.Fatal("prior identity fixture", e)
				}
			}
			var wakes, probes atomic.Int32
			v.AfterResponse = func(ctx context.Context, tx pgx.Tx, run, id string) error {
				if run != p.RunID || id != q.ID {
					return ErrVault
				}
				wakes.Add(1)
				_, e := tx.Exec(ctx, `UPDATE accepted_inputs SET checkpoint='interaction_ready' WHERE input_id='input' AND checkpoint_json->'interaction'->>'interaction_id'=$1`, id)
				return e
			}
			if mode == "cancelled_inflight" {
				provider.before = func() {
					if v.CancelExternal(t.Context(), p.OwnerID, q.ID, q.ScopeDigest) != nil {
						t.Error("in-flight cancellation failed")
					}
				}
			}
			if mode == "revoked_inflight" {
				provider.before = func() {
					if _, e := v.Pool.Exec(t.Context(), `UPDATE action_mandates SET status='revoked'`); e != nil {
						t.Error("in-flight revocation failed", e)
					}
				}
			}
			v.external.probe = &http.Client{Transport: externalProbeTransport(func(r *http.Request) (*http.Response, error) {
				probes.Add(1)
				if r.Method != "GET" || r.URL.String() != q.ExternalScope.VerifyURL {
					t.Error("probe destination changed")
					return nil, ErrVault
				}
				if mode == "lost_rp_probe" {
					return nil, errors.New("private-probe-diagnostic-canary")
				}
				status, body := 401, `{"error":"authentication_required"}`
				if cookies := r.Cookies(); len(cookies) == 1 {
					account, e := v.ExternalAccount(r.Context(), cookies[0].Name, cookies[0].Value)
					if e != nil {
						t.Log("protected RP probe rejected by current-flow guard")
					}
					if e == nil {
						status = 200
						raw, _ := json.Marshal(account)
						body = string(raw)
					}
				} else if mode == "anonymous_leak" {
					body = `{"error":"authentication_required","reference":"leaked-private-fact"}`
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
			})}
			launch, e := v.RespondExternal(t.Context(), p.OwnerID, q.ID, entry.ID, true, externalauth.Consent, q.ScopeDigest)
			if e != nil {
				t.Fatal("protected decision", e)
			}
			u, _ := url.Parse(launch.URL)
			start, e := v.StartExternal(t.Context(), u.Query().Get("ticket"))
			if e != nil {
				t.Fatal("private launch", e)
			}
			auth, _ := url.Parse(start.URL)
			state := auth.Query().Get("state")
			if _, e := v.ExternalAccount(t.Context(), start.CookieName, start.CookieValue); e == nil {
				t.Fatal("launch cookie authenticated before identity verification")
			}
			if mode == "wrong_browser_cookie" {
				other, _ := externalauth.NewChallenge()
				if v.CompleteExternalAuth(t.Context(), state, "synthetic-code-canary", other.State, false) == nil {
					t.Fatal("foreign browser cookie admitted")
				}
				var n int
				if v.Pool.QueryRow(t.Context(), `SELECT count(*) FROM browser_external_auth_exchanges`).Scan(&n) != nil || n != 0 || provider.calls.Load() != 0 {
					t.Fatal("foreign browser spent exchange authority")
				}
			}
			var success atomic.Int32
			var wg sync.WaitGroup
			attempts := 1
			if mode == "concurrent_callbacks" {
				attempts = 6
			}
			for range attempts {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if v.CompleteExternalAuth(t.Context(), state, "synthetic-code-canary", start.CookieValue, mode == "provider_denial") == nil {
						success.Add(1)
					}
				}()
			}
			wg.Wait()
			want := "verified"
			if mode == "wrong_account" || mode == "wrong_subject" || mode == "subject_case" || mode == "untrusted_email" || mode == "provider_denial" {
				want = "unverified"
			}
			if mode == "lost_token_ack" || mode == "lost_rp_probe" || mode == "anonymous_leak" || mode == "cancelled_inflight" || mode == "revoked_inflight" {
				want = "unknown"
			}
			var outcome, reference string
			var authenticated, anonymous bool
			if v.Pool.QueryRow(t.Context(), `SELECT outcome,rp_authenticated_read,rp_anonymous_challenge,rp_reference FROM browser_external_auth_results WHERE request_id=$1`, q.ID).Scan(&outcome, &authenticated, &anonymous, &reference) != nil || outcome != want {
				var identities int
				_ = v.Pool.QueryRow(t.Context(), `SELECT count(*) FROM browser_external_auth_verified_identities`).Scan(&identities)
				t.Fatalf("wrong callback outcome=%s expected=%s provider_calls=%d probes=%d verified_identity_receipts=%d wakes=%d", outcome, want, provider.calls.Load(), probes.Load(), identities, wakes.Load())
			}
			if (outcome == "verified") != (authenticated && anonymous && reference != "") {
				t.Fatal("identity alone produced RP success")
			}
			wantCalls := int32(1)
			if mode == "provider_denial" {
				wantCalls = 0
			}
			if provider.calls.Load() != wantCalls || wakes.Load() != 1 {
				t.Fatal("callback replayed provider call or canonical wake")
			}
			if mode == "concurrent_callbacks" && success.Load() != 1 {
				t.Fatal("more than one callback completed")
			}
			if want == "verified" && probes.Load() != 2 {
				t.Fatal("missing actual authenticated/anonymous probe")
			}
			if want == "unverified" && probes.Load() != 0 {
				t.Fatal("unverified identity reached RP content")
			}
			if v.CompleteExternalAuth(t.Context(), state, "synthetic-code-canary", start.CookieValue, false) == nil || provider.calls.Load() != wantCalls || wakes.Load() != 1 {
				t.Fatal("consumed callback was replayed")
			}
			if mode == "lost_token_ack" {
				restarted, e := NewVault(v.Pool, v.key)
				if e != nil {
					t.Fatal("restart fixture")
				}
				cfg := externalauth.GoogleConfig{ClientID: "123-test.apps.googleusercontent.com", ClientSecret: "synthetic-client-secret-canary", RedirectURL: "https://rp.example/auth/google/callback"}
				if restarted.ConfigureExternalAuth(t.Context(), cfg, q.ExternalScope.RPURL, q.ExternalScope.VerifyURL, v.external.budgets) != nil || restarted.CompleteExternalAuth(t.Context(), state, "synthetic-code-canary", start.CookieValue, false) == nil {
					t.Fatal("restart restored spent callback authority")
				}
			}
			var reservations int
			var effectState string
			if v.Pool.QueryRow(t.Context(), `SELECT count(*) FROM execution_budget_reservations`).Scan(&reservations) != nil || reservations != 1 {
				t.Fatal("private exchange spent budget more than once or bypassed it")
			}
			if v.Pool.QueryRow(t.Context(), `SELECT e.state FROM browser_external_auth_exchanges x JOIN action_effects e ON e.action_id=x.effect_id WHERE x.request_id=$1`, q.ID).Scan(&effectState) != nil {
				t.Fatal("private authentication missing from the effect journal")
			}
			wantEffect := "effect_confirmed"
			if want == "unknown" {
				wantEffect = "effect_unknown"
			}
			if want == "unverified" {
				wantEffect = "effect_absent"
			}
			if effectState != wantEffect {
				t.Fatal("private authentication journal verdict mismatch")
			}
			// Ordinary request metadata must never contain private transport data.
			raw, _ := json.Marshal(q)
			for _, private := range []string{state, start.CookieValue, u.Query().Get("ticket"), "synthetic-client-secret-canary", "synthetic-private-password-canary", "synthetic@example.invalid"} {
				if strings.Contains(string(raw), private) {
					t.Fatal("private auth data leaked into request metadata")
				}
			}
		})
	}
}
