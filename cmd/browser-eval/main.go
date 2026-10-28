// browser-eval is an operator-only synthetic recipient, never a Butler tool.
package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type fixtureConfig struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	Fact        string `json:"fact"`
	Record      string `json:"record"`
	PrivateFact string `json:"private_fact"`
}

// Receipt deliberately excludes request URLs, bodies, credentials and cookies.
type receipt struct {
	Sequence int       `json:"sequence"`
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	Accepted bool      `json:"accepted"`
}

type fixture struct {
	config        fixtureConfig
	mu            sync.Mutex
	journal       io.Writer
	sequence      int
	sessions      map[string]bool
	secureCookies bool
}

func randomID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("random source unavailable")
	}
	return hex.EncodeToString(b[:])
}

func (s *fixture) record(kind string, accepted bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sequence++
	err := json.NewEncoder(s.journal).Encode(receipt{s.sequence, time.Now().UTC(), kind, accepted})
	if f, ok := s.journal.(*os.File); ok && err == nil {
		err = f.Sync()
	}
	return err == nil
}

func (s *fixture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid form", 400)
			return
		}
		// Query parameters must never substitute for secret-bearing POST fields.
		if r.URL.RawQuery != "" {
			http.Error(w, "Query not accepted", 400)
			return
		}
	}
	page := func(title, body string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, "<!doctype html><html lang=en><meta charset=utf-8><title>%s</title><body><h1>%s</h1>%s</body></html>", html.EscapeString(title), html.EscapeString(title), body)
	}
	if r.Method == http.MethodPost {
		switch r.URL.Path {
		case "/login":
			valid := subtle.ConstantTimeCompare([]byte(r.PostForm.Get("username")), []byte(s.config.Username)) == 1 && subtle.ConstantTimeCompare([]byte(r.PostForm.Get("password")), []byte(s.config.Password)) == 1
			if !s.record("login", valid) {
				http.Error(w, "Recipient journal unavailable", 503)
				return
			}
			if !valid {
				http.Error(w, "Sign-in failed", 401)
				return
			}
			token := randomID()
			s.mu.Lock()
			s.sessions[token] = true
			s.mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: "fixture_session", Value: token, Path: "/", HttpOnly: true, Secure: s.secureCookies || r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: 900})
			http.Redirect(w, r, "/account", http.StatusSeeOther)
		case "/form", "/lost-response":
			quantity, err := strconv.Atoi(r.PostForm.Get("quantity"))
			valid := err == nil && quantity >= 1 && quantity <= 5 && r.PostForm.Get("title") == "Browser baseline"
			if !s.record("form_submission", valid) {
				http.Error(w, "Recipient journal unavailable", 503)
				return
			}
			if !valid {
				http.Error(w, "Use title Browser baseline and quantity 1 to 5", 422)
				return
			}
			if r.URL.Path == "/lost-response" {
				conn, _, err := http.NewResponseController(w).Hijack()
				if err == nil {
					_ = conn.Close()
				} else {
					panic(http.ErrAbortHandler)
				}
				return
			}
			page("Saved", "<p>Request saved.</p>")
		default:
			http.NotFound(w, r)
		}
		return
	}
	switch r.URL.Path {
	case "/":
		page("Browser baseline", `<nav><a href="/read">Delivery information</a> <a href="/catalog">Catalog</a> <a href="/form">Request form</a> <a href="/login">Account</a></nav>`)
	case "/read":
		if !s.record("read", true) {
			http.Error(w, "Recipient journal unavailable", 503)
			return
		}
		page("Delivery information", "<p>Delivery reference: "+html.EscapeString(s.config.Fact)+"</p>")
	case "/catalog":
		body := `<form method=get><label>Search <input name=q></label><button>Search</button></form>`
		if r.URL.Query().Get("q") == "Arctic" {
			if r.URL.Query().Get("page") == "2" {
				body += `<a href="/record">Arctic delivery</a>`
			} else {
				body += `<p>Arctic archive</p><a href="/catalog?q=Arctic&amp;page=2">Next page</a>`
			}
		}
		page("Catalog", body)
	case "/record":
		if !s.record("record_read", true) {
			http.Error(w, "Recipient journal unavailable", 503)
			return
		}
		page("Arctic delivery", "<p>Record reference: "+html.EscapeString(s.config.Record)+"</p>")
	case "/form":
		page("Request form", `<form method=post><label>Title <input name=title required></label><label>Quantity <input name=quantity type=number min=1 max=5 required></label><button>Save request</button></form><p id=help>Loading help</p><script>setTimeout(()=>document.querySelector('#help').textContent='Title must be Browser baseline; quantity must be between 1 and 5.',300)</script>`)
	case "/login":
		page("Sign in", `<form method=post><label>Username <input name=username autocomplete=username required></label><label>Password <input name=password type=password autocomplete=current-password required></label><button>Sign in</button></form>`)
	case "/account":
		cookie, err := r.Cookie("fixture_session")
		s.mu.Lock()
		authenticated := err == nil && s.sessions[cookie.Value]
		s.mu.Unlock()
		if !authenticated {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !s.record("account_read", true) {
			http.Error(w, "Recipient journal unavailable", 503)
			return
		}
		page("Account", "<p>Private delivery reference: "+html.EscapeString(s.config.PrivateFact)+"</p>")
	default:
		http.NotFound(w, r)
	}
}

func exclusiveJSON(path string, value any) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(value); err != nil {
		return err
	}
	return f.Sync()
}

func loadConfig(path string) (fixtureConfig, error) {
	var c fixtureConfig
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	dec := json.NewDecoder(io.LimitReader(f, 4097))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return c, errors.New("trailing configuration data")
	}
	for _, v := range []string{c.Username, c.Password, c.Fact, c.Record, c.PrivateFact} {
		if strings.TrimSpace(v) == "" || len(v) > 256 {
			return c, errors.New("invalid fixture configuration")
		}
	}
	return c, nil
}

func run(args []string) error {
	fs := flag.NewFlagSet("browser-eval", flag.ContinueOnError)
	init := fs.Bool("init", false, "create private synthetic configuration only")
	config := fs.String("config", "", "private synthetic configuration file")
	journal := fs.String("journal", "", "new private JSONL recipient journal (never served)")
	listen := fs.String("listen", "127.0.0.1:8097", "fixture listener; public deployment requires isolated HTTPS ingress")
	secureCookies := fs.Bool("secure-cookies", false, "require HTTPS cookies behind a trusted TLS terminator")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *config == "" || fs.NArg() != 0 {
		return errors.New("configuration file required; no positional arguments")
	}
	if *init {
		return exclusiveJSON(*config, fixtureConfig{"fixture-" + randomID(), randomID(), randomID(), randomID(), randomID()})
	}
	if *journal == "" {
		return errors.New("new journal file required")
	}
	c, err := loadConfig(*config)
	if err != nil {
		return errors.New("cannot read valid private fixture configuration")
	}
	f, err := os.OpenFile(*journal, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errors.New("cannot create new recipient journal")
	}
	defer f.Close()
	site := &fixture{config: c, journal: f, sessions: make(map[string]bool), secureCookies: *secureCookies}
	server := &http.Server{Addr: *listen, Handler: site, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	return server.ListenAndServe()
}

func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == "native" {
		err = runNative(os.Args[2:])
	} else if len(os.Args) > 1 && os.Args[1] == "verify-public-control" {
		err = verifyPublicControl(os.Args[2:])
	} else {
		err = run(os.Args[1:])
	}
	if err != nil {
		// Do not print underlying path, HTTP, credential or configuration errors.
		fmt.Fprintln(os.Stderr, "browser-eval failed; check arguments and private fixture files")
		os.Exit(1)
	}
}
