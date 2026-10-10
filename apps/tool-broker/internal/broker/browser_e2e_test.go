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
	"testing"
	"time"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/apps/tool-broker/internal/runtimeclient"
	"github.com/butler/butler/internal/credentials"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
)

type browserCredentialMetadata struct{}

func (browserCredentialMetadata) GetByAlias(_ context.Context, alias string) (credentials.Record, error) {
	if alias != "browser-fixture" {
		return credentials.Record{}, credentials.ErrRecordNotFound
	}
	return credentials.Record{Alias: alias, Status: credentials.StatusActive, AllowedDomains: []string{"external.test"}, AllowedTools: []string{"browser.fill"}, ApprovalPolicy: credentials.ApprovalPolicyAlwaysConfirm, SecretRef: "fixture://browser-password"}, nil
}

type browserFixtureSecret struct{}

func (browserFixtureSecret) ResolveSecretRef(context.Context, string) (string, error) {
	return "browser-private-password", nil
}

func TestBrokerBrowserSequenceE2E(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("real browser e2e requires isolated lab image")
	}
	binary := os.Getenv("BUTLER_BROWSER_E2E_BINARY")
	if binary == "" {
		t.Fatal("BUTLER_BROWSER_E2E_BINARY is required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/submit" {
			_ = r.ParseForm()
			_, _ = fmt.Fprintf(w, "<body>%s|%s</body>", r.Form.Get("a"), r.Form.Get("b"))
			return
		}
		_, _ = fmt.Fprint(w, `<body><form action="/submit" method="post"><input name="a" id="a"><input name="b" id="b"><input type="password" id="password"><button id="submit">Send</button></form></body>`)
	}))
	defer site.Close()
	cmd := exec.Command(binary, "-test.run=^TestBrowserRuntimeEgressHarness$")
	cmd.Env = append(os.Environ(), "BUTLER_GRPC_ADDR="+addr, "BUTLER_BROWSER_EGRESS_HARNESS=1", "BUTLER_BROWSER_FIXTURE_ENDPOINT="+site.URL)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Signal(os.Interrupt); _ = cmd.Wait() })
	deadline := time.Now().Add(10 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("browser runtime did not start")
		}
		time.Sleep(25 * time.Millisecond)
	}
	var catalog struct {
		Tools []map[string]any `json:"tools"`
	}
	payload, err := os.ReadFile(filepath.Join("configs", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, tool := range catalog.Tools {
		if tool["tool_class"] == "browser" {
			tool["runtime_target"] = addr
		}
	}
	payload, _ = json.Marshal(catalog)
	path := filepath.Join(t.TempDir(), "tools.json")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.Load(path, addr)
	if err != nil {
		t.Fatal(err)
	}
	router := runtimeclient.New(nil)
	defer router.Close()
	// Use the real Credential Broker policy and resolver boundary. This fixture
	// intentionally requires approval and is denied before reading the secret.
	credentialBroker := credentials.NewToolCallBroker(credentials.NewBroker(browserCredentialMetadata{}), browserFixtureSecret{})
	server := NewServer(reg, router, credentialBroker, nil)
	fixture := setTestAuthority(server, credentialBroker)
	sequence := 0
	invoke := func(tool string, args map[string]any, refs ...*toolbrokerv1.CredentialRef) *toolbrokerv1.ToolResult {
		t.Helper()
		payload, _ := json.Marshal(args)
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		_ = ctx
		sequence++
		return dispatchForTest(t, server, fixture, &toolbrokerv1.ToolCall{RunId: "broker-run", ToolCallId: fmt.Sprintf("broker-call-%d", sequence), ToolName: tool, ArgsJson: string(payload), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, CredentialRefs: refs}, len(refs) == 0)
	}
	var observation map[string]any
	accept := func(result *toolbrokerv1.ToolResult) {
		t.Helper()
		if result.GetStatus() != "completed" && (result.GetStatus() != "effect_unknown" || result.GetEffectState() != "effect_unknown" || result.GetActionId() == "" || result.GetError().GetRetryable()) {
			t.Fatalf("broker sequence failed: %+v", result)
		}
		if err := json.Unmarshal([]byte(result.GetResultJson()), &observation); err != nil {
			t.Fatal(err)
		}
	}
	args := func(selector string) map[string]any {
		return map[string]any{"browser_session_id": observation["browser_session_id"], "observation_version": observation["observation_version"], "expected_url": observation["final_url"], "selector": selector}
	}
	accept(invoke("browser.open", map[string]any{"url": egresstest.Origin + "/form"}))
	secretArgs := args("#password")
	secretArgs["value"] = map[string]string{"type": "credential_ref", "alias": "browser-fixture", "field": "password"}
	denied := invoke("browser.fill", secretArgs, &toolbrokerv1.CredentialRef{Type: "credential_ref", Alias: "browser-fixture", Field: "password"})
	if denied.GetStatus() != "failed" || !strings.Contains(denied.GetError().GetMessage(), "requires approval") {
		t.Fatal("credential approval was bypassed")
	}
	for _, field := range []string{"a", "b"} {
		request := args("#" + field)
		request["value"] = "value-" + field
		accept(invoke("browser.fill", request))
	}
	accept(invoke("browser.snapshot", map[string]any{"browser_session_id": observation["browser_session_id"]}))
	accept(invoke("browser.click", args("#submit")))
	if observation["text"] != "value-a|value-b" {
		t.Fatalf("Broker/runtime sequence lost state: %v", observation["text"])
	}
	accept(invoke("browser.close", map[string]any{"browser_session_id": observation["browser_session_id"]}))
}
