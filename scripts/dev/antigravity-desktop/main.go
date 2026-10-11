// antigravity-desktop captures the installed Desktop and actual Go transport.
// Synthetic mode has no credentials or egress; the private metadata mode never forwards inference.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const desktopCL = "995734886"
const desktopSHA = "bbb745ed391e82bb18ed85ceef36bb646a2a0cf801903bc1ef0b8e6df8ba1bc8"
const fixturePrompt = "Return only DESKTOP_OFFLINE_OK. Do not use tools."

type fixture struct {
	mu           sync.Mutex
	requests     []map[string]any
	stdout       bytes.Buffer
	publicBinary []byte
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "native-fixture" {
		report, err := nativeFixture()
		if err != nil {
			safeFailure()
		}
		_ = json.NewEncoder(os.Stdout).Encode(report)
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "metadata-gate" {
		if runMetadataGateway() != nil {
			safeFailure()
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "inspect-auth" && os.Getenv("BUTLER_DESKTOP_OFFLINE") == "1" {
		client := http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, Timeout: 5 * time.Second}
		for _, method := range []string{"ListPluginAuthProviders", "HasAuthToken", "GetAuthStatus"} {
			r, _ := http.NewRequest("POST", "https://127.0.0.1:49631/exa.language_server_pb.LanguageServerService/"+method, strings.NewReader(`{}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Connect-Protocol-Version", "1")
			r.Header.Set("x-codeium-csrf-token", "synthetic-desktop-csrf")
			response, err := client.Do(r)
			if err != nil {
				safeFailure()
				return
			}
			b, _ := io.ReadAll(io.LimitReader(response.Body, 65536))
			_ = response.Body.Close()
			var v map[string]any
			_ = json.Unmarshal(b, &v)
			out := map[string]any{"method": method, "http_status": response.StatusCode, "real_model_calls": 0}
			if token, ok := v["hasToken"].(bool); ok {
				out["has_token"] = token
			}
			if a, ok := v["authResult"].(map[string]any); ok {
				out["has_valid_auth"] = a["hasValidAuth"] == true
				out["general_error_present"] = a["generalError"] != nil
				out["license_required"] = a["licenseRequired"] != nil
				out["verification_required"] = a["verificationRequired"] != nil
				diagnostic, _ := json.Marshal(map[string]any{"error": a["generalError"], "ui": a["uiMessage"]})
				words := []string{}
				for _, word := range []string{"scope", "expired", "license", "quota", "unmarshal", "parse", "model", "invalid", "permission", "denied", "consumer", "business", "userinfo", "user info", "keyring", "credentials", "token", "403", "429", "configuration", "subscription", "terms", "tos", "account", "project", "Google", "retry", "connect", "onboard", "email", "region", "verify", "update"} {
					if strings.Contains(strings.ToLower(string(diagnostic)), word) {
						words = append(words, word)
					}
				}
				out["general_error_fixed_flags"] = words
			}
			if providers, ok := v["providers"].([]any); ok {
				out["provider_count"] = len(providers)
				labels := []string{}
				for _, p := range providers {
					if m, ok := p.(map[string]any); ok {
						for _, key := range []string{"plugin", "provider", "type"} {
							if s, ok := m[key].(string); ok {
								switch s {
								case "google", "google-auth", "google_oauth", "google-oauth", "googleOAuth", "oauth", "oauth2", "builtin", "auth":
									labels = append(labels, key+":"+s)
								}
							}
						}
					}
				}
				out["known_provider_labels"] = labels
			}
			_ = json.NewEncoder(os.Stdout).Encode(out)
		}
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "relay" {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:49631", 3*time.Second)
		if err != nil {
			os.Exit(1)
		}
		defer conn.Close()
		go func() {
			_, _ = io.Copy(conn, os.Stdin)
			if tcp, ok := conn.(*net.TCPConn); ok {
				_ = tcp.CloseWrite()
			}
		}()
		_, _ = io.Copy(os.Stdout, conn)
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "report" {
		client := http.Client{Transport: &http.Transport{}, Timeout: 3 * time.Second}
		reportURL := "http://127.0.0.1:3128/report"
		if os.Getenv("BUTLER_DESKTOP_METADATA") == "1" {
			reportURL = "http://desktop-metadata:3128/report"
		}
		r, err := client.Get(reportURL)
		if err != nil {
			safeFailure()
			return
		}
		defer r.Body.Close()
		_, _ = io.Copy(os.Stdout, io.LimitReader(r.Body, 1024*1024))
		return
	}
	if len(os.Args) != 2 || os.Args[1] != "fixture" || os.Getenv("BUTLER_DESKTOP_OFFLINE") != "1" {
		safeFailure()
		return
	}
	if err := runFixture(); err != nil {
		safeFailure()
	}
}

func safeFailure() {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"offline_fixture_failed": true, "real_model_calls": 0, "raw_output_suppressed": true})
	os.Exit(1)
}

func knownHost(host string) bool {
	switch host {
	case "daily-cloudcode-pa.googleapis.com", "cloudcode-pa.googleapis.com", "aicode.googleapis.com", "generativelanguage.googleapis.com", "oauth2.googleapis.com", "www.googleapis.com", "antigravity-unleash.goog", "play.googleapis.com":
		return true
	}
	return false
}

func projection(r *http.Request, body []byte) map[string]any {
	known := knownHost(r.Host)
	host := "UNKNOWN"
	if known {
		host = r.Host
	}
	path := "UNKNOWN"
	for _, prefix := range []string{"/v1internal:", "/v1:", "/v1main:"} {
		for _, method := range []string{"streamGenerateContent", "generateContent", "streamGenerateChat", "generateChat", "internalAtomicAgenticChat", "listModelConfigs", "listExperiments", "getCodeAssistGlobalUserSetting", "loadCodeAssist", "fetchAvailableModels", "fetchConfig", "fetchLicenses", "fetchUserInfo", "retrieveUserQuotaSummary", "fetchAdminControls", "onboardUser"} {
			if r.URL.Path == prefix+method {
				path = r.URL.Path
			}
		}
	}
	if r.URL.Path == "/oauth2/v2/userinfo" || r.URL.Path == "/api/client/features" {
		path = r.URL.Path
	}
	out := map[string]any{"host": host, "path": path, "method": r.Method, "request_bytes": len(body), "bearer_is_synthetic": r.Header.Get("Authorization") == "Bearer synthetic-access", "real_model_calls": 0}
	out["alt_sse"] = r.URL.Query().Get("alt") == "sse"
	ua := r.Header.Get("User-Agent")
	out["user_agent_is_native_butler"] = ua == "antigravity/1.3.3 butler/0.1"
	out["user_agent_mentions_desktop_version"] = strings.Contains(ua, "2.22.0")
	for _, key := range []string{"Accept", "Content-Type"} {
		value := r.Header.Get(key)
		switch value {
		case "application/json", "text/event-stream", "application/json; charset=utf-8":
			out[strings.ToLower(key)] = value
		}
	}
	var root map[string]any
	if json.Unmarshal(body, &root) != nil {
		return out
	}
	allowed := []string{"model", "aicode", "request", "contents", "systemInstruction", "generationConfig", "sessionId", "project", "requestId", "requestType", "userAgent", "labels"}
	keys := []string{}
	unknown := 0
	for key := range root {
		found := false
		for _, k := range allowed {
			if key == k {
				found = true
				keys = append(keys, key)
			}
		}
		if !found {
			unknown++
		}
	}
	sort.Strings(keys)
	out["recognized_top_keys"] = keys
	out["unknown_top_key_count"] = unknown
	model, _ := root["model"].(string)
	if aicode, ok := root["aicode"].(map[string]any); ok {
		model, _ = aicode["experience"].(string)
	}
	out["model"] = "UNKNOWN"
	for _, value := range []string{"gemini-3.8-flash-tiered", "gemini-3.8-flash-high", "gemini-3.8-flash-medium", "gemini-3.8-flash-low", "gemini-3.1-pro-low", "chat-gemini-3.1-pro-low", "gemini-2.5-flash", "gemini-2.5-flash-lite", "gemini-3.8-flash", "gemini-3.8-flash-lite", "gemini-3.8-flash-minimal"} {
		if model == value {
			out["model"] = value
		}
	}
	request := root
	if nested, ok := root["request"].(map[string]any); ok {
		request = nested
	}
	if cfg, ok := request["generationConfig"].(map[string]any); ok {
		configKeys := []string{}
		for _, k := range []string{"maxOutputTokens", "thinkingConfig", "responseMimeType", "temperature", "topP", "topK", "candidateCount"} {
			if _, exists := cfg[k]; exists {
				configKeys = append(configKeys, k)
			}
		}
		out["generation_config_keys"] = configKeys
		if n, ok := cfg["maxOutputTokens"].(float64); ok {
			out["max_output_tokens"] = n
		}
		if thinking, ok := cfg["thinkingConfig"].(map[string]any); ok {
			if n, ok := thinking["thinkingBudget"].(float64); ok {
				out["thinking_budget"] = n
			}
			if level, ok := thinking["thinkingLevel"].(string); ok {
				switch level {
				case "HIGH", "MEDIUM", "LOW", "MINIMAL":
					out["thinking_level"] = level
				}
			}
		}
	}
	if contents, ok := request["contents"].([]any); ok {
		out["contents_count"] = len(contents)
		seen := false
		userBytes := 0
		for _, c := range contents {
			if m, ok := c.(map[string]any); ok {
				if parts, ok := m["parts"].([]any); ok {
					for _, p := range parts {
						if part, ok := p.(map[string]any); ok {
							if text, ok := part["text"].(string); ok {
								seen = seen || strings.Contains(text, fixturePrompt)
								if m["role"] == "user" {
									userBytes += len(text)
								}
							}
						}
					}
				}
			}
		}
		out["fixture_prompt_present"] = seen
		out["user_text_bytes"] = userBytes
	}
	if system, ok := request["systemInstruction"].(map[string]any); ok {
		b, _ := json.Marshal(system)
		out["system_instruction_bytes"] = len(b)
	}
	if tools, ok := request["tools"].([]any); ok {
		out["tool_group_count"] = len(tools)
		count := 0
		for _, tool := range tools {
			if t, ok := tool.(map[string]any); ok {
				if declarations, ok := t["functionDeclarations"].([]any); ok {
					count += len(declarations)
				}
			}
		}
		out["function_declaration_count"] = count
	}
	_, out["session_id_present"] = request["sessionId"]
	return out
}

func (f *fixture) serve(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 65537))
	if err != nil || len(body) > 65536 {
		http.Error(w, "fixture bound", 413)
		return
	}
	if !knownHost(r.Host) {
		http.Error(w, "fixture host denied", 403)
		return
	}
	f.mu.Lock()
	entry := projection(r, body)
	if entry["path"] == "UNKNOWN" && regexp.MustCompile(`^/[A-Za-z0-9/_:.]{1,120}$`).MatchString(r.URL.Path) && bytes.Contains(f.publicBinary, []byte(r.URL.Path)) {
		entry["path"] = r.URL.Path
		entry["path_verified_in_public_binary"] = true
	}
	f.requests = append(f.requests, entry)
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if strings.Contains(r.URL.Path, "GenerateContent") || strings.Contains(r.URL.Path, "GenerateChat") || strings.Contains(r.URL.Path, "AgenticChat") {
		w.WriteHeader(429)
		_, _ = io.WriteString(w, `{"error":{"code":429,"status":"RESOURCE_EXHAUSTED","message":"synthetic offline capture; no upstream"}}`)
		return
	}
	switch {
	case strings.Contains(r.URL.Path, "loadCodeAssist"):
		_, _ = io.WriteString(w, `{"cloudaicompanionProject":"synthetic-project","currentTier":{"id":"free-tier","name":"Antigravity"},"paidTier":{"id":"g1-pro-tier","name":"Google AI Pro"}}`)
	case strings.Contains(r.URL.Path, "fetchAvailableModels"):
		_, _ = io.WriteString(w, `{"models":{"gemini-3.8-flash-tiered":{"displayName":"Gemini 3.8 Flash High","model":"MODEL_PLACEHOLDER_M322","apiProvider":"API_PROVIDER_GOOGLE_GEMINI","supportsThinking":true,"recommended":true,"maxTokens":1048576,"maxOutputTokens":65536,"quotaInfo":{"remainingFraction":1}},"gemini-3.8-flash-high":{"displayName":"Gemini 3.8 Flash High","model":"MODEL_PLACEHOLDER_M322","apiProvider":"API_PROVIDER_GOOGLE_GEMINI","supportsThinking":true,"recommended":true,"maxTokens":1048576,"maxOutputTokens":65536,"quotaInfo":{"remainingFraction":1}}},"defaultAgentModelId":"gemini-3.8-flash-tiered","agentModelSorts":[{"displayName":"Models","groups":[{"displayName":"Gemini","modelIds":["gemini-3.8-flash-tiered"]}]}],"tieredModelIds":{"flash":["gemini-3.8-flash-tiered"]}}`)
	case strings.Contains(r.URL.Path, "userinfo"):
		_, _ = io.WriteString(w, `{"email":"fixture@example.invalid","id":"synthetic-user","name":"Synthetic fixture"}`)
	default:
		_, _ = io.WriteString(w, `{}`)
	}
}

func runFixture() error {
	b, err := os.ReadFile("/language_server")
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != desktopSHA {
		return os.ErrPermission
	}
	stamp, err := exec.Command("/language_server", "--stamp").Output()
	if err != nil || !bytes.Contains(stamp, []byte("Built at CL: "+desktopCL)) {
		return os.ErrPermission
	}
	root, err := os.MkdirTemp("", "desktop-fixture-")
	if err != nil {
		return err
	}
	home := filepath.Join(root, "home")
	profile := filepath.Join(home, ".gemini", "antigravity")
	for _, p := range []string{profile, filepath.Join(home, ".gemini", "config"), filepath.Join(root, "work")} {
		if os.MkdirAll(p, 0700) != nil {
			return os.ErrPermission
		}
	}
	id := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`)) + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"email":"fixture@example.invalid","sub":"synthetic-user","exp":4070908800}`)) + ".fixture"
	login, _ := json.Marshal(map[string]any{"token": map[string]any{"access_token": "synthetic-access", "refresh_token": "synthetic-refresh", "token_type": "Bearer", "expiry": "2099-01-01T00:00:00Z"}, "auth_method": "consumer", "id_token": id})
	if os.WriteFile(filepath.Join(profile, "antigravity-oauth-token"), login, 0600) != nil {
		return os.ErrPermission
	}
	// Desktop has a different standalone token filename from CLI. Probe only fake homes.
	for _, dir := range []string{profile, filepath.Join(home, ".gemini"), filepath.Join(home, ".config", "antigravity"), filepath.Join(home, ".gemini", "config")} {
		if os.MkdirAll(dir, 0700) != nil || os.WriteFile(filepath.Join(dir, "jetski-standalone-oauth-token"), login, 0600) != nil {
			return os.ErrPermission
		}
	}
	if os.WriteFile(filepath.Join(home, ".gemini", "config", "config.json"), []byte(`{}`), 0600) != nil {
		return os.ErrPermission
	}
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	now := time.Now()
	template := x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Synthetic Desktop"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSNames: []string{"*.googleapis.com", "antigravity-unleash.goog"}}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, key)
	if err != nil {
		return err
	}
	cpem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	kpemDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	cert, err := tls.X509KeyPair(cpem, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kpemDER}))
	if err != nil {
		return err
	}
	ca := filepath.Join(root, "ca.pem")
	if os.WriteFile(ca, cpem, 0600) != nil {
		return os.ErrPermission
	}
	f := &fixture{publicBinary: b}
	defer func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		text := f.stdout.String()
		frames := []string{}
		panicLiteral := "UNKNOWN"
		for _, match := range regexp.MustCompile(`(?m)^panic: ([A-Za-z0-9 _:/().-]{1,160})$`).FindAllStringSubmatch(text, 1) {
			if bytes.Contains(f.publicBinary, []byte(match[1])) {
				panicLiteral = match[1]
			}
		}
		for _, match := range regexp.MustCompile(`(?m)^((?:google3/[A-Za-z0-9_/.]+|runtime)[.][A-Za-z0-9_().*]+)\(`).FindAllStringSubmatch(text, 20) {
			frames = append(frames, match[1])
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"fixture_termination": true, "panic_public_literal": panicLiteral, "public_stack_functions": frames, "nil_pointer": strings.Contains(text, "nil pointer"), "malformed_json": strings.Contains(text, "unmarshal"), "requests": f.requests, "real_model_calls": 0, "generation_forwarding_enabled": false, "raw_output_suppressed": true})
	}()
	tlsListener, err := net.Listen("tcp", "127.0.0.1:3129")
	if err != nil {
		return err
	}
	defer tlsListener.Close()
	server := http.Server{Handler: http.HandlerFunc(f.serve), ErrorLog: log.New(io.Discard, "", 0), ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(tls.NewListener(tlsListener, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}))
	defer server.Close()
	proxy := http.Server{ErrorLog: log.New(io.Discard, "", 0), ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/report" {
			f.mu.Lock()
			defer f.mu.Unlock()
			flags := []string{}
			for _, word := range []string{"oauth", "token", "certificate", "auth", "error", "invalid", "missing", "config", "port", "license", "model", "panic"} {
				if strings.Contains(strings.ToLower(f.stdout.String()), word) {
					flags = append(flags, word)
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"requests": f.requests, "diagnostic_flags": flags, "real_model_calls": 0, "synthetic_credentials_only": true, "generation_forwarding_enabled": false, "desktop_build_cl": desktopCL, "desktop_linux_sha256": desktopSHA})
			return
		}
		host, port, err := net.SplitHostPort(r.Host)
		if r.Method != http.MethodConnect || err != nil || port != "443" || !knownHost(host) {
			http.Error(w, "synthetic proxy denied", 403)
			return
		}
		up, err := net.DialTimeout("tcp", "127.0.0.1:3129", time.Second)
		if err != nil {
			http.Error(w, "synthetic proxy unavailable", 503)
			return
		}
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			up.Close()
			return
		}
		_, _ = buf.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = buf.Flush()
		go func() { defer conn.Close(); defer up.Close(); go io.Copy(up, conn); _, _ = io.Copy(conn, up) }()
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:3128")
	if err != nil {
		return err
	}
	defer listener.Close()
	go proxy.Serve(listener)
	defer proxy.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/language_server", "--standalone", "--override_ide_name", "antigravity", "--subclient_type", "hub", "--override_ide_version", "2.22.0", "--override_user_agent_name", "antigravity", "--https_server_port", "49631", "--csrf_token", "synthetic-desktop-csrf", "--app_data_dir", "antigravity", "--api_server_url", "https://generativelanguage.googleapis.com", "--cloud_code_endpoint", "https://daily-cloudcode-pa.googleapis.com", "--disable_telemetry")
	cmd.Dir = filepath.Join(root, "work")
	cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=" + home, "XDG_CONFIG_HOME=" + home, "HTTPS_PROXY=http://127.0.0.1:3128", "HTTP_PROXY=http://127.0.0.1:3128", "NO_PROXY=localhost,127.0.0.1", "SSL_CERT_FILE=" + ca, "SSL_CERT_DIR=" + root}
	if os.Getenv("BUTLER_DESKTOP_METADATA") == "1" {
		cmd.Env = []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/private-desktop/home", "XDG_CONFIG_HOME=/private-desktop/home", "HTTPS_PROXY=http://desktop-metadata:3128", "HTTP_PROXY=http://desktop-metadata:3128", "NO_PROXY=localhost,127.0.0.1", "SSL_CERT_FILE=/private-desktop/ca.pem", "SSL_CERT_DIR=" + root}
	}
	cmd.Stdout = lockedOutput{f}
	cmd.Stderr = lockedOutput{f}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "SYNTHETIC_DESKTOP_STARTING", "https_port": 49631, "real_model_calls": 0})
	return cmd.Run()
}

type lockedOutput struct{ f *fixture }

func (w lockedOutput) Write(b []byte) (int, error) {
	w.f.mu.Lock()
	defer w.f.mu.Unlock()
	if w.f.stdout.Len() < 1024*1024 {
		_, _ = w.f.stdout.Write(b)
	}
	return len(b), nil
}
