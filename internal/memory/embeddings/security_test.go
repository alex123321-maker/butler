package embeddings

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

func TestEmbeddingCannotExceedInputOrReflectAuthErrorOrFollowRedirect(t *testing.T) {
	var calls, redirects atomic.Int32
	secret := "synthetic-private-embedding-key"
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirects.Add(1) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if strings.HasPrefix(r.URL.Path, "/redirect/") {
			http.Redirect(w, r, destination.URL, 302)
			return
		}
		w.WriteHeader(400)
		_, _ = fmt.Fprint(w, secret)
	}))
	defer server.Close()
	provider, _ := NewProvider(Config{APIKey: secret, BaseURL: server.URL}, server.Client())
	if _, err := provider.EmbedQuery(transport.WithInputByteLimit(context.Background(), 8), "private-observation"); err == nil || calls.Load() != 0 {
		t.Fatal("embedding cap did not stop request")
	}
	if _, err := provider.EmbedQuery(context.Background(), "private-observation"); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatal("provider error retained secret")
	}
	provider, _ = NewProvider(Config{APIKey: secret, BaseURL: server.URL + "/redirect"}, server.Client())
	if _, err := provider.EmbedQuery(context.Background(), "private-observation"); err == nil || redirects.Load() != 0 {
		t.Fatal("embedding followed foreign redirect")
	}
}
