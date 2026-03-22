package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/butler/butler/apps/browser-bridge/internal/protocol"
)

func TestAppRunHandlesPingAndBindRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v2/single-tab/bind-requests" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"approval":{"approval_id":"approval-9"}}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	t.Setenv("BUTLER_BROWSER_BRIDGE_ORCHESTRATOR_URL", server.URL)

	var input bytes.Buffer
	writeRequestFrame(t, &input, protocol.Request{ID: "1", Method: "ping"})
	writeRequestFrame(t, &input, protocol.Request{
		ID:     "2",
		Method: "bind.request",
		Params: map[string]any{
			"run_id":      "run-1",
			"session_key": "telegram:chat:1",
			"tab_candidates": []map[string]any{{
				"internal_tab_ref": "browser-a:1",
				"title":            "Inbox",
				"current_url":      "https://mail.example.com",
			}},
		},
	})

	var output bytes.Buffer
	application, err := New(context.Background(), &input, &output, os.Stderr)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := application.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	first := readResponseFrame(t, &output)
	if !first.OK {
		t.Fatalf("expected first response to be ok: %+v", first)
	}
	second := readResponseFrame(t, &output)
	if !second.OK {
		t.Fatalf("expected second response to be ok: %+v", second)
	}
}

func TestAppControlDispatchAction(t *testing.T) {
	t.Setenv("BUTLER_BROWSER_BRIDGE_ORCHESTRATOR_URL", "http://127.0.0.1:18080")
	t.Setenv("BUTLER_BROWSER_BRIDGE_CONTROL_ADDR", "127.0.0.1:29125")

	inputReader, inputWriter := io.Pipe()
	defer inputWriter.Close()
	var output bytes.Buffer

	application, err := New(context.Background(), inputReader, &output, os.Stderr)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- application.Run(context.Background())
	}()

	waitForHealth(t, "http://127.0.0.1:29125/health")

	responseDone := make(chan struct{})
	go func() {
		defer close(responseDone)
		request := readNativeRequestFrame(t, &output)
		if request.Method != "action.dispatch" {
			t.Errorf("expected action.dispatch request, got %q", request.Method)
			return
		}
		dispatchResponse := protocol.Response{
			ID: request.ID,
			OK: true,
			Result: map[string]any{
				"single_tab_session_id": "single-tab-7",
				"session_status":        "ACTIVE",
				"result_json":           `{"ok":true}`,
				"current_url":           "https://example.com",
				"current_title":         "Example",
			},
		}
		writeFrameToPipe(t, inputWriter, dispatchResponse)
	}()

	reqBody := bytes.NewBufferString(`{"single_tab_session_id":"single-tab-7","bound_tab_ref":"17","action_type":"navigate","args_json":"{\"url\":\"https://example.com\"}"}`)
	resp, err := http.Post("http://127.0.0.1:29125/api/v1/actions/dispatch", "application/json", reqBody)
	if err != nil {
		t.Fatalf("POST dispatch failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	<-responseDone
	_ = inputWriter.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for app shutdown")
	}
}

func writeRequestFrame(t *testing.T, w *bytes.Buffer, request protocol.Request) {
	t.Helper()
	writeFrame(t, w, request)
}

func writeFrame(t *testing.T, w *bytes.Buffer, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	if err := binary.Write(w, binary.LittleEndian, uint32(len(payload))); err != nil {
		t.Fatalf("write frame length: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("write frame payload: %v", err)
	}
}

func writeFrameToPipe(t *testing.T, w *io.PipeWriter, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	var length bytes.Buffer
	if err := binary.Write(&length, binary.LittleEndian, uint32(len(payload))); err != nil {
		t.Fatalf("write frame length: %v", err)
	}
	if _, err := w.Write(length.Bytes()); err != nil {
		t.Fatalf("write frame length payload: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("write frame payload: %v", err)
	}
}

func readResponseFrame(t *testing.T, r *bytes.Buffer) protocol.Response {
	t.Helper()
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		t.Fatalf("read response length: %v", err)
	}
	payload := make([]byte, length)
	if _, err := r.Read(payload); err != nil {
		t.Fatalf("read response payload: %v", err)
	}
	var response protocol.Response
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return response
}

func readNativeRequestFrame(t *testing.T, r *bytes.Buffer) protocol.Request {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for r.Len() < 4 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		t.Fatalf("read request length: %v", err)
	}
	payload := make([]byte, length)
	if _, err := r.Read(payload); err != nil {
		t.Fatalf("read request payload: %v", err)
	}
	var request protocol.Request
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return request
}

func waitForHealth(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("health endpoint %s did not become ready", url)
}
