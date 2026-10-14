package antigravity

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestCurrentInferenceAuthorityDeniesImmediatelyBeforeHTTP(t *testing.T) {
	calls, checks := 0, 0
	p := newFixture(t, func(*http.Request) (*http.Response, error) {
		calls++
		return response(frame(`{"text":"done"}`, "STOP")), nil
	})
	ctx := transport.WithInferenceAuthority(context.Background(), func(context.Context) error {
		checks++
		if checks == 1 {
			return nil
		}
		return errors.New("PRIVATE authority reason")
	})
	stream, err := p.StartRun(ctx, request("revoked-before-http"))
	if err != nil {
		t.Fatal(err)
	}
	events := collect(t, stream)
	failed := false
	for _, e := range events {
		if e.TransportError != nil {
			failed = true
			if e.TransportError.Type != transport.ErrorTypeMemorySourceStale || e.TransportError.Message != "memory_source_stale" {
				t.Fatal("unsafe error exposed", e.TransportError)
			}
		}
	}
	if calls != 0 || checks != 2 || !failed {
		t.Fatal("revoked source reached HTTP", calls, checks, failed)
	}
}
