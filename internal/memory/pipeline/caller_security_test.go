package pipeline

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestAuxiliaryModelCapAndErrorDoNotLeakPrivateSource(t *testing.T) {
	var calls atomic.Int32
	secret := "synthetic-private-extractor-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(400)
		_, _ = fmt.Fprint(w, secret)
	}))
	defer server.Close()
	caller, _ := NewOpenAICaller(OpenAICallerConfig{APIKey: secret, BaseURL: server.URL}, server.Client())
	if _, err := caller.Complete(transport.WithInputByteLimit(context.Background(), 8), "kernel", "private-source"); err == nil || calls.Load() != 0 {
		t.Fatal("auxiliary call exceeded cap")
	}
	if _, err := caller.Complete(context.Background(), "kernel", "private-source"); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("auxiliary error leaked auth")
	}
	if _, err := parseExtractionResponse(secret + "not JSON"); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("malformed extraction leaked private output")
	}
}
