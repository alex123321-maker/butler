package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"testing"
)

func TestReadRequestAndWriteResponse(t *testing.T) {
	t.Parallel()

	requestPayload := []byte(`{"id":"req-1","method":"ping"}`)
	var input bytes.Buffer
	if err := binary.Write(&input, binary.LittleEndian, uint32(len(requestPayload))); err != nil {
		t.Fatalf("write request frame length: %v", err)
	}
	if _, err := input.Write(requestPayload); err != nil {
		t.Fatalf("write request frame payload: %v", err)
	}

	request, err := ReadRequest(&input)
	if err != nil {
		t.Fatalf("ReadRequest returned error: %v", err)
	}
	if request.Method != "ping" {
		t.Fatalf("expected ping method, got %q", request.Method)
	}

	var output bytes.Buffer
	if err := WriteResponse(&output, Response{ID: "req-1", OK: true, Result: map[string]any{"status": "ok"}}); err != nil {
		t.Fatalf("WriteResponse returned error: %v", err)
	}
	var length uint32
	if err := binary.Read(&output, binary.LittleEndian, &length); err != nil {
		t.Fatalf("read response frame length: %v", err)
	}
	payload := make([]byte, length)
	if _, err := output.Read(payload); err != nil {
		t.Fatalf("read response frame payload: %v", err)
	}
	var response Response
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("decode response payload: %v", err)
	}
	if !response.OK {
		t.Fatal("expected ok response")
	}
}

func TestReadMessageAndWriteRequest(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	if err := WriteRequest(&output, Request{ID: "req-2", Method: "action.dispatch", Params: map[string]any{"bound_tab_ref": "12"}}); err != nil {
		t.Fatalf("WriteRequest returned error: %v", err)
	}
	message, err := ReadMessage(&output)
	if err != nil {
		t.Fatalf("ReadMessage returned error: %v", err)
	}
	request, ok := message.(Request)
	if !ok {
		t.Fatalf("expected Request payload, got %T", message)
	}
	if request.Method != "action.dispatch" {
		t.Fatalf("expected action.dispatch method, got %q", request.Method)
	}
}
