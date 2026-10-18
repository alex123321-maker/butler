package app

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/singletabcontrol"
)

func guardedRequestBrokerProcess(t *testing.T, f *dispatchAdmissionFixture, service *GuardedTabRequests) *tools.BrokerClient {
	t.Helper()
	binary := os.Getenv("BUTLER_GUARDED_REQUEST_BROKER_BINARY")
	if binary == "" {
		t.Skip("compiled private Broker process required")
	}
	const token = "guarded-request-private-process-fixture-token"
	h, err := singletabcontrol.NewRequestHandler(token, service)
	if err != nil {
		t.Fatal(err)
	}
	return controlBrokerProcess(t, f, h, "/request", "BUTLER_GUARDED_TAB_REQUEST_HARNESS=1")
}
func controlBrokerProcess(t *testing.T, f *dispatchAdmissionFixture, h http.Handler, path, harness string) *tools.BrokerClient {
	t.Helper()
	binary := os.Getenv("BUTLER_GUARDED_REQUEST_BROKER_BINARY")
	if binary == "" {
		t.Skip("compiled private Broker process required")
	}
	const token = "guarded-request-private-process-fixture-token"
	mux := http.NewServeMux()
	mux.Handle("/context", actionauth.NewContextHandler(token, f.reader))
	mux.Handle(path, h)
	control := httptest.NewServer(mux)
	t.Cleanup(control.Close)
	tokenPath := filepath.Join(t.TempDir(), "token")
	if os.WriteFile(tokenPath, []byte(token), 0600) != nil {
		t.Fatal("private fixture token unavailable")
	}
	dsn, err := url.Parse(f.f.pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal("fixture storage unavailable")
	}
	query := dsn.Query()
	query.Set("search_path", f.f.pool.Config().ConnConfig.RuntimeParams["search_path"])
	dsn.RawQuery = query.Encode()
	address := freeAddress(t)
	cmd := exec.Command(binary, "-test.run=^TestDelegationBrokerHarness$", "-test.timeout=50s")
	cmd.Env = append(os.Environ(), "BUTLER_DELEGATION_BROKER_HARNESS=1", harness, "BUTLER_DELEGATION_POSTGRES_URL="+dsn.String(), "BUTLER_DELEGATION_TOKEN_FILE="+tokenPath, "BUTLER_DELEGATION_CONTEXT_URL="+control.URL+"/context", "BUTLER_DELEGATION_ADMISSION_URL="+control.URL+path, "BUTLER_DELEGATION_GRPC_ADDRESS="+address, "BUTLER_DELEGATION_MANIFEST_FILE="+f.manifestPath)
	if harness == "BUTLER_MEMORY_CANDIDATE_HARNESS=1" {
		actual := os.Getenv("BUTLER_MEMORY_CANDIDATE_APP_BINARY")
		if actual == "" {
			t.Skip("compiled actual Broker application required")
		}
		registryPath := filepath.Join(t.TempDir(), "tools.json")
		if os.WriteFile(registryPath, []byte(`{"tools":[]}`), 0600) != nil {
			t.Fatal("empty fixture registry unavailable")
		}
		cmd = exec.Command(actual)
		cmd.Env = append(os.Environ(), "BUTLER_POSTGRES_URL="+dsn.String(), "BUTLER_POSTGRES_MAX_CONNS=2", "BUTLER_POSTGRES_MIN_CONNS=0", "BUTLER_TOOL_REGISTRY_PATH="+registryPath, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+tokenPath, "BUTLER_ACTION_CONTEXT_URL="+control.URL+"/context", "BUTLER_MEMORY_CANDIDATE_ENABLED=true", "BUTLER_MEMORY_CANDIDATE_CONTROL_URL="+control.URL+path, "BUTLER_HTTP_ADDR=127.0.0.1:0", "BUTLER_GRPC_ADDR="+address, "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+f.manifestPath)
	}
	if strings.HasPrefix(harness, "BUTLER_MEMORY_READ_HARNESS=") {
		actual := os.Getenv("BUTLER_MEMORY_READ_APP_BINARY")
		if actual == "" {
			t.Skip("compiled actual Broker application required")
		}
		registryPath := filepath.Join(t.TempDir(), "tools.json")
		if os.WriteFile(registryPath, []byte(`{"tools":[]}`), 0600) != nil {
			t.Fatal("empty fixture registry unavailable")
		}
		cmd = exec.Command(actual)
		cmd.Env = append(os.Environ(), "BUTLER_POSTGRES_URL="+dsn.String(), "BUTLER_POSTGRES_MAX_CONNS=2", "BUTLER_POSTGRES_MIN_CONNS=0", "BUTLER_TOOL_REGISTRY_PATH="+registryPath, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+tokenPath, "BUTLER_ACTION_CONTEXT_URL="+control.URL+"/context", "BUTLER_MEMORY_READ_ENABLED=true", "BUTLER_MEMORY_READ_CONTROL_URL="+control.URL+path, "BUTLER_HTTP_ADDR=127.0.0.1:0", "BUTLER_GRPC_ADDR="+address, "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+f.manifestPath)
	}
	if harness == "BUTLER_SCHEDULES_HARNESS=1" {
		actual := os.Getenv("BUTLER_SCHEDULES_APP_BINARY")
		if actual == "" {
			t.Fatal("compiled actual scheduling Broker application required")
		}
		registryPath := filepath.Join(t.TempDir(), "tools.json")
		if os.WriteFile(registryPath, []byte(`{"tools":[]}`), 0600) != nil {
			t.Fatal("fixture registry unavailable")
		}
		cmd = exec.Command(actual)
		cmd.Env = append(os.Environ(), "BUTLER_POSTGRES_URL="+dsn.String(), "BUTLER_POSTGRES_MAX_CONNS=2", "BUTLER_POSTGRES_MIN_CONNS=0", "BUTLER_TOOL_REGISTRY_PATH="+registryPath, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+tokenPath, "BUTLER_ACTION_CONTEXT_URL="+control.URL+"/context", "BUTLER_SCHEDULES_ENABLED=true", "BUTLER_SCHEDULES_CONTROL_URL="+control.URL+path, "BUTLER_HTTP_ADDR=127.0.0.1:0", "BUTLER_GRPC_ADDR="+address, "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+f.manifestPath)
	}

	if harness == "BUTLER_TOOL_DISCOVERY_HARNESS=1" {
		actual := os.Getenv("BUTLER_TOOL_DISCOVERY_APP_BINARY")
		if actual == "" {
			t.Fatal("compiled discovery Broker required")
		}
		registryPath := filepath.Join(t.TempDir(), "tools.json")
		registry := `{"tools":[{"tool_name":"doctor.check_system","tool_class":"doctor","description":"Inspect system health","status":"active","input_schema_json":"{\"type\":\"object\"}","output_schema_json":"{\"type\":\"object\"}","runtime_target":"unused:9090","contract_revision":"1","effect_contract":{"retry_safety":"read_only","verification_method":"read_result"}}]}`
		if target := os.Getenv("BUTLER_TOOL_DISCOVERY_RUNTIME_TARGET"); target != "" {
			registry = strings.Replace(registry, "unused:9090", target, 1)
		}
		if err := os.WriteFile(registryPath, []byte(registry), 0600); err != nil {
			t.Fatal(err)
		}
		cmd = exec.Command(actual)
		cmd.Env = append(os.Environ(), "BUTLER_POSTGRES_URL="+dsn.String(), "BUTLER_POSTGRES_MAX_CONNS=2", "BUTLER_POSTGRES_MIN_CONNS=0", "BUTLER_TOOL_REGISTRY_PATH="+registryPath, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+tokenPath, "BUTLER_ACTION_CONTEXT_URL="+control.URL+"/context", "BUTLER_TOOL_DISCOVERY_ENABLED=true", "BUTLER_HTTP_ADDR=127.0.0.1:0", "BUTLER_GRPC_ADDR="+address, "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+f.manifestPath)
	}
	log, err := os.OpenFile(filepath.Join(t.TempDir(), "broker.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	cmd.Stdout, cmd.Stderr = log, log
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var processErr error
	go func() { processErr = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
			if processErr != nil {
				t.Error("private Broker failed", processErr)
			}
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			t.Error("private Broker did not stop")
		}
		if t.Failed() {
			raw, _ := os.ReadFile(log.Name())
			if len(raw) > 8192 {
				raw = raw[len(raw)-8192:]
			}
			t.Log(strings.ReplaceAll(string(raw), token, "[private fixture token]"))
		}
	})
	ready := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		conn, err := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			ready = true
			break
		}
		select {
		case <-done:
			t.Fatal("Broker stopped before ready", processErr)
		case <-time.After(25 * time.Millisecond):
		}
	}
	if !ready {
		t.Fatal("private Broker unavailable")
	}
	client, err := tools.DialWithTokenFile(address, tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	anonymous, err := tools.Dial(address)
	if err != nil {
		t.Fatal(err)
	}
	defer anonymous.Close()
	if _, err = anonymous.ExecuteAuthorizedToolCall(f.f.ctx, f.call, f.grant.ID); err == nil {
		t.Fatal("anonymous runtime used Broker request route")
	}
	return client
}
