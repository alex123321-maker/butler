package main

// Read-only metadata gateway. It never forwards a generation, even an unknown route.
import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/jackc/pgx/v5/pgxpool"
)

func desktopMetadataAllowed(host, path, method string) bool {
	if host == "antigravity-unleash.goog" {
		return path == "/api/client/features" && method == "GET"
	}
	if host == "www.googleapis.com" {
		return method == "GET" && (path == "/oauth2/v2/userinfo" || path == "/oauth2/v3/tokeninfo")
	}
	if host != "daily-cloudcode-pa.googleapis.com" && host != "cloudcode-pa.googleapis.com" {
		return false
	}
	if method == "GET" {
		return path == "/v1internal/buildWithGooglePlugins" || path == "/v1internal/cascadeNuxes"
	}
	if method != "POST" {
		return false
	}
	switch path {
	case "/v1internal:loadCodeAssist", "/v1internal:fetchAvailableModels", "/v1internal:fetchUserInfo", "/v1internal:listExperiments", "/v1internal:fetchAdminControls", "/v1internal:retrieveUserQuotaSummary":
		return true
	}
	return false
}

type metadataGateway struct {
	mu        sync.Mutex
	requests  []map[string]any
	client    *http.Client
	token     string
	calls     int
	deadline  time.Time
	avatarURL string
}

func (g *metadataGateway) report() map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	return map[string]any{"requests": g.requests, "mode": "metadata-only", "generation_forwarding_enabled": false, "synthetic_credentials_only": false, "real_model_calls": 0, "origin_audit_matched": true}
}
func (g *metadataGateway) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 65537))
	if err != nil || len(body) > 65536 {
		http.Error(w, "bound", 413)
		return
	}
	entry := projection(r, body)
	entry["bearer_is_synthetic"] = false
	entry["bearer_matches_audited_access"] = r.Header.Get("Authorization") == "Bearer "+g.token
	// Public path literals below come from the installed Desktop. No arbitrary path is exported.
	switch r.URL.Path {
	case "/v1internal:tabChat", "/v1internal/cascadeNuxes", "/v1internal/buildWithGooglePlugins", "/oauth2/v3/tokeninfo":
		entry["path"] = r.URL.Path
	}
	g.mu.Lock()
	isAvatar := r.Method == "GET" && g.avatarURL != "" && "https://"+r.Host+r.URL.RequestURI() == g.avatarURL
	allowed := desktopMetadataAllowed(r.Host, r.URL.Path, r.Method) || isAvatar
	g.calls++
	bounded := g.calls <= 40 && time.Now().Before(g.deadline)
	g.mu.Unlock()
	if !allowed || !bounded {
		entry["denied_before_upstream"] = true
		g.mu.Lock()
		g.requests = append(g.requests, entry)
		g.mu.Unlock()
		http.Error(w, "metadata-only gateway", 403)
		return
	}
	if !isAvatar && r.Host != "antigravity-unleash.goog" && r.Header.Get("Authorization") != "Bearer "+g.token && r.URL.Query().Get("access_token") != g.token {
		entry["denied_before_upstream"] = true
		entry["credential_mismatch"] = true
		g.mu.Lock()
		g.requests = append(g.requests, entry)
		g.mu.Unlock()
		http.Error(w, "original credential required", 401)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, r.Method, "https://"+r.Host+r.URL.RequestURI(), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "request", 502)
		return
	}
	request.Header = r.Header.Clone()
	request.Header.Del("Accept-Encoding")
	request.Header.Del("Content-Length")
	request.Header.Del("Cookie")
	if isAvatar {
		request.Header.Del("Authorization")
	} else if r.Host != "antigravity-unleash.goog" {
		request.Header.Set("Authorization", "Bearer "+g.token)
	}
	response, err := g.client.Do(request)
	if err != nil {
		entry["http_status"] = 0
		g.mu.Lock()
		g.requests = append(g.requests, entry)
		g.mu.Unlock()
		http.Error(w, "metadata unavailable", 502)
		return
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024+1))
	if err != nil || len(data) > 2*1024*1024 {
		http.Error(w, "metadata response bound", 502)
		return
	}
	entry["http_status"] = response.StatusCode
	g.mu.Lock()
	if r.Host == "www.googleapis.com" && r.URL.Path == "/oauth2/v2/userinfo" && response.StatusCode == 200 {
		var user map[string]any
		if json.Unmarshal(data, &user) == nil {
			if picture, ok := user["picture"].(string); ok {
				if u, e := url.Parse(picture); e == nil && u.Scheme == "https" && u.User == nil && u.Port() == "" && regexp.MustCompile(`^lh[0-9]{1,2}[.]googleusercontent[.]com$`).MatchString(u.Hostname()) {
					g.avatarURL = picture
				}
			}
		}
	}
	g.requests = append(g.requests, entry)
	g.mu.Unlock()
	w.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(data)
}

func metadataRead(path string) ([]byte, error) {
	s, e := os.Lstat(path)
	if e != nil || !s.Mode().IsRegular() || s.Mode().Perm() != 0600 || s.Size() > 65536 {
		return nil, errors.New("private reference unavailable")
	}
	return os.ReadFile(path)
}
func runMetadataGateway() error {
	data, err := metadataRead("/operator/control.env")
	if err != nil {
		return err
	}
	defer clear(data)
	allowed := map[string]bool{"BUTLER_POSTGRES_URL": true, "BUTLER_SETTINGS_ENCRYPTION_KEY": true, "HTTP_PROXY": true, "HTTPS_PROXY": true, "NO_PROXY": true, "BUTLER_COMPARE_SUBSCRIPTION_NO_OVERAGE": true}
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok || !allowed[key] || seen[key] || strings.ContainsAny(value, "\r\x00") {
			return errors.New("private environment rejected")
		}
		seen[key] = true
		if os.Setenv(key, value) != nil {
			return errors.New("private environment unavailable")
		}
	}
	if len(seen) != len(allowed) || os.Getenv("BUTLER_COMPARE_SUBSCRIPTION_NO_OVERAGE") != "1" {
		return errors.New("credit policy unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, os.Getenv("BUTLER_POSTGRES_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	manager := providerauth.NewManager(config.NewPostgresSettingsStore(pool))
	reference, err := metadataRead("/auth/import.json")
	if err != nil {
		return err
	}
	defer clear(reference)
	audit, err := manager.AuditAntigravityOrigin(ctx, bytes.NewReader(reference))
	if err != nil || !audit.StoredCredentials || !audit.OAuthApplicationMatches || !audit.OAuthCredentialMatches || !audit.RefreshCredentialMatches {
		return errors.New("origin mismatch")
	}
	auth, err := manager.ResolveAntigravity(ctx)
	if err != nil || !auth.ExpiresAt.After(time.Now().Add(5*time.Minute)) {
		return errors.New("access unavailable")
	}
	loginData, err := metadataRead("/auth/cli.json")
	if err != nil {
		return err
	}
	defer clear(loginData)
	var login map[string]any
	if json.Unmarshal(loginData, &login) != nil {
		return errors.New("profile invalid")
	}
	token, ok := login["token"].(map[string]any)
	if !ok || login["auth_method"] != "consumer" {
		return errors.New("profile invalid")
	}
	token["access_token"], token["expiry"], token["token_type"] = auth.AccessToken, auth.ExpiresAt.Format(time.RFC3339), "Bearer"
	b, err := json.Marshal(login)
	if err != nil {
		return err
	}
	defer clear(b)
	home := "/private-desktop/home"
	for _, dir := range []string{filepath.Join(home, ".gemini", "antigravity"), filepath.Join(home, ".gemini"), filepath.Join(home, ".config", "antigravity"), filepath.Join(home, ".gemini", "config")} {
		if os.MkdirAll(dir, 0700) != nil || os.WriteFile(filepath.Join(dir, "jetski-standalone-oauth-token"), b, 0600) != nil {
			return errors.New("private profile unavailable")
		}
	}
	if os.WriteFile(filepath.Join(home, ".gemini", "config", "config.json"), []byte(`{"userSettings":{"useAICredits":false}}`), 0600) != nil {
		return errors.New("private settings unavailable")
	}
	public, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now()
	template := x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Private Desktop metadata"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), DNSNames: []string{"*.googleapis.com", "*.googleusercontent.com", "antigravity-unleash.goog"}, IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, public, key)
	if err != nil {
		return err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	defer clear(keyDER)
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	cert, err := tls.X509KeyPair(ca, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		return err
	}
	if os.WriteFile("/private-desktop/ca.pem", ca, 0600) != nil {
		return errors.New("private CA unavailable")
	}
	g := &metadataGateway{token: auth.AccessToken, client: &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone(), Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, deadline: time.Now().Add(3 * time.Minute)}
	listener, err := net.Listen("tcp", "127.0.0.1:3129")
	if err != nil {
		return err
	}
	defer listener.Close()
	server := http.Server{Handler: http.HandlerFunc(g.serve), ReadHeaderTimeout: 5 * time.Second, ErrorLog: log.New(io.Discard, "", 0)}
	go server.Serve(tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}))
	defer server.Close()
	proxy := http.Server{ReadHeaderTimeout: 5 * time.Second, ErrorLog: log.New(io.Discard, "", 0), Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/report" {
			_ = json.NewEncoder(w).Encode(g.report())
			return
		}
		host, port, e := net.SplitHostPort(r.Host)
		avatarHost := regexp.MustCompile(`^lh[0-9]{1,2}[.]googleusercontent[.]com$`).MatchString(host)
		if r.Method != "CONNECT" || e != nil || port != "443" || (!knownHost(host) && !avatarHost) {
			label := "UNKNOWN"
			if knownHost(host) {
				label = host
			}
			g.mu.Lock()
			g.requests = append(g.requests, map[string]any{"host": label, "path": "CONNECT_DENIED", "denied_before_upstream": true, "real_model_calls": 0})
			g.mu.Unlock()
			http.Error(w, "proxy denied", 403)
			return
		}
		up, e := net.DialTimeout("tcp", "127.0.0.1:3129", time.Second)
		if e != nil {
			http.Error(w, "proxy unavailable", 503)
			return
		}
		conn, buf, e := w.(http.Hijacker).Hijack()
		if e != nil {
			up.Close()
			return
		}
		_, _ = buf.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buf.Flush()
		go func() { defer conn.Close(); defer up.Close(); go io.Copy(up, conn); _, _ = io.Copy(conn, up) }()
	})}
	outer, err := net.Listen("tcp", ":3128")
	if err != nil {
		return err
	}
	defer outer.Close()
	go proxy.Serve(outer)
	defer proxy.Close()
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"metadata_gateway_ready": true, "origin": audit, "real_model_calls": 0, "generation_forwarding_enabled": false})
	<-ctx.Done()
	_ = json.NewEncoder(os.Stdout).Encode(g.report())
	return nil
}
