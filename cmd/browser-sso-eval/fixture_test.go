package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testSite struct {
	f       *fixture
	rp, idp *httptest.Server
	c       *http.Client
	journal *bytes.Buffer
	offset  atomic.Int64
}

func client() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func site(t *testing.T) *testSite {
	t.Helper()
	s := &testSite{c: client(), journal: &bytes.Buffer{}}
	s.f = newFixture(config{"sso-user-canary", "sso-password-canary", "PRIVATE-SSO-REFERENCE"}, "", "", s.journal)
	s.f.now = func() time.Time { return time.Now().Add(time.Duration(s.offset.Load())) }
	s.rp = httptest.NewServer(s.f.handler(false))
	s.idp = httptest.NewServer(s.f.handler(true))
	s.f.rp, s.f.idp = s.rp.URL, s.idp.URL
	t.Cleanup(s.rp.Close)
	t.Cleanup(s.idp.Close)
	return s
}
func request(t *testing.T, c *http.Client, method, target string, fields url.Values, origin string) (*http.Response, string) {
	t.Helper()
	body := ""
	if fields != nil {
		body = fields.Encode()
	}
	r, err := http.NewRequest(method, target, strings.NewReader(body))
	if err != nil {
		t.Fatal("invalid test request")
	}
	if fields != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	response, err := c.Do(r)
	if err != nil {
		t.Fatal("fixture HTTP request failed")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16385))
	if err != nil || len(raw) > 16384 {
		t.Fatal("unbounded fixture response")
	}
	return response, string(raw)
}
func (s *testSite) begin(t *testing.T) (string, string) {
	t.Helper()
	r, _ := request(t, s.c, "GET", s.rp.URL+"/", nil, "")
	if r.StatusCode != 200 {
		t.Fatal("RP entry unavailable")
	}
	r, _ = request(t, s.c, "GET", s.rp.URL+"/start", nil, "")
	if r.StatusCode != 302 {
		t.Fatal("missing authorize redirect")
	}
	authorize := r.Header.Get("Location")
	r, body := request(t, s.c, "GET", authorize, nil, "")
	if r.StatusCode != 200 {
		t.Fatal("provider form unavailable")
	}
	match := regexp.MustCompile(`name=csrf value="([a-f0-9]{64})"`).FindStringSubmatch(body)
	if len(match) != 2 {
		t.Fatal("missing private form CSRF")
	}
	return authorize, match[1]
}
func (s *testSite) login(t *testing.T, csrf, password string) (*http.Response, string) {
	t.Helper()
	r, _ := request(t, s.c, "POST", s.idp.URL+"/login", url.Values{"username": {s.f.cfg.Username}, "password": {password}, "csrf": {csrf}}, s.idp.URL)
	return r, r.Header.Get("Location")
}
func assertCount(t *testing.T, raw, kind string, want int) {
	t.Helper()
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.Contains(line, `"kind":"`+kind+`"`) && strings.Contains(line, `"accepted":true`) {
			count++
		}
	}
	if count != want {
		t.Fatal("wrong accepted recipient count", kind, count, want)
	}
}
func TestSSOFixtureLoginCallbackAndPrivateAccount(t *testing.T) {
	s := site(t)
	r, _ := request(t, s.c, "GET", s.rp.URL+"/account", nil, "")
	if r.StatusCode != 401 {
		t.Fatal("anonymous account access")
	}
	_, csrf := s.begin(t)
	r, callback := s.login(t, csrf, s.f.cfg.Password)
	if r.StatusCode != 303 || !strings.HasPrefix(callback, s.rp.URL+"/callback?") {
		t.Fatal("missing code callback")
	}
	r, _ = request(t, s.c, "GET", s.rp.URL+"/account", nil, "")
	if r.StatusCode != 401 {
		t.Fatal("provider login alone authenticated RP")
	}
	r, body := request(t, s.c, "GET", callback, nil, "")
	if r.StatusCode != 200 || strings.Contains(body, "code=") || strings.Contains(body, "state=") {
		t.Fatal("callback exposed tokens")
	}
	r, body = request(t, s.c, "GET", s.rp.URL+"/account", nil, "")
	if r.StatusCode != 200 || !strings.Contains(body, s.f.cfg.PrivateFact) {
		t.Fatal("missing private account result")
	}
	assertCount(t, s.journal.String(), "idp_login", 1)
	assertCount(t, s.journal.String(), "callback_accepted", 1)
	assertCount(t, s.journal.String(), "account_read", 1)
	u, _ := url.Parse(callback)
	for _, secret := range []string{s.f.cfg.Username, s.f.cfg.Password, s.f.cfg.PrivateFact, csrf, u.Query().Get("code"), u.Query().Get("state")} {
		if secret == "" || strings.Contains(s.journal.String(), secret) {
			t.Fatal("recipient journal exposed private material")
		}
	}
}
func TestSSOFixtureRejectsForeignExpiredReplayedAndSwappedFlows(t *testing.T) {
	for _, mode := range []string{"wrong_password", "csrf_replay", "foreign_idp_browser", "foreign_rp_browser", "state_swap", "pkce_mismatch", "expiry", "callback_replay", "wrong_redirect", "duplicate_query", "missing_origin", "query_credentials"} {
		t.Run(mode, func(t *testing.T) {
			s := site(t)
			authorize, csrf := s.begin(t)
			switch mode {
			case "wrong_redirect", "duplicate_query":
				u, _ := url.Parse(authorize)
				q := u.Query()
				if mode == "wrong_redirect" {
					q.Set("redirect_uri", "https://other.test/callback")
				} else {
					q.Add("state", opaque())
				}
				u.RawQuery = q.Encode()
				r, _ := request(t, s.c, "GET", u.String(), nil, "")
				if r.StatusCode != 400 {
					t.Fatal("unbound authorize accepted")
				}
				return
			case "pkce_mismatch":
				s.f.mu.Lock()
				a := s.f.forms[csrf]
				a.challenge = challenge("different-verifier")
				s.f.forms[csrf] = a
				s.f.mu.Unlock()
			case "expiry":
				s.offset.Store(int64(flowTTL + time.Second))
			case "foreign_idp_browser", "missing_origin", "query_credentials":
				c := s.c
				origin := s.idp.URL
				target := s.idp.URL + "/login"
				if mode == "foreign_idp_browser" {
					c = client()
				}
				if mode == "missing_origin" {
					origin = ""
				}
				if mode == "query_credentials" {
					target += "?password=not-a-form-field"
				}
				r, _ := request(t, c, "POST", target, url.Values{"username": {s.f.cfg.Username}, "password": {s.f.cfg.Password}, "csrf": {csrf}}, origin)
				if r.StatusCode < 400 {
					t.Fatal("unbound secret submission accepted")
				}
				assertCount(t, s.journal.String(), "idp_login", 0)
				return
			}
			password := s.f.cfg.Password
			if mode == "wrong_password" {
				password = "wrong-password-canary"
			}
			r, callback := s.login(t, csrf, password)
			if mode == "wrong_password" || mode == "expiry" {
				if r.StatusCode < 400 {
					t.Fatal("invalid login accepted")
				}
				assertCount(t, s.journal.String(), "callback_accepted", 0)
				return
			}
			if r.StatusCode != 303 {
				t.Fatal("initial login rejected")
			}
			if mode == "csrf_replay" {
				r, _ = s.login(t, csrf, password)
				if r.StatusCode != 403 {
					t.Fatal("form replay accepted")
				}
				assertCount(t, s.journal.String(), "idp_login", 1)
				return
			}
			if mode == "state_swap" {
				_, secondCSRF := s.begin(t)
				second, other := s.login(t, secondCSRF, password)
				if second.StatusCode != 303 {
					t.Fatal("second flow unavailable")
				}
				u, _ := url.Parse(callback)
				v, _ := url.Parse(other)
				q := u.Query()
				q.Set("state", v.Query().Get("state"))
				u.RawQuery = q.Encode()
				callback = u.String()
			}
			c := s.c
			if mode == "foreign_rp_browser" {
				c = client()
			}
			r, _ = request(t, c, "GET", callback, nil, "")
			if mode == "callback_replay" {
				if r.StatusCode != 200 {
					t.Fatal("initial callback failed")
				}
				r, _ = request(t, s.c, "GET", callback, nil, "")
				if r.StatusCode != 403 {
					t.Fatal("code/state replay accepted")
				}
				assertCount(t, s.journal.String(), "callback_accepted", 1)
			} else {
				if r.StatusCode != 403 {
					t.Fatal("foreign state or proof accepted")
				}
				assertCount(t, s.journal.String(), "callback_accepted", 0)
			}
		})
	}
}

type failingJournal struct {
	fail atomic.Bool
	body bytes.Buffer
}

func (j *failingJournal) Write(p []byte) (int, error) {
	if j.fail.Load() {
		return 0, errors.New("private fixture journal failure")
	}
	return j.body.Write(p)
}
func TestSSOFixtureJournalFailureNeverCreatesAuthCookie(t *testing.T) {
	for _, phase := range []string{"provider", "callback", "account"} {
		t.Run(phase, func(t *testing.T) {
			s := site(t)
			j := &failingJournal{}
			s.f.journal = j
			_, csrf := s.begin(t)
			if phase == "provider" {
				j.fail.Store(true)
			}
			r, callback := s.login(t, csrf, s.f.cfg.Password)
			if phase == "provider" {
				if r.StatusCode != 503 {
					t.Fatal("provider proceeded without journal")
				}
				return
			}
			if phase == "callback" {
				j.fail.Store(true)
			}
			r, _ = request(t, s.c, "GET", callback, nil, "")
			if phase == "callback" {
				if r.StatusCode != 503 {
					t.Fatal("callback proceeded without journal")
				}
				u, _ := url.Parse(s.rp.URL)
				for _, c := range s.c.Jar.Cookies(u) {
					if c.Name == "sso_rp_auth" {
						t.Fatal("auth cookie issued without durable receipt")
					}
				}
				return
			}
			j.fail.Store(true)
			r, body := request(t, s.c, "GET", s.rp.URL+"/account", nil, "")
			if r.StatusCode != 503 || strings.Contains(body, s.f.cfg.PrivateFact) {
				t.Fatal("private fact released after journal failure")
			}
		})
	}
}
func TestSSOFixtureFlowLimitExpires(t *testing.T) {
	s := site(t)
	request(t, s.c, "GET", s.rp.URL+"/", nil, "")
	for range 64 {
		r, _ := request(t, s.c, "GET", s.rp.URL+"/start", nil, "")
		if r.StatusCode != 302 {
			t.Fatal("flow limit applied too early")
		}
	}
	r, _ := request(t, s.c, "GET", s.rp.URL+"/start", nil, "")
	if r.StatusCode != 429 {
		t.Fatal("unbounded pending flows")
	}
	s.offset.Store(int64(flowTTL + time.Second))
	request(t, s.c, "GET", s.rp.URL+"/", nil, "")
	r, _ = request(t, s.c, "GET", s.rp.URL+"/start", nil, "")
	if r.StatusCode != 302 {
		t.Fatal("expired flows retained capacity")
	}
}
