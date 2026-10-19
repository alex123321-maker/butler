package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/apps/tool-broker/internal/runtimeclient"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func webRuntimeProcess(t *testing.T, binary, harness, endpoint string) string {
	t.Helper()
	if binary == "" {
		t.Fatal("compiled real runtime test binary is required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	testName, flag, fixture := "TestWebFetchRuntimeHarness", "BUTLER_WEBFETCH_RUNTIME_HARNESS=1", "BUTLER_WEBFETCH_FIXTURE_ENDPOINT="
	if harness == "http" {
		testName, flag, fixture = "TestExperienceHTTPRuntimeHarness", "BUTLER_EXPERIENCE_HTTP_HARNESS=1", "BUTLER_EXPERIENCE_RECIPIENT_ENDPOINT="
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	command := exec.CommandContext(ctx, binary, "-test.run=^"+testName+"$")
	command.Env = append(os.Environ(), flag, fixture+endpoint, "BUTLER_GRPC_ADDR="+address)
	command.Stdout, command.Stderr = io.Discard, io.Discard
	if err := command.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = command.Wait() })
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			connection.Close()
			return address
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("real runtime subprocess did not start")
	return ""
}

// Two gRPC boundaries: private Broker RPC -> real runtime subprocess RPC. The
// actual registry schemas, PG grants/journal/budgets and exact approval binding
// are used. Only owner decisions and the public-DNS recipient are fixtures.
func TestWebToolsBrokerSixDefectsIntegration(t *testing.T) {
	if os.Getenv("BUTLER_WEB_TOOLS_BROKER_E2E") != "1" {
		t.Skip("dedicated PostgreSQL and compiled runtime subprocesses required")
	}
	if os.Getenv("BUTLER_TEST_ACTIONAUTH_POSTGRES_URL") == "" {
		t.Fatal("isolated PostgreSQL is mandatory for this acceptance")
	}
	pool := resourceBrokerDB(t)
	var requests atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		switch r.URL.Path {
		case "/article":
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<html><head><style>STYLE_NOISE</style></head><body><h1>Useful article</h1><script>SCRIPT_NOISE</script></body></html>`)
		case "/missing", "/limited", "/server-error", "/redirect":
			w.Header().Set("Content-Type", "text/plain")
			status := map[string]int{"/missing": 404, "/limited": 429, "/server-error": 500, "/redirect": 302}[r.URL.Path]
			w.WriteHeader(status)
			fmt.Fprint(w, "private failure body")
		case "/large":
			w.Header().Set("Content-Type", "text/plain")
			fmt.Fprint(w, strings.Repeat("x", 2*1024*1024+128))
		case "/binary":
			w.Header().Set("Content-Type", "application/pdf")
			fmt.Fprint(w, "%PDF-1.7\n\x00\xff")
		default:
			t.Error("unexpected recipient path")
			w.WriteHeader(500)
		}
	}))
	defer site.Close()
	httpTarget := webRuntimeProcess(t, os.Getenv("BUTLER_WEB_TOOLS_HTTP_BINARY"), "http", site.URL)
	webTarget := webRuntimeProcess(t, os.Getenv("BUTLER_WEB_TOOLS_WEBFETCH_BINARY"), "webfetch", site.URL)
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "configs", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, tool := range catalog.Tools {
		if tool["tool_class"] == "http" {
			tool["runtime_target"] = httpTarget
		}
		if tool["tool_class"] == "webfetch" {
			tool["runtime_target"] = webTarget
		}
	}
	data, _ = json.Marshal(catalog)
	path := filepath.Join(t.TempDir(), "tools.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.Load(path, "")
	if err != nil {
		t.Fatal(err)
	}
	router := runtimeclient.New(nil)
	defer router.Close()
	server := NewServer(reg, router, nil, nil)
	policy := budget.DefaultPolicy()
	controller := budget.NewController(pool, policy)
	store := actionauth.NewPostgresStore(pool)
	store.SetBudgets(controller)
	reader := resourceReader{authFixture: newAuthFixture(), pool: pool}
	server.SetBudgets(controller)
	server.SetActionAuthorizer(actionauth.NewService(store, reader, nil))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	rpc := grpc.NewServer()
	toolbrokerv1.RegisterToolBrokerServiceServer(rpc, server)
	go func() { _ = rpc.Serve(listener) }()
	defer rpc.Stop()
	connection, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	client := toolbrokerv1.NewToolBrokerServiceClient(connection)
	sequence := 0
	invoke := func(t *testing.T, name string, args map[string]any) (*toolbrokerv1.ToolResult, map[string]any) {
		t.Helper()
		sequence++
		encoded, _ := json.Marshal(args)
		call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: fmt.Sprintf("web-%d", sequence), ToolName: name, ArgsJson: string(encoded), AutonomyMode: 3}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		prepared, err := client.PrepareToolCall(ctx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call})
		if err != nil || prepared.GetError() != nil {
			t.Fatalf("prepare failed: %v %v", err, prepared.GetError())
		}
		intent, err := store.GetIntent(ctx, prepared.GetAction().GetActionId())
		if err != nil {
			t.Fatal(err)
		}
		id := "approval-" + call.ToolCallId
		reader.mu.Lock()
		reader.approvals[id] = actionauth.ApprovalSnapshot{ApprovalID: id, Status: "approved", Binding: intent.Binding, ResolvedAt: time.Now(), ExpiresAt: time.Now().Add(time.Hour)}
		reader.mu.Unlock()
		grant, err := client.AuthorizeToolCall(ctx, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: intent.Binding.ActionID, ApprovalId: id})
		if err != nil || grant.GetError() != nil {
			t.Fatalf("authorize failed: %v %v", err, grant.GetError())
		}
		response, err := client.ExecuteToolCall(ctx, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: grant.GetAuthorizationId()})
		if err != nil {
			t.Fatal(err)
		}
		result := response.GetResult()
		if result.GetActionId() == "" || len(result.GetEvidenceRefs()) != 1 {
			t.Fatal("durable Broker action/evidence missing")
		}
		recorded, err := store.GetEffect(ctx, result.GetActionId())
		if err != nil || string(recorded.State) != result.GetEffectState() {
			t.Fatal("journal and response disagree", err)
		}
		payload := map[string]any{}
		if result.GetResultJson() != "" {
			if err := json.Unmarshal([]byte(result.GetResultJson()), &payload); err != nil {
				t.Fatal(err)
			}
		}
		return result, payload
	}
	accept := func(t *testing.T, r *toolbrokerv1.ToolResult) {
		t.Helper()
		if r.GetStatus() != "completed" || r.GetEffectState() != string(actionauth.StateConfirmed) {
			t.Fatalf("read did not survive Broker schema/journal: %s %v", r.GetStatus(), r.GetError())
		}
	}
	refuse := func(t *testing.T, r *toolbrokerv1.ToolResult, reason string) {
		t.Helper()
		if r.GetStatus() != "failed" || r.GetError() == nil || r.GetError().GetRetryable() || r.GetEffectState() != string(actionauth.StateUnknown) || !strings.Contains(r.GetError().GetDetailsJson(), reason) {
			t.Fatalf("explicit read diagnostic lost: %v", r)
		}
	}
	t.Run("no dispatch without authorization", func(t *testing.T) {
		before := requests.Load()
		r, err := client.ExecuteToolCall(context.Background(), &toolbrokerv1.ExecuteToolCallRequest{ToolCall: &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "forged", ToolName: "web.fetch", ArgsJson: `{"url":"http://external.test/article"}`, AutonomyMode: 3}})
		if err != nil || r.GetResult().GetStatus() != "failed" || requests.Load() != before {
			t.Fatal("unapproved runtime dispatch")
		}
	})
	t.Run("script and style excluded", func(t *testing.T) {
		r, p := invoke(t, "web.fetch", map[string]any{"url": egresstest.Origin + "/article"})
		accept(t, r)
		if p["content_text"] != "Useful article" {
			t.Fatal("non-readable content returned", p["content_text"])
		}
	})
	t.Run("HTTP status failures preserved", func(t *testing.T) {
		for _, path := range []string{"/missing", "/limited", "/server-error", "/redirect"} {
			r, _ := invoke(t, "web.extract", map[string]any{"url": egresstest.Origin + path})
			refuse(t, r, "http_status")
			var d map[string]any
			_ = json.Unmarshal([]byte(r.GetError().GetDetailsJson()), &d)
			want := map[string]int{"/missing": 404, "/limited": 429, "/server-error": 500, "/redirect": 302}[path]
			if d["status_code"] != float64(want) || strings.Contains(r.String(), "private failure body") {
				t.Fatal("HTTP status lost or body leaked")
			}
		}
	})
	t.Run("truncation survives fetch and extract", func(t *testing.T) {
		for _, name := range []string{"web.fetch", "web.extract"} {
			r, p := invoke(t, name, map[string]any{"url": egresstest.Origin + "/large"})
			accept(t, r)
			metadata, _ := p["metadata"].(map[string]any)
			text, _ := p["content_text"].(string)
			if metadata["truncated"] != true || len(text) != 2*1024*1024 {
				t.Fatalf("incomplete result not marked: %v bytes=%d", metadata, len(text))
			}
		}
	})
	t.Run("binary MIME refused", func(t *testing.T) {
		r, _ := invoke(t, "web.fetch", map[string]any{"url": egresstest.Origin + "/binary"})
		refuse(t, r, "unsupported_content")
		if r.GetResultJson() != "" {
			t.Fatal("binary body published")
		}
	})
	t.Run("unsupported selector refused before request", func(t *testing.T) {
		before := requests.Load()
		r, _ := invoke(t, "http.parse_html", map[string]any{"url": egresstest.Origin + "/article", "selector": "article h1"})
		if r.GetStatus() != "failed" || !strings.Contains(r.GetError().GetMessage(), "unsupported selector") || requests.Load() != before {
			t.Fatal("unsupported selector masked or fetched")
		}
	})
	t.Run("no match is valid empty array", func(t *testing.T) {
		r, p := invoke(t, "http.parse_html", map[string]any{"content": "<h1>Present</h1>", "selector": ".absent"})
		accept(t, r)
		matches, ok := p["matches"].([]any)
		if !ok || len(matches) != 0 || p["count"] != float64(0) {
			t.Fatal("empty array lost or schema rejected", p)
		}
	})
	t.Run("positive parser and extract controls", func(t *testing.T) {
		r, p := invoke(t, "http.parse_html", map[string]any{"content": "<h1 class=title>Present</h1>", "selector": "h1.title"})
		accept(t, r)
		if p["count"] != float64(1) {
			t.Fatal("supported parser regressed")
		}
		r, p = invoke(t, "web.extract", map[string]any{"url": egresstest.Origin + "/article"})
		accept(t, r)
		if p["status_code"] != float64(200) || p["content_text"] != "Useful article" || p["provider"] != "plain_http_fallback" {
			t.Fatal("extract provenance lost")
		}
	})
	var reservations int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM execution_budget_reservations`).Scan(&reservations); err != nil || reservations != sequence {
		t.Fatalf("not every dispatch reserved a budget: %d/%d %v", reservations, sequence, err)
	}
}
