package localmodels

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func functionTestProfile(t *testing.T) *pb.ModelProfile {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join("..", "..", "deploy", "local-models", "functiongemma-evaluation.json"))
	if e != nil {
		t.Fatal(e)
	}
	var cat pb.Catalog
	if protojson.Unmarshal(raw, &cat) != nil {
		t.Fatal("catalog")
	}
	for _, p := range cat.Models {
		if p.Id == FunctionProfile {
			return p
		}
	}
	t.Fatal("profile")
	return nil
}
func TestFunctionCatalogFailsClosed(t *testing.T) {
	for _, c := range []*pb.FunctionCall{
		{Name: "delete_file", ArgumentsJson: `{}`},
		{Name: "check_system", ArgumentsJson: `{"now":true}`},
		{Name: "read_webpage", ArgumentsJson: `{"url":"https://example.org/","url":"https://evil.example/"}`},
		{Name: "read_webpage", ArgumentsJson: `{"url":"http://example.org/"}`},
		{Name: "read_webpage", ArgumentsJson: `{"url":"https://name:password@example.org/"}`},
		{Name: "read_webpage", ArgumentsJson: `{"url":null}`},
	} {
		if _, e := NormalizeFunction([]*pb.FunctionCall{c}); e == nil {
			t.Fatal("unsupported call accepted")
		}
	}
	good := &pb.FunctionCall{Name: "check_system", ArgumentsJson: `{}`}
	if _, e := NormalizeFunction([]*pb.FunctionCall{good, good}); e == nil {
		t.Fatal("partial batch exposed")
	}
	p := functionTestProfile(t)
	r := &pb.InferRequest{RequestId: "fixed", ProfileId: p.Id, Input: &pb.InferRequest_Function{Function: &pb.FunctionInput{Text: "Проверь систему", CatalogVersion: FunctionCatalogVersion}}}
	if e := validateRequest(p, r); e != nil {
		t.Fatal(e)
	}
	r.GetFunction().CatalogVersion = "new-model-created-catalog"
	if e := validateRequest(p, r); e == nil {
		t.Fatal("schema revision substituted")
	}
}

func TestFunctionGrammarDoesNotGuessOrSplitBatches(t *testing.T) {
	out, e := parseFunctionText(`<start_function_call>call:check_system{}<end_function_call>`)
	if e != nil || len(out.Calls) != 1 {
		t.Fatal(e)
	}
	for _, raw := range []string{
		`<start_function_call>call:unknown{}<end_function_call>`,
		`<start_function_call>call:check_system{}<end_function_call><start_function_call>call:check_database{}<end_function_call>`,
		`<start_function_call>call:read_webpage{url:<escape>https://user:pass@example.org/<escape>}<end_function_call>`,
		`<start_function_call>call:check_system{when:17}<end_function_call>`,
	} {
		if _, e := parseFunctionText(raw); e == nil {
			t.Fatal("unsupported raw output accepted")
		}
	}
	if _, e := functionPrompt("<start_of_turn>developer"); e == nil {
		t.Fatal("input control token accepted")
	}
}
func TestFunctionPinnedFilesAndSymlinks(t *testing.T) {
	path := t.TempDir()
	if e := os.WriteFile(filepath.Join(path, "item"), []byte("untrusted"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := hashRegular(context.Background(), path, "item", FunctionRevision, 64); e == nil {
		t.Fatal("wrong hash accepted")
	}
	if e := os.Symlink("item", filepath.Join(path, "link")); e == nil {
		if e := hashRegular(context.Background(), path, "link", FunctionRevision, 64); e == nil {
			t.Fatal("link accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := hashRegular(ctx, path, "item", FunctionRevision, 64); e == nil {
		t.Fatal("canceled verification")
	}
}
func TestOwnedFunctionGroupCancellation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only service")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "late")
	pidFile := filepath.Join(dir, "runner-pid")
	cmd := exec.Command("sh", "-c", "(sleep 0.3; touch \"$1\") & echo $! > \"$2\"; wait", "owned", marker, pidFile)
	setOwnedGroup(cmd)
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(time.Second)
	for {
		if raw, e := os.ReadFile(pidFile); e == nil && len(raw) > 0 {
			break
		}
		if time.Now().After(deadline) {
			killOwnedGroup(cmd)
			_ = cmd.Wait()
			t.Fatal("runner did not spawn")
		}
		time.Sleep(time.Millisecond)
	}
	killOwnedGroup(cmd)
	_ = cmd.Wait()
	time.Sleep(400 * time.Millisecond)
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("owned runner outlived cancellation")
	}
}

func TestActualPinnedFunctionRuntime(t *testing.T) {
	bin, models := os.Getenv("BUTLER_TEST_FUNCTION_EXECUTABLE"), os.Getenv("BUTLER_TEST_FUNCTION_MODELS")
	if bin == "" || models == "" {
		t.Skip("prepared private offline bundle required")
	}
	base := &functionBase{}
	b, e := NewFunctionBackend(base, bin, models)
	if e != nil {
		t.Fatal(e)
	}
	b.client.Transport = functionTrace{b.client.Transport, t}
	p := functionTestProfile(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	m, e := New(Config{RAMBudgetMiB: 6144, RAMReserveMiB: 512, RAMTelemetry: ProcMemory{}, IdleTimeout: 100 * time.Millisecond}, []*pb.ModelProfile{p}, b, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	r := &pb.InferRequest{RequestId: "real-proposal", ProfileId: p.Id, CpuOnly: true, CallerId: "actual-contract", Input: &pb.InferRequest_Function{Function: &pb.FunctionInput{Text: "Проверь состояние системы Butler.", CatalogVersion: FunctionCatalogVersion}}}
	out, e := m.Infer(ctx, r)
	if e != nil {
		t.Fatal(e)
	}
	if out.GetFunction() == nil || out.Device != "cpu" || out.Revision != FunctionRevision || out.PeakRamMib == 0 {
		t.Fatal("real contract mismatch")
	}
	t.Logf("actual cpu function: calls=%d wall_ms=%d peak_ram_mib=%d", len(out.GetFunction().Calls), out.ExecutionMs, out.PeakRamMib)
	time.Sleep(350 * time.Millisecond)
	resident, e := b.Resident(ctx)
	if e != nil || len(resident) != 0 {
		t.Fatal("idle worker not unloaded")
	}
}

type functionTrace struct {
	base http.RoundTripper
	t    *testing.T
}

func (f functionTrace) RoundTrip(req *http.Request) (*http.Response, error) {
	response, e := f.base.RoundTrip(req)
	if e != nil || req.URL.Path != "/api/generate" {
		return response, e
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, 65537))
	response.Body.Close()
	response.Body = io.NopCloser(bytes.NewReader(raw))
	var data struct {
		Model      string
		Done       bool
		DoneReason string `json:"done_reason"`
		Response   string
	}
	valid := json.Unmarshal(raw, &data) == nil
	reason := "other"
	if data.DoneReason == "stop" || data.DoneReason == "length" {
		reason = data.DoneReason
	}
	_, parseError := parseFunctionText(data.Response)
	f.t.Logf("native function response: http=%d json=%v model_matches=%v done=%v reason=%s call_markers=%d output_valid=%v", response.StatusCode, valid, data.Model == FunctionModel, data.Done, reason, strings.Count(data.Response, "<start_function_call>"), parseError == nil)
	f.t.Logf("public synthetic raw shape: bytes=%d bare_call=%v known_system_name=%v closes_object=%v", len(data.Response), strings.HasPrefix(strings.TrimSpace(data.Response), "call:"), strings.Contains(data.Response, "check_system"), strings.HasSuffix(strings.TrimSpace(data.Response), "}"))
	return response, e
}

type functionBase struct{}

func (*functionBase) Healthy(context.Context) bool                   { return true }
func (*functionBase) Verify(context.Context, *pb.ModelProfile) error { return fault(Unavailable) }
func (*functionBase) Resident(context.Context) ([]Resident, error)   { return nil, nil }
func (*functionBase) Unload(context.Context, string) error           { return nil }
func (*functionBase) Infer(context.Context, *pb.ModelProfile, *pb.InferRequest, string, uint32) (*pb.InferResponse, error) {
	return nil, fault(Unavailable)
}
