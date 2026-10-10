package runtime

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFixedSelfHostedServiceDoesNotFollowRedirectOrEchoErrors(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer target.Close()
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/?token=synthetic-private", 307)
	}))
	defer service.Close()
	provider := NewSelfHostedProvider(service.URL, service.Client())
	if _, err := provider.Fetch(context.Background(), "https://external.test", false); err == nil || strings.Contains(err.Error(), "synthetic-private") {
		t.Fatal("operator service redirect accepted or raw error leaked")
	}
	if redirected.Load() != 0 {
		t.Fatal("fixed service changed destination")
	}
}

func TestReturnedProviderMetadataCannotExposeSecretFields(t *testing.T) {
	result := sanitizeFetchResult(FetchResult{ContentText: "password=synthetic-private", Metadata: map[string]any{"Authorization": "Bearer synthetic-private", "cookie": "synthetic-cookie", "storage_state": "synthetic-state"}})
	text := fmt.Sprint(result)
	for _, secret := range []string{"synthetic-private", "synthetic-cookie", "synthetic-state"} {
		if strings.Contains(text, secret) {
			t.Fatal("returned secret field leaked")
		}
	}
}
