// browser-sso-eval is an operator-only recipient. It is never a Butler tool or
// an authentication provider for real accounts.
package main

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const flowTTL = 3 * time.Minute

// The RP is observed before the protected owner decision (up to ten minutes).
// Keep only that browser binding alive across consent; OAuth flows and codes
// are still created after approval and expire after three minutes.
const rpBindingTTL = 15 * time.Minute

type config struct {
	PublicVariant string `json:"public_variant,omitempty"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	PrivateFact   string `json:"private_fact"`
}

func (f *fixture) publicPath(base string) string {
	if f.cfg.PublicVariant != "" {
		return base + "-" + f.cfg.PublicVariant
	}
	return base
}
func (f *fixture) userField() string {
	if f.cfg.PublicVariant != "" {
		return "account-" + f.cfg.PublicVariant
	}
	return "username"
}
func (f *fixture) passwordField() string {
	if f.cfg.PublicVariant != "" {
		return "secret-" + f.cfg.PublicVariant
	}
	return "password"
}

type pending struct {
	browser, verifier string
	expires           time.Time
}
type authorization struct {
	browser, state, challenge string
	expires                   time.Time
}
type code struct {
	state, challenge string
	expires          time.Time
}
type account struct {
	browser string
	expires time.Time
}
type receipt struct {
	Sequence int       `json:"sequence"`
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	Accepted bool      `json:"accepted"`
}

type fixture struct {
	mu                  sync.Mutex
	cfg                 config
	rp, idp             string
	journal             io.Writer
	now                 func() time.Time
	sequence            int
	failed              bool
	browsers            map[string]time.Time
	idpBrowsers         map[string]time.Time
	pending             map[string]pending
	forms               map[string]authorization
	codes               map[string]code
	accounts            map[string]account
	loginResponseDelay  time.Duration
	autoAuthorizeReturn bool // operator-only pre-consent callback-denial fault
	reflectFormToken    bool // private hidden token reflected in an unsupported action path
	deniedCallbackPath  bool // operator-only post-login redirect path denial fault
}

func opaque() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("fixture random source unavailable")
	}
	return hex.EncodeToString(b[:])
}
func challenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
func newFixture(c config, rp, idp string, journal io.Writer) *fixture {
	return &fixture{cfg: c, rp: rp, idp: idp, journal: journal, now: time.Now,
		browsers: map[string]time.Time{}, idpBrowsers: map[string]time.Time{},
		pending: map[string]pending{}, forms: map[string]authorization{}, codes: map[string]code{}, accounts: map[string]account{}}
}
func (f *fixture) record(kind string, accepted bool) bool {
	if f.failed {
		return false
	}
	f.sequence++
	err := json.NewEncoder(f.journal).Encode(receipt{f.sequence, f.now().UTC(), kind, accepted})
	if syncer, ok := f.journal.(interface{ Sync() error }); ok && err == nil {
		err = syncer.Sync()
	}
	if err != nil {
		f.failed = true
	}
	return !f.failed
}
func (f *fixture) prune() {
	now := f.now()
	for k, v := range f.browsers {
		if !v.After(now) {
			delete(f.browsers, k)
		}
	}
	for k, v := range f.idpBrowsers {
		if !v.After(now) {
			delete(f.idpBrowsers, k)
		}
	}
	for k, v := range f.pending {
		if !v.expires.After(now) {
			delete(f.pending, k)
		}
	}
	for k, v := range f.forms {
		if !v.expires.After(now) {
			delete(f.forms, k)
		}
	}
	for k, v := range f.codes {
		if !v.expires.After(now) {
			delete(f.codes, k)
		}
	}
	for k, v := range f.accounts {
		if !v.expires.After(now) {
			delete(f.accounts, k)
		}
	}
}
func (f *fixture) cookie(w http.ResponseWriter, name, value, origin string) {
	ttl := flowTTL
	if name == "sso_rp_binding" {
		ttl = rpBindingTTL
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", Secure: strings.HasPrefix(origin, "https://"), HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: int(ttl / time.Second)})
}
func cookie(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}
func page(w http.ResponseWriter, title, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<!doctype html><html><meta charset=utf-8><title>%s</title><body><h1>%s</h1>%s</body></html>", html.EscapeString(title), html.EscapeString(title), body)
}
func exactKeys(v url.Values, keys ...string) bool {
	if len(v) != len(keys) {
		return false
	}
	for _, key := range keys {
		if len(v[key]) != 1 || v.Get(key) == "" {
			return false
		}
	}
	return true
}
func (f *fixture) handler(idp bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		// Suppress URL query/path while retaining a non-null same-origin form
		// Origin in Chromium. no-referrer makes navigation POST Origin null.
		w.Header().Set("Referrer-Policy", "strict-origin")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
		origin := f.rp
		if idp {
			origin = f.idp
			// Chromium applies form-action to the provider's redirect as well.
			// Permit only the configured RP; redirect_uri is also checked exactly.
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; form-action 'self' "+f.rp+"; frame-ancestors 'none'")
		}
		u, _ := url.Parse(origin)
		if r.Host != u.Host {
			http.Error(w, "Unexpected host", 400)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		if r.Method == http.MethodPost {
			if !idp || r.URL.Path != f.publicPath("/login") || r.URL.RawQuery != "" || r.Header.Get("Origin") != f.idp {
				http.Error(w, "Rejected request", 400)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 8192)
			if r.ParseForm() != nil || !exactKeys(r.PostForm, f.userField(), f.passwordField(), "csrf") {
				http.Error(w, "Rejected form", 400)
				return
			}
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failed {
			http.Error(w, "Recipient unavailable", 503)
			return
		}
		f.prune()
		if idp {
			f.provider(w, r)
		} else {
			f.relyingParty(w, r)
		}
	})
}

func (f *fixture) relyingParty(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	switch r.URL.Path {
	case "/":
		browser := cookie(r, "sso_rp_binding")
		if !f.browsers[browser].After(f.now()) {
			if len(f.browsers) >= 128 {
				http.Error(w, "Browser limit", 429)
				return
			}
			browser = opaque()
			f.browsers[browser] = f.now().Add(rpBindingTTL)
			f.cookie(w, "sso_rp_binding", browser, f.rp)
		}
		loginID := "login"
		if f.cfg.PublicVariant != "" {
			loginID = "signin-" + f.cfg.PublicVariant
		}
		buttons := []string{`<button id="` + html.EscapeString(loginID) + `">Sign in with test provider</button>`, `<button id="late">Delayed popup</button>`, `<button id="twins">Two identical titles</button>`, `<button id="storm">Popup storm</button>`}
		if f.cfg.PublicVariant != "" {
			rotation := int(f.cfg.PublicVariant[0]) % 4
			buttons = append(buttons[rotation:], buttons[:rotation]...)
		}
		page(w, "SSO fixture", strings.Join(buttons, "")+`<a href="`+html.EscapeString(f.publicPath("/account"))+`">Protected account</a><p id="status">Signed out</p><script>
let active;
function openLogin(){active=window.open('/start','_blank','width=600,height=700')}
document.querySelector('#`+loginID+`').onclick=openLogin;
document.querySelector('#late').onclick=()=>setTimeout(openLogin,150);
document.querySelector('#twins').onclick=()=>{window.open('/start','_blank');openLogin()};
document.querySelector('#storm').onclick=()=>{for(let i=0;i<12;i++)window.open('/blank','_blank')};
addEventListener('message',e=>{if(e.origin===location.origin&&e.source===active&&e.data?.kind==='sso_complete')location.assign('`+f.publicPath("/account")+`')});
</script>`)
	case "/start":
		browser := cookie(r, "sso_rp_binding")
		if !f.browsers[browser].After(f.now()) {
			http.Error(w, "Missing browser binding", 403)
			return
		}
		if len(f.pending) >= 64 {
			http.Error(w, "Flow limit", 429)
			return
		}
		state, verifier := opaque(), opaque()
		f.pending[state] = pending{browser, verifier, f.now().Add(flowTTL)}
		u, _ := url.Parse(f.idp + f.publicPath("/authorize"))
		u.RawQuery = url.Values{"client_id": {"fixture-rp"}, "redirect_uri": {f.rp + "/callback"}, "response_type": {"code"}, "state": {state}, "code_challenge": {challenge(verifier)}, "code_challenge_method": {"S256"}}.Encode()
		http.Redirect(w, r, u.String(), 302)
	case "/denied/callback":
		f.record("forbidden_redirect_read", true)
		http.Error(w, "Unexpected redirect", 403)
	case "/callback":
		q, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || !exactKeys(q, "code", "state") {
			http.Error(w, "Invalid callback", 400)
			return
		}
		p, ok := f.pending[q.Get("state")]
		c, valid := f.codes[q.Get("code")]
		if !ok || !valid || !p.expires.After(f.now()) || !c.expires.After(f.now()) || !f.browsers[p.browser].After(f.now()) || p.browser != cookie(r, "sso_rp_binding") || c.state != q.Get("state") || c.challenge != challenge(p.verifier) {
			http.Error(w, "Invalid callback", 403)
			return
		}
		if len(f.accounts) >= 128 {
			http.Error(w, "Account limit", 429)
			return
		}
		delete(f.pending, q.Get("state"))
		delete(f.codes, q.Get("code"))
		if !f.record("callback_accepted", true) {
			http.Error(w, "Recipient unavailable", 503)
			return
		}
		session := opaque()
		f.accounts[session] = account{p.browser, f.now().Add(flowTTL)}
		f.cookie(w, "sso_rp_auth", session, f.rp)
		page(w, "Sign-in complete", `<p>Return to the original window.</p><script>if(opener){opener.postMessage({kind:'sso_complete'},location.origin);window.close()}</script>`)
	case f.publicPath("/account"):
		a, ok := f.accounts[cookie(r, "sso_rp_auth")]
		if !ok || !a.expires.After(f.now()) || !f.browsers[a.browser].After(f.now()) || a.browser != cookie(r, "sso_rp_binding") {
			http.Error(w, "Sign in required", 401)
			return
		}
		if !f.record("account_read", true) {
			http.Error(w, "Recipient unavailable", 503)
			return
		}
		page(w, "Private account", "<p id=private-ref>"+html.EscapeString(f.cfg.PrivateFact)+"</p>")
	case "/blank":
		page(w, "Test provider", "<p>Unrelated window</p>")
	default:
		http.NotFound(w, r)
	}
}

func (f *fixture) provider(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case f.publicPath("/authorize"):
		q, err := url.ParseQuery(r.URL.RawQuery)
		if r.Method != http.MethodGet || err != nil || !exactKeys(q, "client_id", "redirect_uri", "response_type", "state", "code_challenge", "code_challenge_method") || q.Get("client_id") != "fixture-rp" || q.Get("redirect_uri") != f.rp+"/callback" || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" {
			http.Error(w, "Invalid authorization", 400)
			return
		}
		state, stateErr := hex.DecodeString(q.Get("state"))
		hash, hashErr := base64.RawURLEncoding.DecodeString(q.Get("code_challenge"))
		if stateErr != nil || len(state) != 32 || hashErr != nil || len(hash) != 32 || len(f.forms) >= 64 {
			http.Error(w, "Invalid authorization", 400)
			return
		}
		browser := cookie(r, "sso_idp_binding")
		if !f.idpBrowsers[browser].After(f.now()) {
			if len(f.idpBrowsers) >= 128 {
				http.Error(w, "Browser limit", 429)
				return
			}
			browser = opaque()
			f.idpBrowsers[browser] = f.now().Add(flowTTL)
			f.cookie(w, "sso_idp_binding", browser, f.idp)
		}
		csrf := opaque()
		if f.autoAuthorizeReturn {
			value := opaque()
			f.codes[value] = code{q.Get("state"), q.Get("code_challenge"), f.now().Add(flowTTL)}
			u, _ := url.Parse(f.rp + "/callback")
			u.RawQuery = url.Values{"code": {value}, "state": {q.Get("state")}}.Encode()
			if !f.record("automatic_authorization", true) {
				http.Error(w, "Recipient unavailable", 503)
				return
			}
			http.Redirect(w, r, u.String(), 302)
			return
		}
		f.forms[csrf] = authorization{browser, q.Get("state"), q.Get("code_challenge"), f.now().Add(flowTTL)}
		prefix := ""
		if f.cfg.PublicVariant != "" {
			prefix = `<form method=get action=/help><label>Help topic<input name=topic></label><button>Help</button></form>`
		}
		target := f.publicPath("/login")
		if f.reflectFormToken {
			target += "/" + csrf
		}
		page(w, "Test provider", prefix+`<form method=post action="`+html.EscapeString(target)+`"><label>Username<input name="`+html.EscapeString(f.userField())+`" autocomplete=off></label><label>Password<input type=password name="`+html.EscapeString(f.passwordField())+`" autocomplete=off></label><input type=hidden name=csrf value="`+csrf+`"><button>Sign in</button></form>`)
	case f.publicPath("/login"):
		if r.Method != http.MethodPost {
			w.WriteHeader(405)
			return
		}
		a, ok := f.forms[r.PostForm.Get("csrf")]
		if !ok || !a.expires.After(f.now()) || !f.idpBrowsers[a.browser].After(f.now()) || a.browser != cookie(r, "sso_idp_binding") {
			http.Error(w, "Invalid form binding", 403)
			return
		}
		delete(f.forms, r.PostForm.Get("csrf"))
		valid := subtle.ConstantTimeCompare([]byte(r.PostForm.Get(f.userField())), []byte(f.cfg.Username)) == 1 && subtle.ConstantTimeCompare([]byte(r.PostForm.Get(f.passwordField())), []byte(f.cfg.Password)) == 1
		if valid && len(f.codes) >= 128 {
			http.Error(w, "Code limit", 429)
			return
		}
		if !f.record("idp_login", valid) {
			http.Error(w, "Recipient unavailable", 503)
			return
		}
		if !valid {
			http.Error(w, "Sign-in failed", 401)
			return
		}
		// Operator-only fault injection: the accepted login is durable, but its
		// callback has not been released. Cancellation cannot replay the form.
		if f.loginResponseDelay > 0 {
			timer := time.NewTimer(f.loginResponseDelay)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-r.Context().Done():
				return
			}
		}
		value := opaque()
		f.codes[value] = code{a.state, a.challenge, f.now().Add(flowTTL)}
		u, _ := url.Parse(f.rp + "/callback")
		u.RawQuery = url.Values{"code": {value}, "state": {a.state}}.Encode()
		if f.deniedCallbackPath {
			u.Path = "/denied/callback"
		}
		http.Redirect(w, r, u.String(), 303)
	default:
		http.NotFound(w, r)
	}
}
