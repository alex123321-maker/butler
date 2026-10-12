package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
	"github.com/jackc/pgx/v5/pgxpool"
)

func verifyHighHistory(root string) error {
	if verifyHistory(root) != nil || verifyComparison(root) != nil {
		return denied
	}
	for path, expected := range map[string]string{
		"deploy/antigravity/native8192-20261011.json":           "f3474b9d7b6441d1a9ea66dcf84b8aecc7dbe0c7dfcc1494bff3eaa7c19dcdb5",
		"deploy/antigravity/desktop-interception-20261012.json": "208f7d4702451b36e8f27ab25474fcb3edc66e1e9a18074fd5a946d2ba47dffe",
	} {
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return denied
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != expected {
			return denied
		}
	}
	return nil
}

// highGate only admits the unchanged production High body once. It does not
// remove fields, change caps, spoof a system instruction or retry a rejection.
type highGate struct {
	mu            sync.Mutex
	upstream      *http.Client
	auth          providerauth.AntigravityAuth
	budget        admission
	observation   *observation
	metadataCalls int
}

func validHighRequest(r *http.Request, data []byte, auth providerauth.AntigravityAuth) bool {
	if r.Method != "POST" || r.Host != "daily-cloudcode-pa.googleapis.com" || r.URL.Path != "/v1internal:streamGenerateContent" || r.URL.RawQuery != "alt=sse" || r.Header.Get("Authorization") != "Bearer "+auth.AccessToken || r.Header.Get("Content-Type") != "application/json" || r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("User-Agent") != "antigravity/2.22.0 butler/0.1" || len(data) > requestLimit || bytes.Contains(data, []byte(auth.AccessToken)) {
		return false
	}
	var root map[string]any
	if !uniqueJSON(data) || json.Unmarshal(data, &root) != nil || len(root) != 6 || root["model"] != antigravity.FlashHighModel || root["project"] != auth.ProjectID || root["userAgent"] != "antigravity" || root["requestType"] != "agent" {
		return false
	}
	id, ok := root["requestId"].(string)
	if !ok || !strings.HasPrefix(id, "agent-") || len(id) != 42 {
		return false
	}
	nested, ok := root["request"].(map[string]any)
	if !ok || len(nested) != 3 {
		return false
	}
	session, ok := nested["sessionId"].(string)
	if !ok || len(session) != 32 {
		return false
	}
	contents, ok := nested["contents"].([]any)
	if !ok || len(contents) != 1 {
		return false
	}
	content, ok := contents[0].(map[string]any)
	if !ok || !knownKeys(content, "role", "parts") {
		return false
	}
	parts, ok := content["parts"].([]any)
	if !ok || len(parts) != 1 {
		return false
	}
	part, ok := parts[0].(map[string]any)
	if !ok || !knownKeys(part, "text") {
		return false
	}
	var request struct {
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
		Config struct {
			Output   int `json:"maxOutputTokens"`
			Thinking struct {
				Budget int `json:"thinkingBudget"`
			} `json:"thinkingConfig"`
		} `json:"generationConfig"`
	}
	encoded, _ := json.Marshal(nested)
	if json.Unmarshal(encoded, &request) != nil || len(request.Contents) != 1 || request.Contents[0].Role != "user" || len(request.Contents[0].Parts) != 1 || request.Contents[0].Parts[0].Text != comparePrompt || request.Config.Output != 8192 || request.Config.Thinking.Budget != -1 {
		return false
	}
	gc, ok := nested["generationConfig"].(map[string]any)
	if !ok || len(gc) != 2 {
		return false
	}
	tc, ok := gc["thinkingConfig"].(map[string]any)
	if !ok || len(tc) != 1 {
		return false
	}
	return true
}

func (g *highGate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(io.LimitReader(r.Body, requestLimit+1))
	if err != nil || len(data) > requestLimit {
		http.Error(w, "input bound", 403)
		return
	}
	metadata := r.Method == "POST" && r.Host == "daily-cloudcode-pa.googleapis.com" && r.URL.Path == "/v1internal:loadCodeAssist" && r.URL.RawQuery == "" && string(data) == "{}" && r.Header.Get("Authorization") == "Bearer "+g.auth.AccessToken
	if metadata {
		g.mu.Lock()
		g.metadataCalls++
		allowed := g.metadataCalls == 1
		g.mu.Unlock()
		if !allowed {
			http.Error(w, "metadata allowance spent", 403)
			return
		}
	} else if !validHighRequest(r, data, g.auth) {
		http.Error(w, "High profile denied", 403)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()
	var result observation
	if !metadata {
		id, err := g.budget.Reserve(ctx, "go")
		if err != nil {
			http.Error(w, "High allowance spent", 403)
			return
		}
		result = observation{Attempt: id, Client: "go", Host: r.Host, Path: r.URL.Path, Model: antigravity.FlashHighModel, RequestBytes: len(data), OutputCap: 8192, APIStatus: "UNKNOWN", Reason: "UNKNOWN", Outcome: "UNKNOWN"}
		defer func() {
			persist, stop := context.WithTimeout(context.Background(), 5*time.Second)
			defer stop()
			if g.budget.Observe(persist, result.Attempt, result) != nil {
				result.Outcome = "OBSERVATION_PERSIST_FAILED"
			}
			g.mu.Lock()
			g.observation = &result
			g.mu.Unlock()
		}()
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://"+r.Host+r.URL.RequestURI(), bytes.NewReader(data))
	// Fixed safe headers; no replayable POST or redirects. Never forward cookies,
	// credit settings, API keys or arbitrary client authorization.
	for _, key := range []string{"Content-Type", "Accept", "User-Agent"} {
		req.Header.Set(key, r.Header.Get(key))
	}
	req.Header.Set("Authorization", "Bearer "+g.auth.AccessToken)
	resp, err := g.upstream.Do(req)
	if err != nil {
		result.Outcome = "CONNECTION_OR_TIMEOUT"
		http.Error(w, "upstream unavailable", 502)
		return
	}
	defer resp.Body.Close()
	result.HTTPStatus = resp.StatusCode
	limit := responseLimit
	if metadata {
		limit = 1024 * 1024
	}
	response, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if err != nil || len(response) > limit {
		result.Outcome = "RESPONSE_BOUND_OR_TIMEOUT"
		result.ResponseBoundExceeded = len(response) > limit
		http.Error(w, "response bound", 502)
		return
	}
	if !metadata {
		projectResponse(response, &result)
		if resp.StatusCode != 200 {
			result.Outcome = "HTTP_REJECTED"
		} else if result.MalformedResponse || result.ToolProposal || result.Finish != "STOP" || !result.MarkerMatch {
			result.Outcome = "INVALID_OR_NONTERMINAL"
			http.Error(w, "High acceptance failed", 502)
			return
		} else {
			result.Outcome = "MARKER_COMPLETE"
		}
	}
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(response)
}

func runHigh() error {
	if os.Getenv("BUTLER_HIGH_APPROVED") != "1" || verifyHighHistory("/src") != nil || loadOperatorEnvironment() != nil {
		return denied
	}
	if os.Getenv("BUTLER_COMPARE_SUBSCRIPTION_NO_OVERAGE") != "1" {
		return denied
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	bootPhase = "HIGH_PRIVATE_ORIGIN"
	pool, err := pgxpool.New(ctx, os.Getenv("BUTLER_POSTGRES_URL"))
	if err != nil {
		return denied
	}
	defer pool.Close()
	manager := providerauth.NewManager(config.NewPostgresSettingsStore(pool))
	reference, err := privateRead("/auth/import.json")
	if err != nil {
		return denied
	}
	defer clear(reference)
	origin, err := manager.AuditAntigravityOrigin(ctx, bytes.NewReader(reference))
	if err != nil || !origin.StoredCredentials || !origin.OAuthApplicationMatches || !origin.OAuthCredentialMatches || !origin.RefreshCredentialMatches {
		return denied
	}
	bootPhase = "HIGH_SUBSCRIPTION_DISCOVERY"
	auth, err := manager.ResolveAntigravityCloudCode(ctx)
	if err != nil || !auth.ExpiresAt.After(time.Now().Add(3*time.Minute)) {
		code := highDiscoveryDiagnostic(err)
		if err == nil {
			code = "ACCESS_VALIDITY_TOO_SHORT"
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"phase": bootPhase, "fixed_diagnostic": code, "real_model_calls": 0})
		return denied
	}
	upstream := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone(), Timeout: 45 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	bootPhase = "HIGH_DIAGNOSTIC_LEDGER"
	ledger, err := pgxpool.New(ctx, os.Getenv("BUTLER_COMPARE_POSTGRES_URL"))
	if err != nil {
		return denied
	}
	defer ledger.Close()
	if ledger.Ping(ctx) != nil {
		return denied
	}
	budget := &pgAdmission{pool: ledger, root: "/src", high: true}
	if budget.init(ctx) != nil {
		return denied
	}
	g := &highGate{upstream: upstream, auth: auth, budget: budget}
	server := httptest.NewUnstartedServer(g)
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	old := http.DefaultTransport
	http.DefaultTransport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12}, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "daily-cloudcode-pa.googleapis.com:443" {
			return nil, denied
		}
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	defer func() { http.DefaultTransport = old }()
	bootPhase = "HIGH_ACTUAL_PROVIDER"
	provider, err := antigravity.NewProvider(antigravity.Config{Model: antigravity.FlashHighModel, AuthSource: manager, Timeout: 45 * time.Second})
	if err != nil {
		return denied
	}
	stream, err := provider.StartRun(transport.WithInputByteLimit(ctx, requestLimit), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "high-acceptance", SessionKey: "high-acceptance", ProviderName: antigravity.Name, ModelName: antigravity.FlashHighModel}, InputItems: []transport.InputItem{{Role: "user", Content: comparePrompt}}, StreamingEnabled: true})
	if err != nil {
		return denied
	}
	final := false
	diagnostic := "UNKNOWN"
	for event := range stream {
		if event.TransportError != nil {
			diagnostic = transport.DiagnosticCode(event.TransportError)
		}
		if event.AssistantFinal != nil {
			final = event.AssistantFinal.Content == marker && event.AssistantFinal.FinishReason == "STOP"
			diagnostic = "ASSISTANT_FINAL"
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"profile": "flash-high-subscription", "origin": origin, "paid_subscription_confirmed": true, "actual_go_provider": true, "native_final_verified": final, "native_diagnostic": diagnostic, "metadata_calls_through_gate": g.metadataCalls, "result": g.observation, "historical_attempts": 21, "historical_known_usage_pairs": 7, "historical_unknown_usage_pairs": 14, "historical_input_subtotal": 26939, "historical_output_subtotal": 581, "supplemental_credit_allowance": 0, "retry_enabled": false, "window_sealed": true})
}

func highDiscoveryDiagnostic(err error) string {
	if err == nil {
		return "READY"
	}
	switch err.Error() {
	case "Antigravity control-plane request failed":
		return "CONNECTION_OR_TIMEOUT"
	case "Antigravity control-plane response is invalid":
		return "INVALID_METADATA_RESPONSE"
	case "Antigravity paid subscription was not confirmed":
		return "PAID_TIER_UNCONFIRMED"
	case "Antigravity subscription project is unavailable":
		return "PROJECT_UNAVAILABLE"
	case "Antigravity encrypted credential persistence failed":
		return "ENCRYPTED_PERSISTENCE"
	default:
		return "CONTROL_PLANE_REJECTED"
	}
}
