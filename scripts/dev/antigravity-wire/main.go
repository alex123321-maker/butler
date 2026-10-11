// Command antigravity-wire captures the installed CLI against synthetic HTTPS.
// It refuses any network interface except loopback. No operator profile is read.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

const prompt = "Return only OFFLINE_OK. Do not use tools."

func main() {
	if len(os.Args) != 2 || os.Getenv("BUTLER_ANTIGRAVITY_OFFLINE") != "1" {
		panic("isolated offline invocation required")
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		panic("interface check failed")
	}
	if !loopbackOnly(ifaces) {
		panic("non-loopback interface denied")
	}
	binary := os.Args[1]
	b, err := os.ReadFile(binary)
	if err != nil {
		panic("CLI binary read failed")
	}
	hash := sha256.Sum256(b)
	clear(b)
	dir, err := os.MkdirTemp("", "antigravity-offline-")
	if err != nil {
		panic("fixture directory failed")
	}
	defer os.RemoveAll(dir)
	home := filepath.Join(dir, "home")
	profile := filepath.Join(home, ".gemini", "antigravity-cli")
	work := filepath.Join(dir, "work")
	for _, p := range []string{profile, filepath.Join(work, ".agents", "agents")} {
		if os.MkdirAll(p, 0700) != nil {
			panic("fixture mkdir failed")
		}
	}
	write := func(p, data string) {
		if os.WriteFile(p, []byte(data), 0600) != nil {
			panic("fixture write failed")
		}
	}
	idToken := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"email":"fixture@example.invalid","sub":"synthetic-user","exp":4070908800}`)) + ".fixture"
	fakeLogin, _ := json.Marshal(map[string]any{"token": map[string]any{"access_token": "synthetic-access", "token_type": "Bearer", "refresh_token": "synthetic-refresh", "expiry": "2099-01-01T00:00:00Z"}, "auth_method": "consumer", "id_token": idToken})
	write(filepath.Join(profile, "antigravity-oauth-token"), string(fakeLogin))
	write(filepath.Join(profile, "settings.json"), `{"useG1Credits":false,"enableTelemetry":false,"toolPermission":"strict","permissions":{"allow":[],"ask":[],"deny":["command(*)","unsandboxed(*)","read_file(*)","write_file(*)","read_url(*)","execute_url(*)","mcp(*)"]}}`)
	write(filepath.Join(work, ".agents", "agents", "offline-wire.md"), "---\nname: offline-wire\ndescription: Synthetic wire fixture\nmainAgent: true\nsubagent: false\ntools: []\nmcpServers: []\nskills: []\nplugins: []\ncommandExecutionPolicy: off\n---\nReturn only OFFLINE_OK. Do not use tools.\n")
	cert, certPEM := fixtureCert()
	certPath := filepath.Join(dir, "ca.pem")
	write(certPath, string(certPEM))
	var mu sync.Mutex
	var observations []map[string]any
	var generationCalls int
	phase := "cli"
	captured := make(chan struct{}, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 8*1024*1024+1))
		var value map[string]any
		_ = json.Unmarshal(body, &value)
		keys := make([]string, 0, len(value))
		for k := range value {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		entry := map[string]any{"method": r.Method, "host": r.Host, "path": r.URL.Path, "alt": r.URL.Query().Get("alt"), "accept": r.Header.Get("Accept"), "keys": keys, "content_type": r.Header.Get("Content-Type"), "user_agent": r.Header.Get("User-Agent"), "authorization_is_fixture": r.Header.Get("Authorization") == "Bearer synthetic-access"}
		mu.Lock()
		currentPhase := phase
		mu.Unlock()
		entry["client"] = currentPhase
		isGeneration := strings.Contains(r.URL.Path, "GenerateContent")
		if isGeneration {
			entry["generation"] = summarize(value)
			if currentPhase == "cli" {
				mu.Lock()
				generationCalls++
				mu.Unlock()
			}
		}
		mu.Lock()
		observations = append(observations, entry)
		mu.Unlock()
		if isGeneration && currentPhase == "cli" {
			select {
			case captured <- struct{}{}:
			default:
			}
			// Capture is the gate; no CLI semantic-success or live API claim follows.
			time.Sleep(250 * time.Millisecond)
			http.Error(w, "offline capture complete", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "loadCodeAssist"):
			io.WriteString(w, `{"cloudaicompanionProject":"synthetic-project","currentTier":{"id":"free-tier","name":"Antigravity"},"paidTier":{"id":"g1-pro-tier","name":"Google AI Pro"}}`)
		case strings.Contains(r.URL.Path, "fetchAvailableModels"):
			io.WriteString(w, `{"models":{"gemini-3.8-flash-tiered":{"displayName":"Gemini 3.8 Flash","model":"MODEL_PLACEHOLDER_M322","apiProvider":"API_PROVIDER_GOOGLE_GEMINI","supportsThinking":true,"recommended":true,"maxTokens":1048576,"maxOutputTokens":65536,"quotaInfo":{"remainingFraction":1}}},"defaultAgentModelId":"gemini-3.8-flash-tiered","agentModelSorts":[{"displayName":"Models","groups":[{"displayName":"Gemini","modelIds":["gemini-3.8-flash-tiered"]}]}],"tieredModelIds":{"flash":["gemini-3.8-flash-tiered"]}}`)
		case strings.Contains(r.URL.Path, "fetchConfig"):
			io.WriteString(w, `{}`)
		case strings.Contains(r.URL.Path, "fetchLicenses"):
			io.WriteString(w, `{}`)
		case strings.Contains(r.URL.Path, "userinfo"):
			io.WriteString(w, `{"email":"fixture@example.invalid","id":"synthetic-user","name":"Synthetic fixture"}`)
		case strings.Contains(r.URL.Path, "streamGenerateContent"):
			chunk := `{"candidates":[{"index":0,"content":{"role":"model","parts":[{"text":"OFFLINE_OK"}]},"finishReason":"STOP"}],"modelVersion":"gemini-3.8-flash","usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`
			if strings.Contains(r.Host, "cloudcode-pa") {
				chunk = `{"response":` + chunk + `}`
			}
			if r.URL.Query().Get("alt") == "sse" {
				w.Header().Set("Content-Type", "text/event-stream")
				io.WriteString(w, "data: "+chunk+"\n\n")
			} else {
				io.WriteString(w, "["+chunk+"]")
			}
		default:
			io.WriteString(w, `{}`)
		}
	}))
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	defer server.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "fixture requires CONNECT", 400)
			return
		}
		host, port, err := net.SplitHostPort(r.Host)
		if err != nil || port != "443" || !allowedHost(host) {
			safeHost := "unrecognized"
			if regexp.MustCompile(`^[a-z0-9.-]{1,100}:(443|80)$`).MatchString(r.Host) {
				safeHost = r.Host
			}
			mu.Lock()
			observations = append(observations, map[string]any{"denied_connect": safeHost})
			mu.Unlock()
		}
		if err != nil || port != "443" || !allowedHost(host) {
			http.Error(w, "fixture host denied", 403)
			return
		}
		upstream, err := net.DialTimeout("tcp", server.Listener.Addr().String(), time.Second)
		if err != nil {
			http.Error(w, "fixture unavailable", 503)
			return
		}
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			upstream.Close()
			return
		}
		fmt.Fprint(buf, "HTTP/1.1 200 Connection Established\r\n\r\n")
		buf.Flush()
		go func() {
			defer conn.Close()
			defer upstream.Close()
			go io.Copy(upstream, conn)
			io.Copy(conn, upstream)
		}()
	}))
	defer proxy.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--agent", "offline-wire", "--model", "Gemini 3.8 Flash", "--print", prompt, "--print-timeout", "20s", "--output-format", "json", "--disable-slash-commands")
	cmd.Dir = work
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + home, "XDG_CONFIG_HOME=" + home, "TERM=dumb", "HTTPS_PROXY=" + proxy.URL, "HTTP_PROXY=" + proxy.URL, "SSL_CERT_FILE=" + certPath, "SSL_CERT_DIR=" + dir, "ANTIGRAVITY_APP_DATA_DIR=" + profile}
	if os.Getenv("BUTLER_WIRE_MODE") == "aicode" {
		cmd.Env = append(cmd.Env, "USE_AICODE=true")
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err = cmd.Start()
	if err != nil {
		panic("isolated CLI start failed")
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	captureComplete := false
	select {
	case <-captured:
		captureComplete = true
		cancel()
		err = <-done
	case err = <-done:
	case <-ctx.Done():
		err = <-done
	}
	mu.Lock()
	phase = "butler"
	mu.Unlock()
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)
	proxyURL, _ := url.Parse(proxy.URL)
	http.DefaultTransport = &http.Transport{Proxy: http.ProxyURL(proxyURL), TLSClientConfig: &tls.Config{RootCAs: pool}}
	p, providerErr := antigravity.NewProvider(antigravity.Config{AuthSource: fixtureAuth{}, Timeout: 5 * time.Second})
	if providerErr != nil {
		panic("fixture provider failed")
	}
	goCtx, goCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer goCancel()
	stream, providerErr := p.StartRun(goCtx, transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "offline-wire", SessionKey: "offline-wire", ProviderName: antigravity.Name, ModelName: antigravity.DefaultModel}, InputItems: []transport.InputItem{{Role: "system", Content: "Return only OFFLINE_OK. Do not use tools."}, {Role: "user", Content: prompt}}, StreamingEnabled: true})
	butlerOK := false
	if providerErr == nil {
		for event := range stream {
			if event.AssistantFinal != nil {
				butlerOK = event.AssistantFinal.Content == "OFFLINE_OK"
			}
			if event.TransportError != nil {
				butlerOK = false
			}
		}
	}
	flags := []string{}
	for _, word := range []string{"login", "auth", "model", "quota", "error", "OFFLINE_OK", "certificate", "timeout", "project", "permission", "invalid", "missing", "unsupported", "marshal", "decode", "json", "not found", "keyring", "expiry", "email", "tier", "userinfo", "entitlement", "config", "cannot", "fetch", "agent", "parse", "signature", "verified", "failed", "home", "settings", "provider", "user", "token", "credential", "secret", "cookie", "profile", "enum", "unknown", "empty", "field", "unmarshal", "M322", "does not exist"} {
		if strings.Contains(strings.ToLower(output.String()), strings.ToLower(word)) {
			flags = append(flags, word)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"cli_sha256": hex.EncodeToString(hash[:]), "cli_capture_complete": captureComplete, "cli_exit_ok": err == nil, "butler_fixture_complete": butlerOK, "diagnostic_flags": flags, "observations": observations, "cli_generation_attempts": generationCalls, "real_model_calls": 0})
	if !captureComplete || !butlerOK || generationCalls != 1 {
		os.Exit(1)
	}
}

type fixtureAuth struct{}

func (fixtureAuth) ResolveAntigravity(context.Context) (providerauth.AntigravityAuth, error) {
	return providerauth.AntigravityAuth{AccessToken: "synthetic-access", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func loopbackOnly(ifaces []net.Interface) bool {
	if len(ifaces) == 0 {
		return false
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback == 0 {
			return false
		}
	}
	return true
}

func allowedHost(host string) bool {
	switch host {
	case "aicode.googleapis.com", "cloudcode-pa.googleapis.com", "daily-cloudcode-pa.googleapis.com", "play.googleapis.com", "antigravity-unleash.goog", "oauth2.googleapis.com", "www.googleapis.com", "antigravity-cli-auto-updater-974169037036.us-central1.run.app":
		return true
	}
	return false
}

func summarize(value map[string]any) map[string]any {
	out := map[string]any{}
	for _, k := range []string{"aicode", "generationConfig", "model", "sessionId", "entitlement", "request"} {
		if v, exists := value[k]; exists {
			out[k+"_present"] = true
			switch k {
			case "model":
				if s, ok := v.(string); ok && strings.HasPrefix(s, "gemini-") {
					out[k] = s
				}
			case "aicode":
				if m, ok := v.(map[string]any); ok {
					out["aicode_keys"] = sortedKeys(m)
					out["experience_type"] = fmt.Sprintf("%T", m["experience"])
					if s, ok := m["experience"].(string); ok {
						out["experience_expected"] = s == "gemini-3.8-flash-tiered"
						if regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_. -]{0,99}$`).MatchString(s) {
							out["experience"] = s
						}
					}
				}
			case "generationConfig":
				if m, ok := v.(map[string]any); ok {
					out["generation_config_keys"] = sortedKeys(m)
					for _, key := range []string{"maxOutputTokens", "temperature", "topP", "topK", "candidateCount"} {
						if f, ok := m[key].(float64); ok {
							out[key] = f
						}
					}
				}
			}
		}
	}
	if contents, ok := value["contents"].([]any); ok {
		out["contents_count"] = len(contents)
		roles := []string{}
		promptSeen := false
		for _, c := range contents {
			if m, ok := c.(map[string]any); ok {
				if role, ok := m["role"].(string); ok && (role == "user" || role == "model") {
					roles = append(roles, role)
				}
				if parts, ok := m["parts"].([]any); ok {
					for _, p := range parts {
						if part, ok := p.(map[string]any); ok {
							if text, ok := part["text"].(string); ok && strings.Contains(text, prompt) {
								promptSeen = true
							}
						}
					}
				}
			}
		}
		out["roles"] = roles
		out["prompt_seen"] = promptSeen
	}
	if tools, ok := value["tools"].([]any); ok {
		out["tools_count"] = len(tools)
	}
	if nested, ok := value["request"].(map[string]any); ok {
		out["request"] = summarize(nested)
		out["request_keys"] = sortedKeys(nested)
	}
	_, out["system_instruction_present"] = value["systemInstruction"]
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fixtureCert() (tls.Certificate, []byte) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("fixture key failed")
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Offline fixture"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), DNSNames: []string{"*.googleapis.com", "antigravity-unleash.goog", "antigravity-cli-auto-updater-974169037036.us-central1.run.app"}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		panic("fixture certificate failed")
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic("fixture TLS failed")
	}
	return cert, certPEM
}
