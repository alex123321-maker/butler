package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/extensions"
	"github.com/butler/butler/internal/owneridentity"
)

func TestExtensionOwnerBoundaryNeverForwardsChannelToken(t *testing.T) {
	ownerToken := strings.Repeat("owner-fixture-", 4)
	serviceToken := strings.Repeat("service-fixture-", 4)
	var calls atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+serviceToken {
			t.Error("owner token forwarded into backend")
		}
		var envelope struct {
			Actor   owneridentity.Actor
			Request extensions.ControlRequest
		}
		if json.NewDecoder(r.Body).Decode(&envelope) != nil || envelope.Actor.OwnerID != "owner" || envelope.Actor.Channel != "web" {
			t.Error("verified owner provenance lost")
		}
		json.NewEncoder(w).Encode(extensions.ControlResponse{})
	}))
	defer backend.Close()
	h := ReviewedExtensions(NewOwnerControlAuth(ownerToken, "owner"), extensions.NewControlClient(backend.URL, serviceToken))
	for _, body := range []string{`{"operation":"disable","extension_id":"notes"}`, `{"operation":"list","operation":"enable"}`, `{"operation":"list","owner_id":"forged"}`} {
		r := httptest.NewRequest("POST", "/api/v4/extensions", strings.NewReader(body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 || calls.Load() != 0 {
			t.Fatal("unauthenticated control touched backend")
		}
		r = httptest.NewRequest("POST", "/api/v4/extensions", strings.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+ownerToken)
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if strings.Contains(body, "disable") {
			if w.Code != 200 || calls.Load() != 1 {
				t.Fatal("verified control unavailable")
			}
			calls.Store(0)
		} else if w.Code != 400 || calls.Load() != 0 {
			t.Fatal("ambiguous control reached backend")
		}
	}
	if _, err := extensions.NewControlClient(backend.URL, serviceToken).Control(context.Background(), extensions.ControlRequest{Operation: "list"}); err == nil {
		t.Fatal("caller without verified owner context used control")
	}
}
