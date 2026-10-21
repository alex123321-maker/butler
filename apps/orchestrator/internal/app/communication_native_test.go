package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/channel/telegramrich"
	"github.com/butler/butler/apps/orchestrator/internal/communication"
	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The normal Manager owns renewal and encrypted persistence throughout the
// native run. This adapter allows only that provider's routine refresh write;
// no import, other setting, deletion or copied refresh lineage is permitted.
type communicationProviderAuth struct {
	store *config.PostgresSettingsStore
}

func (s communicationProviderAuth) Get(ctx context.Context, key string) (config.Setting, error) {
	if key != "BUTLER_PROVIDER_AUTH_ANTIGRAVITY" {
		return config.Setting{}, errors.New("unexpected provider credential reference")
	}
	v, err := s.store.Get(ctx, key)
	if err != nil {
		return config.Setting{}, errors.New("private provider reference unavailable")
	}
	return v, nil
}
func (s communicationProviderAuth) Set(ctx context.Context, setting config.Setting) (config.Setting, error) {
	if setting.Key != "BUTLER_PROVIDER_AUTH_ANTIGRAVITY" || setting.UpdatedBy != "provider-auth-refresh" || setting.Component != "orchestrator" || !setting.IsSecret {
		return config.Setting{}, errors.New("only normal encrypted provider renewal is permitted")
	}
	return s.store.Set(ctx, setting)
}
func (communicationProviderAuth) Delete(context.Context, string) error {
	return errors.New("native acceptance cannot delete provider credentials")
}

func communicationNativeProvider(t *testing.T, trace *communicationScript) transport.ModelProvider {
	t.Helper()
	route := communicationNativeModelRoute(t)
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_COMMUNICATION_AUTH_POSTGRES_URL"))
	if err != nil {
		t.Fatal("private provider database reference unavailable")
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal("private provider read unavailable")
	}
	t.Cleanup(pool.Close)
	auth := providerauth.NewManager(communicationProviderAuth{store: config.NewPostgresSettingsStore(pool)})
	p, err := providerfactory.New(auth, slog.New(slog.NewTextHandler(io.Discard, nil))).Build(context.Background(), providerfactory.BuildConfig{ActiveProvider: "antigravity", AntigravityModel: "gemini-3.8-flash-high", AntigravityMaximumOutputTokens: 8192, Timeout: 90 * time.Second})
	if err != nil {
		t.Fatal("native provider composition unavailable")
	}
	return &communicationNativeTrace{ModelProvider: p.Provider, trace: trace, route: route}
}

func communicationNativeModelRoute(t *testing.T) *communicationModelRoute {
	t.Helper()
	proxy, err := communicationModelProxy(os.Getenv("HTTPS_PROXY"), http.ProxyFromEnvironment)
	if err != nil {
		t.Fatal("native model requires an explicit proxy for every external provider endpoint")
	}
	port := proxy.Port()
	if port == "" {
		port = "80"
		if proxy.Scheme == "https" {
			port = "443"
		}
	}
	lookup, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	ips, err := net.DefaultResolver.LookupIPAddr(lookup, proxy.Hostname())
	if err != nil || len(ips) == 0 {
		t.Fatal("native model proxy cannot be resolved")
	}
	route := &communicationModelRoute{proxyAddresses: map[string]bool{}, t: t}
	for _, ip := range ips {
		route.proxyAddresses[net.JoinHostPort(ip.IP.String(), port)] = true
	}
	t.Cleanup(func() {
		t.Logf("native model proxy: connection_requests=%d connected=%d denied_direct=%d", route.requested.Load(), route.connected.Load(), route.denied.Load())
	})
	return route
}

// Both the auth manager and native provider use the standard environment proxy
// selector. Validate its effective decision, including NO_PROXY, before release.
func communicationModelProxy(raw string, selectProxy func(*http.Request) (*url.URL, error)) (*url.URL, error) {
	if len(raw) == 0 || len(raw) > 4096 {
		return nil, errors.New("model proxy unavailable")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("model proxy unavailable")
	}
	for _, endpoint := range []string{"https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist", "https://daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent", "https://aicode.googleapis.com/v1:streamGenerateContent", "https://oauth2.googleapis.com/token"} {
		req, _ := http.NewRequest(http.MethodPost, endpoint, nil)
		selected, err := selectProxy(req)
		if err != nil || selected == nil || selected.String() != u.String() {
			return nil, errors.New("model proxy selection denied")
		}
	}
	return u, nil
}

type communicationModelRoute struct {
	proxyAddresses               map[string]bool
	t                            *testing.T
	requested, connected, denied atomic.Int64
}

func (r *communicationModelRoute) context(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	r.t.Cleanup(cancel)
	ctx = providerauth.WithAntigravityDiagnostics(ctx, func(d providerauth.AntigravityDiagnostic) {
		r.t.Logf("native auth: stage=%s outcome=%s status=%d", d.Stage, d.Outcome, d.HTTPStatus)
	})
	return httptrace.WithClientTrace(ctx, &httptrace.ClientTrace{
		// GetConn can report the logical Google host on a reused HTTP/2
		// connection. Check actual socket endpoints, including pooled sockets.
		ConnectStart: func(_, address string) {
			if r.proxyAddresses[address] {
				r.requested.Add(1)
			}
			// The context's nettrace is also inherited by pgx. Only GotConn
			// identifies an HTTP socket; a PostgreSQL dial is not model egress.
		},
		GotConn: func(info httptrace.GotConnInfo) {
			address := info.Conn.RemoteAddr().String()
			if r.proxyAddresses[address] {
				r.connected.Add(1)
				return
			}
			if !communicationKernelLoopback(address) {
				r.denied.Add(1)
				cancel()
			}
		},
	})
}

func communicationKernelLoopback(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func TestCommunicationNativeProxyKeepsKernelLoopbackAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	defer server.Close()
	route := &communicationModelRoute{t: t}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(route.context(ctx), http.MethodPost, server.URL, nil)
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal("kernel loopback was cancelled by provider trace")
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || route.denied.Load() != 0 || route.requested.Load() != 0 {
		t.Fatal("kernel loopback treated as model proxy request")
	}
}

type communicationNativeTrace struct {
	transport.ModelProvider
	trace *communicationScript
	route *communicationModelRoute
}

func (p *communicationNativeTrace) InferenceLimits() (int64, int64) {
	if bounded, ok := p.ModelProvider.(interface{ InferenceLimits() (int64, int64) }); ok {
		return bounded.InferenceLimits()
	}
	return 0, 0
}

func (p *communicationNativeTrace) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	p.trace.starts++
	p.trace.runID = q.Context.RunID
	readyReport := false
	for _, d := range q.ToolDefinitions {
		if d.Name == "communication.report" {
			readyReport = true
		}
	}
	p.route.t.Logf("native initial model catalog: tools=%d report=%t", len(q.ToolDefinitions), readyReport)
	var audit []byte
	if e := p.trace.f.pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(jsonb_build_object('tool',tool_name,'status',status,'error_class',error->>'error_class','error_message',CASE WHEN error->>'error_class'='ERROR_CLASS_VALIDATION_ERROR' THEN left(error->>'message',200) ELSE '' END,'result_status',result->>'status')),'[]'::jsonb) FROM tool_calls WHERE run_id=$1`, q.Context.RunID).Scan(&audit); e == nil {
		p.route.t.Logf("native recorded tool states: %s", audit)
	}
	stream, err := p.ModelProvider.StartRun(p.route.context(ctx), q)
	return p.observe(ctx, stream, err)
}
func (p *communicationNativeTrace) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	p.trace.starts++
	var result struct {
		Status string `json:"status"`
		Error  struct {
			Class string `json:"error_class"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(q.ToolResultJSON), &result) == nil {
		p.route.t.Logf("native tool receipt: status=%s class=%s", result.Status, result.Error.Class)
	}
	stream, err := p.ModelProvider.SubmitToolResult(p.route.context(ctx), q)
	return p.observe(ctx, stream, err)
}
func (p *communicationNativeTrace) ContinueRun(ctx context.Context, q transport.ContinueRunRequest) (transport.EventStream, error) {
	p.trace.starts++
	stream, err := p.ModelProvider.ContinueRun(p.route.context(ctx), q)
	return p.observe(ctx, stream, err)
}

func (p *communicationNativeTrace) observe(ctx context.Context, stream transport.EventStream, err error) (transport.EventStream, error) {
	if err != nil {
		return nil, err
	}
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		for event := range stream {
			if event.ToolCall != nil && communicationcontrol.IsTool(event.ToolCall.ToolName) {
				a, e := communicationcontrol.DecodeArgs(event.ToolCall.ToolName, event.ToolCall.ArgsJSON)
				p.route.t.Logf("native proposal: tool=%s valid=%t purpose=%s blocks=%d", event.ToolCall.ToolName, e == nil, a.Purpose, len(a.Blocks))
				var fields map[string]json.RawMessage
				_ = json.Unmarshal([]byte(event.ToolCall.ArgsJSON), &fields)
				nulls := []string{}
				for k, v := range fields {
					if string(v) == "null" {
						nulls = append(nulls, k)
					}
				}
				sort.Strings(nulls)
				p.route.t.Logf("native proposal null fields: %v", nulls)
				if e == nil && event.ToolCall.ToolName == "communication.report" {
					_, planErr := (telegramrich.Planner{Profile: telegramrich.Capabilities{Message: true, Photo: true, Rich: true, Document: true}}).Plan(ctx, nil, p.trace.runID, communication.Binding{Channel: "telegram"}, a)
					p.route.t.Logf("native report shape: planner_valid=%t", planErr == nil)
					for _, b := range a.Blocks {
						p.route.t.Logf("native report block: type=%s text=%t items=%d rows=%d ref=%t alt=%t", b.Type, b.Text != "", len(b.Items), len(b.Rows), b.ArtifactRef != "", b.Alt != "")
					}
				}
			}
			if event.AssistantFinal != nil {
				p.route.t.Logf("native internal final: bytes=%d", len(event.AssistantFinal.Content))
			}
			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func TestCommunicationNativeModelProxyRejectsBypass(t *testing.T) {
	proxy, _ := url.Parse("http://operator:private@127.0.0.1:18080")
	for _, mode := range []string{"explicit", "bypass", "different", "failure"} {
		t.Run(mode, func(t *testing.T) {
			_, err := communicationModelProxy(proxy.String(), func(req *http.Request) (*url.URL, error) {
				if req.URL.Host == "daily-cloudcode-pa.googleapis.com" {
					switch mode {
					case "bypass":
						return nil, nil
					case "different":
						return &url.URL{Scheme: "http", Host: "other:80"}, nil
					case "failure":
						return nil, errors.New("private proxy error")
					}
				}
				return proxy, nil
			})
			if (err == nil) != (mode == "explicit") {
				t.Fatal("incorrect proxy admission")
			}
			if err != nil && strings.Contains(err.Error(), "operator") {
				t.Fatal("proxy credential disclosed")
			}
		})
	}
	for _, raw := range []string{"", "socks5://127.0.0.1:1080", "http://proxy/path", "https://proxy?secret=value"} {
		if _, err := communicationModelProxy(raw, http.ProxyURL(proxy)); err == nil {
			t.Fatal("invalid proxy accepted")
		}
	}
}

func TestNativeCommunicationTelegramAgentIntegration(t *testing.T) {
	if os.Getenv("BUTLER_COMMUNICATION_AUTH_POSTGRES_URL") == "" || os.Getenv("BUTLER_TEST_TELEGRAM_RICH_TOKEN_FILE") == "" {
		t.Skip("explicit private native provider and isolated test bot references required")
	}
	communicationCoreScenarios(t, true, true)
}

// Reachability only: no model inference, auth-store read, refresh or credential.
func TestNativeCommunicationModelProxyConnectivity(t *testing.T) {
	if os.Getenv("BUTLER_TEST_MODEL_PROXY_PROBE") != "1" {
		t.Skip("explicit isolated proxy probe required")
	}
	route := communicationNativeModelRoute(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(route.context(ctx), http.MethodPost, "https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone(), Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal("provider endpoint unavailable through required proxy")
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if route.requested.Load() < 1 || route.connected.Load() < 1 || route.denied.Load() != 0 {
		t.Fatal("proxy connection not confirmed")
	}
	if resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden {
		t.Fatal("unexpected unauthenticated provider status")
	}
	t.Logf("native model proxy reachability: HTTP %d; no authorization or inference", resp.StatusCode)
}
