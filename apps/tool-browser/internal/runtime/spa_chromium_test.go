package runtime

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/browserspa"
	"github.com/butler/butler/internal/browserspa/eval"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"google.golang.org/grpc/metadata"
)

type spaRuntimeAuthority struct {
	spec     resourceexec.RuntimeSpec
	finished bool
	revoked  bool
	targets  []string
	mu       sync.Mutex
}

func (a *spaRuntimeAuthority) Inspect(context.Context, resourceexec.RuntimeRequest) (resourceexec.RuntimeSpec, error) {
	return a.spec, nil
}
func (a *spaRuntimeAuthority) Begin(context.Context, resourceexec.RuntimeRequest) (resourceexec.RuntimeSpec, error) {
	return a.spec, nil
}
func (a *spaRuntimeAuthority) Finish(_ context.Context, r resourceexec.Receipt) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.finished = r.Completed
	return nil
}
func (a *spaRuntimeAuthority) Check(ctx context.Context, r resourceexec.RuntimeRequest, target string) (resourceexec.RuntimeSpec, error) {
	if ctx.Err() != nil || r.ToolName != browserspa.Tool || !browserspa.NetworkURL("https://example.com/", target) {
		return resourceexec.RuntimeSpec{}, resourceexec.ErrDenied
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.revoked {
		return resourceexec.RuntimeSpec{}, resourceexec.ErrDenied
	}
	a.targets = append(a.targets, target)
	return a.spec, nil
}

func TestSPAReadChromium(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("requires actual Chromium")
	}
	for _, mode := range []string{"normal", "cookie_reflection", "mutation", "private", "foreign", "worker_mutation", "worker_private", "ws_send", "oversized", "storm", "lost_query", "infinite", "cancelled", "revoked", "helper_lost"} {
		t.Run(mode, func(t *testing.T) {
			owned, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			querySeen := make(chan struct{}, 1)
			var mu sync.Mutex
			var events []eval.Event
			scenario := mode
			if mode == "cancelled" || mode == "revoked" || mode == "helper_lost" {
				scenario = "normal"
			}
			title := "Independent runtime result"
			if mode == "cookie_reflection" {
				scenario = "normal"
				title = "private-spa-cookie-82619"
			}
			f, err := eval.New(eval.Config{Origin: "https://example.com", Variant: "runtimetest01", Category: "books", Title: title, Marker: "runtime-receipt-1837", Stock: 61, Scenario: scenario}, func(e eval.Event) {
				mu.Lock()
				events = append(events, e)
				mu.Unlock()
				if e.Kind == "query" {
					select {
					case querySeen <- struct{}{}:
					default:
					}
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "cookie_reflection" {
					http.SetCookie(w, &http.Cookie{Name: "fixture_session", Value: title, Path: "/", Secure: true, HttpOnly: true})
				}
				f.ServeHTTP(w, r)
			}))
			defer server.Close()
			if server.Certificate().VerifyHostname("example.com") != nil {
				t.Fatal("fixture TLS identity mismatch")
			}
			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			policy := egresstest.Policy(server)
			policy.TLSRoots = roots
			script, _ := filepath.Abs(filepath.Join("..", "..", "scripts", "browser_runtime.mjs"))
			runner := NewPlaywrightRunnerWithPolicy("node", script, policy)
			runner.workspaceRoot = filepath.Clean(filepath.Join(filepath.Dir(script), "..", "..", ".."))
			backend := NewServer(runner, nil)
			backend.SPAEnabled = true
			defer backend.Close()
			authority := &spaRuntimeAuthority{spec: resourceexec.RuntimeSpec{Lane: "spa-fixture-lane", Fence: 1, Deadline: time.Now().Add(20 * time.Second), DispatchID: strings.Repeat("1", 48)}}
			if mode == "helper_lost" {
				if err = runner.start(owned); err != nil {
					t.Fatal("helper did not start")
				}
			}
			if mode == "cancelled" || mode == "revoked" || mode == "helper_lost" {
				process := runner.cmd
				go func() {
					select {
					case <-owned.Done():
						return
					case <-querySeen:
					}
					switch mode {
					case "cancelled":
						cancel()
					case "revoked":
						authority.mu.Lock()
						authority.revoked = true
						authority.mu.Unlock()
					case "helper_lost":
						_ = process.Process.Kill()
					}
				}()
			}
			token := strings.Repeat("t", 48)
			guarded, err := resourceexec.NewRuntime(backend, authority, token)
			if err != nil {
				t.Fatal(err)
			}
			args := browserspa.Args{URL: f.URL(), Filter: &browserspa.Filter{Selector: "input[name=category]", Value: "books"}, SettleMS: 350}
			if mode == "infinite" {
				args.ReadySelector = "#result[data-ready=true]"
			}
			raw, _ := json.Marshal(args)
			call := &toolbrokerv1.ToolCall{RunId: "run-spa", ToolCallId: "call-spa", ToolName: browserspa.Tool, ArgsJson: string(raw)}
			req := &runtimev1.ExecuteRequest{ToolCall: call, Contract: browserspa.Contract(), Context: &runtimev1.ExecutionContext{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, RuntimeTarget: browserspa.Contract().RuntimeTarget, ActionId: "spa-effect", ActionFingerprint: strings.Repeat("f", 64)}}
			ctx := metadata.NewIncomingContext(owned, metadata.Pairs("authorization", "Bearer "+token, "butler-resource-ticket", strings.Repeat("1", 48)))
			response, err := guarded.Execute(ctx, req)
			if err != nil {
				t.Fatal("guarded runtime transport failed")
			}
			mu.Lock()
			counts := map[string]int{}
			filtered := 0
			for _, e := range events {
				counts[e.Kind]++
				if e.Kind == "query" && e.Accepted && e.Category == "books" {
					filtered++
				}
			}
			mu.Unlock()
			result := response.GetResult()
			if mode != "normal" && mode != "cookie_reflection" {
				if result.GetStatus() == "completed" || counts["forbidden_effect"] != 0 || counts["ws_client_message"] != 0 {
					t.Fatalf("hostile fixture escaped: mode=%s status=%s counts=%v", mode, result.GetStatus(), counts)
				}
				if mode == "lost_query" && counts["query"] != 1 {
					t.Fatal("lost query was retried")
				}
				if (mode == "cancelled" || mode == "revoked" || mode == "helper_lost") && counts["query"] != 1 {
					t.Fatal("fault duplicated or skipped accepted query")
				}
				return
			}
			if result.GetStatus() != "completed" {
				t.Fatalf("SPA runtime failed: status=%s code=%s", result.GetStatus(), result.GetError().GetDetailsJson())
			}
			var observed Result
			if json.Unmarshal([]byte(result.GetResultJson()), &observed) != nil || observed.SPAProfile != browserspa.Profile || observed.NetworkState != "frozen" || observed.SPANonce != "" || observed.SessionID != "" || !strings.Contains(observed.Text, "stock:61") || !strings.Contains(observed.Text, "worker: true") {
				t.Fatal("invalid bounded observation")
			}
			if _, err := browserspa.ReadResultURL(result.GetResultJson(), args); err != nil {
				t.Fatal("invalid typed read proof")
			}
			if mode == "normal" && !strings.Contains(observed.Text, title) {
				t.Fatal("filtered result missing")
			}
			if mode == "cookie_reflection" && (strings.Contains(result.GetResultJson(), title) || !strings.Contains(observed.Text, "[REDACTED]")) {
				t.Fatal("cookie reflected into model observation")
			}
			if counts["query"] != 2 || filtered != 1 || counts["worker_script"] < 2 || counts["cache_origin"] != 1 || counts["ws_client_message"] != 0 {
				t.Fatalf("independent recipient mismatch: %v", counts)
			}
			authority.mu.Lock()
			finished := authority.finished
			targetCount := len(authority.targets)
			authority.mu.Unlock()
			if !finished || targetCount < 10 {
				t.Fatal("live authority/finish was bypassed")
			}
		})
	}
}
