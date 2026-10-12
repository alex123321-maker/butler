// antigravity-compare is a private operator backend, never a production provider.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

const privateRoot = "/private"

var bootPhase = "INVOCATION"

func main() {
	if len(os.Args) == 2 && os.Args[1] == "high-inspect" {
		if err := inspectHigh(); err != nil {
			safeExit("HIGH_INSPECTION_FAILED")
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "high" {
		if err := runHigh(); err != nil {
			safeExit("HIGH_BACKEND_FAILED")
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "-probe-network" {
		probeNetwork()
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "-control" {
		control(os.Args[2])
		return
	}
	if len(os.Args) != 2 || (os.Args[1] != "inspect" && os.Args[1] != "execute" && os.Args[1] != "native8192") {
		safeExit("INVOCATION_DENIED")
	}
	if err := serve(os.Args[1]); err != nil {
		safeExit("PRIVATE_BACKEND_FAILED")
	}
}

func safeExit(reason string) {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": reason, "phase": bootPhase, "raw_diagnostics_suppressed": true})
	os.Exit(1)
}

func control(action string) {
	switch action {
	case "ready", "report", "run-go", "post-metadata", "shutdown":
	default:
		safeExit("CONTROL_DENIED")
	}
	key, err := os.ReadFile(filepath.Join(privateRoot, "control-key"))
	if err != nil {
		safeExit("CONTROL_UNAVAILABLE")
	}
	req, _ := http.NewRequest("POST", "http://127.0.0.1:3128/"+action, nil)
	req.Header.Set("X-Diagnostic-Key", string(key))
	clear(key)
	client := http.Client{Transport: &http.Transport{}, Timeout: 55 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		safeExit("CONTROL_UNAVAILABLE")
	}
	defer resp.Body.Close()
	_, _ = io.Copy(os.Stdout, io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode != 200 {
		os.Exit(1)
	}
}

func privateRead(path string) ([]byte, error) {
	s, err := os.Lstat(path)
	if err != nil || !s.Mode().IsRegular() || s.Mode().Perm() != 0600 || s.Size() > 65536 {
		return nil, denied
	}
	return os.ReadFile(path)
}

func serve(mode string) error {
	bootPhase = "PRIVATE_OPERATOR_ENVIRONMENT"
	if loadOperatorEnvironment() != nil {
		return denied
	}
	bootPhase = "POLICY_AND_HISTORICAL_ARTIFACTS"
	if os.Getenv("BUTLER_COMPARE_SUBSCRIPTION_NO_OVERAGE") != "1" || verifyHistory("/src") != nil {
		return denied
	}
	if mode != "inspect" && os.Getenv("BUTLER_COMPARE_APPROVED") != "1" {
		return denied
	}
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()
	bootPhase = "ENCRYPTED_POSTGRES_ACCESS"
	pool, err := pgxpool.New(ctx, os.Getenv("BUTLER_POSTGRES_URL"))
	if err != nil {
		return denied
	}
	defer pool.Close()
	if pool.Ping(ctx) != nil {
		return denied
	}
	manager := providerauth.NewManager(config.NewPostgresSettingsStore(pool))
	bootPhase = "PRIVATE_ORIGIN_AUDIT"
	reference, err := privateRead("/auth/import.json")
	if err != nil {
		return denied
	}
	defer clear(reference)
	origin, err := manager.AuditAntigravityOrigin(ctx, bytes.NewReader(reference))
	if err != nil || !origin.StoredCredentials || !origin.OAuthApplicationMatches || !origin.OAuthCredentialMatches || !origin.RefreshCredentialMatches {
		return denied
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"phase": "ORIGIN_MATCHED", "origin": origin, "real_model_calls": 0})
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	http.DefaultTransport = oauthAuditTransport{base: baseTransport}
	bootPhase = "PRIVATE_MANAGER_RESOLVE"
	auth, err := manager.ResolveAntigravity(ctx)
	http.DefaultTransport = baseTransport
	if err != nil {
		kind := "UNKNOWN"
		switch err.Error() {
		case "Antigravity control-plane request failed":
			kind = "CONNECTION"
		case "Antigravity refresh response is invalid", "Antigravity control-plane response is invalid":
			kind = "INVALID_RESPONSE"
		case "Antigravity encrypted credential persistence failed":
			kind = "PERSISTENCE"
		case "Antigravity refresh request failed":
			kind = "REQUEST_BUILD"
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"phase": "MANAGER_RESOLVE_FAILED", "fixed_error_class": kind, "real_model_calls": 0})
		return denied
	}
	if !auth.ExpiresAt.After(time.Now().Add(3 * time.Minute)) {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"phase": "ACCESS_VALIDITY_TOO_SHORT", "real_model_calls": 0})
		return denied
	}
	// Manager alone owns refresh and persists rotation in the existing encrypted PG.
	// The CLI gets that same short-lived token; it is not allowed to refresh itself.
	bootPhase = "PRIVATE_CLI_PREPARATION"
	loginData, err := privateRead("/auth/cli.json")
	if err != nil {
		return denied
	}
	defer clear(loginData)
	var login map[string]any
	if json.Unmarshal(loginData, &login) != nil {
		return denied
	}
	tokens, ok := login["token"].(map[string]any)
	if !ok || login["auth_method"] != "consumer" {
		return denied
	}
	tokens["access_token"], tokens["expiry"], tokens["token_type"] = auth.AccessToken, auth.ExpiresAt.Format(time.RFC3339), "Bearer"
	for _, dir := range []string{"home/.gemini/antigravity-cli", "work/.agents/agents", "cli-logs"} {
		if os.MkdirAll(filepath.Join(privateRoot, dir), 0700) != nil {
			return denied
		}
	}
	write := func(path string, data []byte) error {
		return os.WriteFile(filepath.Join(privateRoot, path), data, 0600)
	}
	b, _ := json.Marshal(login)
	if write("home/.gemini/antigravity-cli/antigravity-oauth-token", b) != nil {
		return denied
	}
	clear(b)
	if write("home/.gemini/antigravity-cli/settings.json", []byte(`{"useG1Credits":false,"enableTelemetry":false,"toolPermission":"strict","permissions":{"allow":[],"ask":[],"deny":["command(*)","unsandboxed(*)","read_file(*)","write_file(*)","read_url(*)","execute_url(*)","mcp(*)"]}}`)) != nil {
		return denied
	}
	agent := "---\nname: compare\ndescription: Bounded operator diagnostic\nmainAgent: true\nsubagent: false\ntools: []\nmcpServers: []\nskills: []\nplugins: []\ncommandExecutionPolicy: off\n---\n" + comparePrompt + "\n"
	if write("work/.agents/agents/compare.md", []byte(agent)) != nil {
		return denied
	}
	cert, certPEM := certificate()
	if write("ca.pem", certPEM) != nil {
		return denied
	}
	keyBytes := make([]byte, 32)
	if _, err = rand.Read(keyBytes); err != nil {
		return denied
	}
	key := hex.EncodeToString(keyBytes)
	clear(keyBytes)
	if write("control-key", []byte(key)) != nil {
		return denied
	}
	upstream := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone(), Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	bootPhase = "DIAGNOSTIC_LEDGER"
	ledgerPool, err := pgxpool.New(ctx, os.Getenv("BUTLER_COMPARE_POSTGRES_URL"))
	if err != nil {
		return denied
	}
	defer ledgerPool.Close()
	budget := &pgAdmission{pool: ledgerPool, root: "/src", native8192: mode == "native8192"}
	g := &gate{mode: mode, client: upstream, token: auth.AccessToken, budget: budget, onGeneration: make(chan struct{}, 1)}
	var metaMu sync.Mutex
	metadata := []map[string]any{}
	metadataCalls := 0
	avatarURL := ""
	appendMeta := func(host, path string, status int, data []byte) {
		metaMu.Lock()
		defer metaMu.Unlock()
		metadata = append(metadata, metadataProjection(host, path, status, data))
	}
	stage := func(name string) {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"phase": name, "real_model_calls": 0, "raw_diagnostics_suppressed": true})
	}
	stage("AUTH_ORIGIN_AND_ACCESS_READY")
	// Read-only preflight selects a display alias from the actual public model catalog.
	load := func(host, path string, body []byte) ([]byte, int, error) {
		metaCtx, stop := context.WithTimeout(ctx, 10*time.Second)
		defer stop()
		method := http.MethodPost
		if body == nil {
			method = http.MethodGet
		}
		req, _ := http.NewRequestWithContext(metaCtx, method, "https://"+host+path, bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+auth.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "antigravity/1.3.3 butler/0.1")
		resp, err := upstream.Do(req)
		if err != nil {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"phase": "METADATA_HTTP_FAILED", "host": host, "path": path, "http_status": 0, "connection_error_class": connectionClass(err), "real_model_calls": 0})
			return nil, 0, denied
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
		if err != nil || len(data) > 2*1024*1024 {
			return nil, resp.StatusCode, denied
		}
		appendMeta(host, path, resp.StatusCode, data)
		_ = json.NewEncoder(os.Stdout).Encode(metadataProjection(host, path, resp.StatusCode, data))
		return data, resp.StatusCode, nil
	}
	bootPhase = "ACCOUNT_METADATA"
	data, status, err := load("cloudcode-pa.googleapis.com", "/v1internal:loadCodeAssist", []byte(`{}`))
	if (err != nil || status != 200) && mode == "inspect" {
		return denied
	}
	stage("ACCOUNT_METADATA_OBSERVED")
	var account map[string]any
	_ = json.Unmarshal(data, &account)
	paid, ok := account["paidTier"].(map[string]any)
	proNow := ok && paid["id"] == "g1-pro-tier"
	if !proNow && mode == "inspect" {
		return denied
	}
	project, _ := account["cloudaicompanionProject"].(string)
	projectBody, _ := json.Marshal(map[string]string{"project": project})
	bootPhase = "ACTUAL_FLASH_CATALOG"
	data, status, err = load("cloudcode-pa.googleapis.com", "/v1internal:fetchAvailableModels", projectBody)
	if (err != nil || status != 200) && mode == "inspect" {
		return denied
	}
	stage("CATALOG_METADATA_OBSERVED")
	var catalog map[string]any
	_ = json.Unmarshal(data, &catalog)
	models, ok := catalog["models"].(map[string]any)
	if !ok && mode == "inspect" {
		return denied
	}
	target, ok := models[flash].(map[string]any)
	if !ok && mode == "inspect" {
		return denied
	}
	alias, _ := target["displayName"].(string)
	aliasFromMetadata := len(alias) > 0
	if alias == "" {
		alias = "gemini-3.8-flash-medium"
	}
	if len(alias) > 100 || strings.ContainsAny(alias, "\r\n\x00") {
		return denied
	}
	if write("model-arg", []byte(alias)) != nil || write("prompt", []byte(comparePrompt)) != nil {
		return denied
	}
	bootPhase = "NATIVE_MODEL_CONFIGURATION"
	data, status, err = load("aicode.googleapis.com", "/v1:fetchConfig", nil)
	if (err != nil || status != 200) && mode == "inspect" {
		return denied
	}
	stage("NATIVE_CONFIG_OBSERVED")
	_, _, _ = load("aicode.googleapis.com", "/v1:fetchLicenses", nil)
	// Start the immutable generation deadline after metadata preparation.
	// A restart still cannot initialize the same PostgreSQL ledger again.
	if budget.init(ctx) != nil {
		return denied
	}
	bootPhase = "GATE_LISTENERS"
	tlsListener, err := net.Listen("tcp", "127.0.0.1:3129")
	if err != nil {
		return denied
	}
	var metadataWG sync.WaitGroup
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, ErrorLog: log.New(io.Discard, "", 0), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, requestLimit+1))
		if err != nil || len(body) > requestLimit {
			http.Error(w, "request bound", 413)
			return
		}
		if strings.Contains(r.URL.Path, "GenerateContent") {
			g.generate(w, r, body)
			return
		}
		if stubbed(r.Host, r.URL.Path) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{}`)
			return
		}
		metaMu.Lock()
		isAvatar := r.Method == "GET" && avatarURL != "" && "https://"+r.Host+r.URL.RequestURI() == avatarURL
		metaMu.Unlock()
		if !isAvatar && !metadataAllowed(r.Host, r.URL.Path, r.Method) {
			path := "UNKNOWN"
			if regexp.MustCompile(`^/(v1internal|v1):[A-Za-z]{1,64}$`).MatchString(r.URL.Path) {
				path = r.URL.Path
			}
			metaMu.Lock()
			metadata = append(metadata, map[string]any{"host": r.Host, "path": path, "method": r.Method, "denied_before_upstream": true})
			metaMu.Unlock()
			http.Error(w, "metadata route denied", 403)
			return
		}
		metaMu.Lock()
		metadataCalls++
		count := metadataCalls
		metaMu.Unlock()
		if count > 30 {
			http.Error(w, "metadata bound", 429)
			return
		}
		metadataWG.Add(1)
		defer metadataWG.Done()
		request, _ := http.NewRequestWithContext(r.Context(), r.Method, "https://"+r.Host+r.URL.RequestURI(), bytes.NewReader(body))
		request.Header = r.Header.Clone()
		request.Header.Del("Content-Length")
		request.Header.Del("Accept-Encoding")
		if isAvatar {
			request.Header.Del("Authorization")
			request.Header.Del("Cookie")
		} else if r.Host != "antigravity-unleash.goog" {
			request.Header.Set("Authorization", "Bearer "+auth.AccessToken)
		}
		resp, err := upstream.Do(request)
		if err != nil {
			http.Error(w, "metadata unavailable", 502)
			return
		}
		defer resp.Body.Close()
		response, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
		if err != nil || len(response) > 2*1024*1024 {
			http.Error(w, "metadata bound", 502)
			return
		}
		if r.Host == "www.googleapis.com" && r.URL.Path == "/oauth2/v2/userinfo" && resp.StatusCode == 200 {
			var info map[string]any
			if json.Unmarshal(response, &info) == nil {
				picture, _ := info["picture"].(string)
				u, err := url.Parse(picture)
				if err == nil && u.Scheme == "https" && avatarHost(u.Host) && u.User == nil && u.Fragment == "" {
					metaMu.Lock()
					avatarURL = picture
					metaMu.Unlock()
				}
			}
		}
		if isAvatar {
			// Profile URL, image and identity never enter safe evidence.
			metaMu.Lock()
			metadata = append(metadata, map[string]any{"host": "PROFILE_AVATAR", "path": "PRIVATE_PROFILE_AVATAR", "http_status": resp.StatusCode, "bearer_forwarded": false, "observed_at": time.Now().UTC().Format(time.RFC3339Nano)})
			metaMu.Unlock()
		} else {
			appendMeta(r.Host, r.URL.Path, resp.StatusCode, response)
		}
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(response)
	})}
	go server.Serve(tls.NewListener(tlsListener, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}))
	defer server.Close()
	done := make(chan struct{}, 1)
	var goOnce sync.Once
	goOK := false
	goDiagnostic := "NOT_STARTED"
	report := func() map[string]any {
		base, _, _ := budget.profile()
		accountCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		var spent *int
		var count int
		if budget.pool.QueryRow(accountCtx, `SELECT spent FROM diagnostic_window WHERE id=1`).Scan(&count) == nil {
			spent = &count
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		metaMu.Lock()
		defer metaMu.Unlock()
		var total *int
		if spent != nil {
			n := base + *spent
			total = &n
		}
		return map[string]any{"mode": mode, "historical_attempts": base, "new_attempts": spent, "total_attempts": total, "accounting_available": spent != nil, "origin": origin, "google_ai_pro_current_preflight_confirmed": proNow, "google_ai_pro_prior_metadata_confirmed": true, "cli_model_arg": alias, "cli_alias_from_metadata": aliasFromMetadata, "cli_safe_diagnostics": cliDiagnosticsForMode(mode), "generation_seen": len(g.projections) > 0, "generation_projections": g.projections, "observations": g.results, "metadata": metadata, "native_marker_complete": goOK, "native_diagnostic_code": goDiagnostic, "supplemental_cost_policy": "subscription_no_overage", "cli_credit_use_disabled": true, "raw_diagnostics_suppressed": true}
	}
	proxy := &http.Server{ReadHeaderTimeout: 5 * time.Second, ErrorLog: log.New(io.Discard, "", 0), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			host, port, err := net.SplitHostPort(r.Host)
			if err != nil || port != "443" || !proxyHost(host) {
				host = strings.ToLower(strings.TrimSuffix(host, "."))
				known := "UNKNOWN"
				switch host {
				case "aiplatform.googleapis.com", "alkalimakersuiteapplets.pa.googleapis.com", "autopush-aicode.sandbox.googleapis.com", "autopush-cloudcode-pa.sandbox.googleapis.com", "generativelanguage.googleapis.com", "iamcredentials.googleapis.com", "oauth2.googleapis.com", "oauth2.mtls.googleapis.com", "preprod-daily-cloudcode-pa.sandbox.googleapis.com", "safebrowsing.googleapis.com", "sts.googleapis.com", "www.mtls.googleapis.com", "antigravity.google", "antigravity.google.com", "antigravity.goog", "auth.cloud.google", "accounts.google.com", "www.gstatic.com", "csp.withgoogle.com", "check-proxy-existence.com", "api.github.com", "github.com", "raw.githubusercontent.com":
					known = host
				}
				ip := net.ParseIP(host)
				local := host == "localhost" || (ip != nil && (ip.IsLoopback() || ip.IsUnspecified()))
				if known == "UNKNOWN" && regexp.MustCompile(`^[a-z][a-z0-9.-]{1,120}[.](com|app|dev|goog|google|net|io|org|co)$`).MatchString(host) {
					// A host may be projected only if it is an immutable public CLI literal,
					// never an account-derived URL or an arbitrary request value.
					bin, err := os.ReadFile("/installed-cli")
					if err == nil && bytes.Contains(bin, []byte(host)) {
						known = host
					}
					clear(bin)
				}
				googleDomain := "UNKNOWN"
				for _, suffix := range []string{"google.com", "googleapis.com", "googleusercontent.com", "gstatic.com", "cloud.google", "antigravity.google", "withgoogle.com"} {
					if host == suffix || strings.HasSuffix(host, "."+suffix) {
						googleDomain = suffix
					}
				}
				_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"phase": "CONNECT_DENIED", "known_service_host": known, "google_domain": googleDomain, "local_address": local, "private_ip": ip != nil && ip.IsPrivate(), "single_label_host": !strings.Contains(host, "."), "port_443": port == "443", "generation_dispatched": false})
				http.Error(w, "host denied", 403)
				return
			}
			up, err := net.DialTimeout("tcp", "127.0.0.1:3129", time.Second)
			if err != nil {
				http.Error(w, "fixture unavailable", 503)
				return
			}
			conn, buffer, err := w.(http.Hijacker).Hijack()
			if err != nil {
				up.Close()
				return
			}
			_, _ = buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
			_ = buffer.Flush()
			go func() { defer conn.Close(); defer up.Close(); go io.Copy(up, conn); _, _ = io.Copy(conn, up) }()
			return
		}
		if r.Method != "POST" || r.Header.Get("X-Diagnostic-Key") != key {
			http.Error(w, "control denied", 403)
			return
		}
		switch r.URL.Path {
		case "/ready", "/report":
			_ = json.NewEncoder(w).Encode(report())
		case "/run-go":
			if mode != "execute" && mode != "native8192" {
				http.Error(w, "execution disabled", 403)
				return
			}
			goOnce.Do(func() {
				g.mu.Lock()
				goDiagnostic = "STARTED"
				g.mu.Unlock()
				roots := x509.NewCertPool()
				roots.AppendCertsFromPEM(certPEM)
				localProxy, _ := url.Parse("http://127.0.0.1:3128")
				http.DefaultTransport = &http.Transport{Proxy: http.ProxyURL(localProxy), TLSClientConfig: &tls.Config{RootCAs: roots}}
				p, err := antigravity.NewProvider(antigravity.Config{AuthSource: fixedAuth{auth}, Timeout: 45 * time.Second})
				if err != nil {
					return
				}
				runCtx, stop := context.WithTimeout(transport.WithInputByteLimit(ctx, requestLimit), 45*time.Second)
				defer stop()
				stream, err := p.StartRun(runCtx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "diagnostic-go", SessionKey: "diagnostic", ProviderName: antigravity.Name, ModelName: flash}, InputItems: []transport.InputItem{{Role: "user", Content: comparePrompt}}, StreamingEnabled: true})
				if err != nil {
					g.mu.Lock()
					goDiagnostic = transport.DiagnosticCode(err)
					g.mu.Unlock()
					return
				}
				for event := range stream {
					if event.TransportError != nil {
						g.mu.Lock()
						goDiagnostic = transport.DiagnosticCode(event.TransportError)
						g.mu.Unlock()
					}
					if event.AssistantFinal != nil {
						g.mu.Lock()
						goOK = event.AssistantFinal.Content == marker && event.AssistantFinal.FinishReason == "STOP"
						goDiagnostic = "ASSISTANT_FINAL"
						g.mu.Unlock()
					}
				}
			})
			_ = json.NewEncoder(w).Encode(report())
		case "/post-metadata":
			_, _, _ = load("cloudcode-pa.googleapis.com", "/v1internal:fetchAvailableModels", projectBody)
			_ = json.NewEncoder(w).Encode(report())
		case "/shutdown":
			_ = json.NewEncoder(w).Encode(report())
			select {
			case done <- struct{}{}:
			default:
			}
		default:
			http.Error(w, "control denied", 403)
		}
	})}
	listener, err := net.Listen("tcp", "0.0.0.0:3128")
	if err != nil {
		return denied
	}
	go proxy.Serve(listener)
	defer proxy.Close()
	bootPhase = "GATE_READY"
	select {
	case <-done:
	case <-ctx.Done():
	}
	return nil
}

type fixedAuth struct{ auth providerauth.AntigravityAuth }

type oauthAuditTransport struct{ base http.RoundTripper }

func (o oauthAuditTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	proxyUsed := false
	if tr, ok := o.base.(*http.Transport); ok && tr.Proxy != nil {
		p, _ := tr.Proxy(req)
		proxyUsed = p != nil
	}
	var traceMu sync.Mutex
	traceFlags := map[string]bool{}
	setFlag := func(key string, value bool) { traceMu.Lock(); traceFlags[key] = value; traceMu.Unlock() }
	trace := &httptrace.ClientTrace{GotConn: func(httptrace.GotConnInfo) { setFlag("connection_obtained", true) }, TLSHandshakeDone: func(tls.ConnectionState, error) { setFlag("tls_handshake_callback", true) }, WroteRequest: func(info httptrace.WroteRequestInfo) { setFlag("request_written", info.Err == nil) }, GotFirstResponseByte: func() { setFlag("first_response_byte", true) }}
	resp, err := o.base.RoundTrip(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
	if req.URL.Host == "oauth2.googleapis.com" && req.URL.Path == "/token" {
		status := 0
		reason := "UNKNOWN"
		if resp != nil {
			status = resp.StatusCode
		}
		if resp != nil && status != 200 {
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, 65537))
			if readErr == nil && len(data) <= 65536 {
				var value struct {
					Error string `json:"error"`
				}
				if json.Unmarshal(data, &value) == nil {
					switch value.Error {
					case "invalid_grant", "invalid_client", "unauthorized_client", "access_denied", "unsupported_grant_type", "temporarily_unavailable":
						reason = strings.ToUpper(value.Error)
					}
				}
			}
			resp.Body = &replayBody{Reader: io.MultiReader(bytes.NewReader(data), resp.Body), Closer: resp.Body}
		}
		traceMu.Lock()
		defer traceMu.Unlock()
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"phase": "OAUTH_REFRESH_HTTP", "http_status": status, "oauth_error_enum": reason, "proxy_selected": proxyUsed, "connection_failed": err != nil, "connection_error_class": connectionClass(err), "safe_trace_flags": traceFlags, "real_model_calls": 0})
	}
	return resp, err
}

type replayBody struct {
	io.Reader
	io.Closer
}

func (f fixedAuth) ResolveAntigravity(context.Context) (providerauth.AntigravityAuth, error) {
	return f.auth, nil
}

func proxyHost(h string) bool {
	if avatarHost(h) {
		return true
	}
	switch h {
	case "aicode.googleapis.com", "cloudcode-pa.googleapis.com", "daily-cloudcode-pa.googleapis.com", "www.googleapis.com", "antigravity-unleash.goog", "play.googleapis.com", "antigravity-cli-auto-updater-974169037036.us-central1.run.app":
		return true
	}
	return false
}
func avatarHost(h string) bool {
	return regexp.MustCompile(`^lh[0-9]{1,2}[.]googleusercontent[.]com$`).MatchString(h)
}
func stubbed(host, path string) bool {
	return (host == "antigravity-unleash.goog" && path != "/api/client/features") || host == "play.googleapis.com" || host == "antigravity-cli-auto-updater-974169037036.us-central1.run.app" || strings.HasSuffix(path, ":writeTrajectoryAcls")
}
func metadataAllowed(host, path, method string) bool {
	if host == "antigravity-unleash.goog" && path == "/api/client/features" && method == "GET" {
		return true
	}
	if host == "aicode.googleapis.com" && method == "GET" {
		return path == "/v1:fetchConfig" || path == "/v1:fetchLicenses"
	}
	if host == "www.googleapis.com" && method == "GET" {
		return path == "/oauth2/v2/userinfo"
	}
	if (host == "cloudcode-pa.googleapis.com" || host == "daily-cloudcode-pa.googleapis.com") && method == "POST" {
		switch path {
		case "/v1internal:loadCodeAssist", "/v1internal:fetchAvailableModels", "/v1internal:retrieveUserQuotaSummary", "/v1internal:retrieveUserQuota", "/v1internal:fetchUserInfo", "/v1internal:listExperiments", "/v1internal:fetchAdminControls":
			return true
		}
	}
	return false
}
func metadataProjection(host, path string, status int, data []byte) map[string]any {
	p := map[string]any{"host": host, "path": path, "http_status": status, "observed_at": time.Now().UTC().Format(time.RFC3339Nano)}
	p["json_valid"] = json.Valid(data)
	p["received_gzip_bytes"] = len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
	var m map[string]any
	if json.Unmarshal(data, &m) != nil {
		return p
	}
	if tier, ok := m["paidTier"].(map[string]any); ok {
		p["google_ai_pro"] = tier["id"] == "g1-pro-tier"
	}
	if models, ok := m["models"].(map[string]any); ok {
		if target, ok := models[flash].(map[string]any); ok {
			p["flash_advertised"] = true
			if quota, ok := target["quotaInfo"].(map[string]any); ok {
				if f, ok := quota["remainingFraction"].(float64); ok && f >= 0 && f <= 1 {
					p["flash_remaining_fraction"] = f
				}
			}
		}
	}
	// Only structural key names for license/config diagnosis, never account values.
	keys := []string{}
	for _, key := range []string{"licenses", "availableModels", "creditConfig", "creditOverages", "useG1Credits", "paidTier", "currentTier", "credits", "creditBalance", "settings", "error", "response", "data", "modelInfos", "modelConfig", "project", "loadCodeAssistResponse"} {
		if _, exists := m[key]; exists {
			keys = append(keys, key)
		}
	}
	p["recognized_keys"] = keys
	return p
}

func loadOperatorEnvironment() error {
	data, err := privateRead(filepath.Join(privateRoot, "control.env"))
	if err != nil {
		return denied
	}
	defer clear(data)
	allowed := map[string]bool{"BUTLER_POSTGRES_URL": true, "BUTLER_SETTINGS_ENCRYPTION_KEY": true, "HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true, "BUTLER_COMPARE_SUBSCRIPTION_NO_OVERAGE": true}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || !allowed[key] || seen[key] || strings.ContainsAny(value, "\r\x00") {
			return denied
		}
		seen[key] = true
		if os.Setenv(key, value) != nil {
			return denied
		}
	}
	for key := range allowed {
		if !seen[key] {
			return denied
		}
	}
	return nil
}

func cliDiagnostics() map[string]any {
	b, err := os.ReadFile(filepath.Join(privateRoot, "cli-logs/output"))
	if err != nil || len(b) > 1024*1024 {
		return map[string]any{"available": false}
	}
	defer clear(b)
	s := string(b)
	lower := strings.ToLower(s)
	ids := regexp.MustCompile(`gemini-[a-z0-9.-]{1,80}|MODEL_PLACEHOLDER_M[0-9]{1,5}`).FindAllString(s, 80)
	models := []string{}
	seen := map[string]bool{}
	for _, id := range ids {
		if !seen[id] {
			models = append(models, id)
			seen[id] = true
		}
	}
	flags := []string{}
	for _, word := range []string{"unknown model", "invalid model", "unsupported model", "not found", "authentication", "expired", "certificate", "timeout", "error", "login", "models", "fetch", "json", "unmarshal", "decode", "parse", "project", "initialize", "configuration", "permission", "onboard", "list", "available", "unknown", "unsupported", "invalid", "missing", "failed", "command", "flag", "account", "subscription", "tier", "token", "keyring", "profile", "consumer", "connect", "context", "deadline", "eof", "enabled", "empty", "network", "backend", "no models", "unable", "cannot", "save", "write", "open", "mkdir", "file", "no such", "read-only", "register", "403", "401", "500", "refresh", "id token", "id_token", "retrieve", "malformed", "oauth", "scope", "storage", "access token", "permission denied", "failed to load", "failed to get", "failed to fetch", "failed to save", "failed to write", "failed to create", "failed to read", "failed to generate", "failed to acquire", "failed to lock"} {
		if strings.Contains(lower, word) {
			flags = append(flags, word)
		}
	}
	return map[string]any{"available": true, "public_model_ids": models, "fixed_flags": flags, "output_bytes": len(b)}
}

func cliDiagnosticsForMode(mode string) map[string]any {
	if mode == "native8192" {
		return map[string]any{"available": false, "cli_launched": false, "cached_output_ignored": true}
	}
	return cliDiagnostics()
}
func certificate() (tls.Certificate, []byte) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic("certificate unavailable")
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Private diagnostic gate"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), DNSNames: []string{"*.googleapis.com", "*.googleusercontent.com", "antigravity-unleash.goog", "antigravity-cli-auto-updater-974169037036.us-central1.run.app"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		panic("certificate unavailable")
	}
	raw, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		panic("certificate unavailable")
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	clear(raw)
	clear(keyPEM)
	if err != nil {
		panic("certificate unavailable")
	}
	return cert, certPEM
}
