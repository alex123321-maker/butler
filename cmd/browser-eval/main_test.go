package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testSite(t *testing.T) (*httptest.Server, *http.Client, *bytes.Buffer) {
	t.Helper()
	journal := new(bytes.Buffer)
	s := httptest.NewServer(&fixture{config: fixtureConfig{"test-user", "canary-password", "public-reference", "record-reference", "private-reference"}, journal: journal, sessions: make(map[string]bool)})
	t.Cleanup(s.Close)
	jar, _ := cookiejar.New(nil)
	return s, &http.Client{Jar: jar}, journal
}

func readResponse(t *testing.T, r *http.Response, err error) (int, string) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return r.StatusCode, string(b)
}

func get(t *testing.T, c *http.Client, u string) (int, string) {
	t.Helper()
	r, err := c.Get(u)
	return readResponse(t, r, err)
}

func post(t *testing.T, c *http.Client, u string, v url.Values) (int, string) {
	t.Helper()
	r, err := c.PostForm(u, v)
	return readResponse(t, r, err)
}

func TestPrivateAccountRequiresActualLogin(t *testing.T) {
	s, c, journal := testSite(t)
	_, body := get(t, c, s.URL+"/account")
	if strings.Contains(body, "private-reference") {
		t.Fatal("unauthenticated disclosure")
	}
	status, body := post(t, c, s.URL+"/login", url.Values{"username": {"test-user"}, "password": {"wrong"}})
	if status != 401 || strings.Contains(body, "wrong") {
		t.Fatal("wrong password accepted or reflected")
	}
	status, body = post(t, c, s.URL+"/login", url.Values{"username": {"test-user"}, "password": {"canary-password"}})
	if status != 200 || !strings.Contains(body, "private-reference") {
		t.Fatal("login did not yield protected data")
	}
	if strings.Contains(journal.String(), "canary") || strings.Contains(journal.String(), "test-user") || strings.Contains(journal.String(), "private-reference") {
		t.Fatal("journal contains private input or answer")
	}
	dec := json.NewDecoder(strings.NewReader(journal.String()))
	for i, want := range []receipt{{Kind: "login", Accepted: false}, {Kind: "login", Accepted: true}, {Kind: "account_read", Accepted: true}} {
		var got receipt
		if err := dec.Decode(&got); err != nil {
			t.Fatal(err)
		}
		if got.Sequence != i+1 || got.Kind != want.Kind || got.Accepted != want.Accepted || got.At.IsZero() {
			t.Fatalf("unexpected receipt: %+v", got)
		}
	}
	other := &http.Client{}
	_, body = get(t, other, s.URL+"/account")
	if strings.Contains(body, "private-reference") {
		t.Fatal("session leaked to another client")
	}
}

func TestLostResponseRetainsAppliedEffectAndCountsReplay(t *testing.T) {
	s, c, journal := testSite(t)
	values := url.Values{"title": {"Browser baseline"}, "quantity": {"3"}}
	for range 2 {
		r, err := c.PostForm(s.URL+"/lost-response", values)
		if err == nil {
			r.Body.Close()
			t.Fatal("expected lost response")
		}
	}
	if strings.Count(journal.String(), `"accepted":true`) != 2 {
		t.Fatal("recipient hid duplicate effects")
	}
	status, _ := post(t, c, s.URL+"/form", url.Values{"title": {"Browser baseline"}, "quantity": {"99"}})
	if status != 422 || strings.Count(journal.String(), `"accepted":false`) != 1 {
		t.Fatal("invalid form applied")
	}
}

func TestFixtureRoutesDoNotExposeOracleOrConfiguration(t *testing.T) {
	s, c, journal := testSite(t)
	for _, path := range []string{"/oracle", "/config", "/journal", "/../config", "/config.json"} {
		status, _ := get(t, c, s.URL+path)
		if status != 404 {
			t.Fatalf("unexpected exposed route %s: %d", path, status)
		}
	}
	status, _ := post(t, c, s.URL+"/login?password=canary-password", url.Values{"username": {"test-user"}})
	if status != 400 || journal.Len() != 0 {
		t.Fatal("secret query accepted")
	}
	for _, path := range []string{"/read", "/catalog?q=Arctic", "/catalog?q=Arctic&page=2", "/catalog?q=Arctic+delivery", "/catalog?q=arctic+delivery&page=2", "/record"} {
		status, body := get(t, c, s.URL+path)
		if status != 200 || strings.Contains(body, "canary-password") {
			t.Fatal("navigation fixture failed")
		}
		if strings.HasPrefix(path, "/catalog?") {
			link := "Next page"
			if strings.Contains(path, "page=2") {
				link = "Arctic delivery"
			}
			if !strings.Contains(body, link) {
				t.Fatal("normal search query did not expose the expected navigation")
			}
		}
	}
	if strings.Contains(journal.String(), "Arctic") {
		t.Fatal("journal contains request query")
	}
}

type brokenJournal struct{}

func (brokenJournal) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestJournalFailureDoesNotGrantLogin(t *testing.T) {
	s := &fixture{config: fixtureConfig{Username: "u", Password: "p"}, journal: brokenJournal{}, sessions: make(map[string]bool)}
	r := httptest.NewRequest("POST", "/login", strings.NewReader("username=u&password=p"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 503 || len(w.Result().Cookies()) != 0 || len(s.sessions) != 0 {
		t.Fatal("journal failure allowed login")
	}
}

func TestPrivateConfigurationIsStrictAndNeverOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := run([]string{"-init", "-config", path}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if err := run([]string{"-init", "-config", path}); err == nil {
		t.Fatal("overwrote configuration")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("configuration changed")
	}
	if _, err := loadConfig(path); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{}`, string(before) + `{}`, strings.Replace(string(before), `"username"`, `"unknown"`, 1)} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := loadConfig(path); err == nil {
			t.Fatal("accepted invalid configuration")
		}
	}
}

func TestTLSProxyModeForcesSecureCookieWithoutTrustingForwardedHeaders(t *testing.T) {
	for _, secure := range []bool{false, true} {
		s := &fixture{config: fixtureConfig{Username: "u", Password: "p"}, journal: new(bytes.Buffer), sessions: make(map[string]bool), secureCookies: secure}
		r := httptest.NewRequest("POST", "http://fixture/login", strings.NewReader("username=u&password=p"))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("X-Forwarded-Proto", "https")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		cookies := w.Result().Cookies()
		if len(cookies) != 1 || cookies[0].Secure != secure || !cookies[0].HttpOnly {
			t.Fatal("TLS proxy cookie policy failed")
		}
	}
}
