package generatedtools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/owneridentity"
)

func TestConverterPrivateClientsNeverSelectNetworkRuntimeOrFollowControlRedirect(t *testing.T) {
	for _, target := range []string{"converter:9090", "unix:///tmp/other.sock", "https://collect.example", "unix:///run/butler-converter/runtime.sock?leak=1"} {
		if _, _, e := NewRuntimeClient(target, strings.Repeat("x", 40)); e == nil {
			t.Fatal("runtime target widened", target)
		}
	}
	if _, _, e := NewRuntimeClient(RuntimeTarget, "short"); e == nil {
		t.Fatal("unauthenticated runtime")
	}
	var reached atomic.Int64
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Add(1) }))
	defer collector.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, collector.URL, http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	c, e := NewClient(redirect.URL+"/internal/converters", strings.Repeat("x", 40))
	if e != nil {
		t.Fatal(e)
	}
	ctx := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "fixture"})
	if _, e = c.Control(ctx, ControlRequest{Operation: "list"}); e == nil || reached.Load() != 0 {
		t.Fatal("service authentication followed redirect", e)
	}
	for _, endpoint := range []string{collector.URL + "/collect", collector.URL + "/internal/converters?q=1", collector.URL + "/internal/%63onverters", collector.URL + "/internal/converters#x"} {
		if _, e = NewClient(endpoint, strings.Repeat("x", 40)); e == nil {
			t.Fatal("control route widened")
		}
	}
}
