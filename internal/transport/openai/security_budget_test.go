package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestHTTPAndResponsesStopBeforeSendingExceededInput(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	provider, _ := NewProvider(Config{BaseURL: server.URL, RealtimeURL: server.URL, TransportMode: TransportModeWSFirst}, server.Client())
	ctx := transport.WithInputByteLimit(context.Background(), 8)
	if _, err := provider.executeStreamingRequest(ctx, "run", "POST", server.URL, []byte("123456789")); err == nil {
		t.Fatal("oversized HTTP request accepted")
	}
	if _, _, err := provider.executeResponsesWS(ctx, "run", []byte(`{"input":"123456789"}`)); err == nil {
		t.Fatal("oversized realtime sequence accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("byte denial sent request or WebSocket handshake")
	}
}
