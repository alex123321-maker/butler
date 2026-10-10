package openaicodex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestSerializedCodexInputIsBoundedBeforeAuthOrHTTP(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	provider, _ := NewProvider(Config{BaseURL: server.URL, AuthSource: staticAuth{}}, server.Client())
	if _, err := provider.executeStreamingRequest(transport.WithInputByteLimit(context.Background(), 8), "run", "session", []byte("123456789")); err == nil {
		t.Fatal("oversized request accepted")
	}
	if calls.Load() != 0 {
		t.Fatal("byte denial sent HTTP request")
	}
}
