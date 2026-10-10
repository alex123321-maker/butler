package api

import (
	"context"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOwnerControlAuthRejectsClaimedIdentity(t *testing.T) {
	token := strings.Repeat("x", 40)
	auth := NewOwnerControlAuth(token, "trusted-owner")
	for _, header := range []string{"", "Bearer wrong", "Basic " + token} {
		r := httptest.NewRequest(http.MethodPost, "/approve", nil)
		r.Header.Set("Authorization", header)
		r.Header.Set("X-Butler-Actor", "trusted-owner")
		if _, ok := auth.Authenticate(r); ok {
			t.Fatalf("claimed actor authorized with %q", header)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/approve", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Butler-Actor", "attacker")
	r.Header.Set("X-Butler-Delivery-ID", "decision-1")
	actor, ok := auth.Authenticate(r)
	if !ok || actor.OwnerID != "trusted-owner" || actor.Channel != "web" || actor.DeliveryID != "decision-1" {
		t.Fatalf("trusted scope: %#v %v", actor, ok)
	}
	if _, ok = NewOwnerControlAuth("", "").Authenticate(r); ok {
		t.Fatal("unconfigured authority permitted decision")
	}
}

type actorCapturingResolver struct {
	actor approvals.Actor
	calls int
}

func (s *actorCapturingResolver) ResolveByToolCall(ctx context.Context, p approvals.ResolveByToolCallParams) (approvals.Record, bool, error) {
	s.actor, _ = approvals.ActorFromContext(ctx)
	s.calls++
	return approvals.Record{ApprovalID: "approval-1", Status: approvals.StatusApproved}, true, nil
}
func TestApprovalEndpointRequiresOwnerProof(t *testing.T) {
	store := &fakeApprovalsStore{itemByID: map[string]approvals.Record{"approval-1": {ApprovalID: "approval-1", ToolCallID: "call-1", Status: approvals.StatusPending}}}
	resolver := &actorCapturingResolver{}
	server := NewApprovalsServer(store, resolver, nil)
	token := strings.Repeat("fixture-token-", 4)
	server.SetOwnerControlAuth(NewOwnerControlAuth(token, "configured-owner"))
	for _, header := range []string{"", "Bearer wrong", "Bearer " + token} {
		r := httptest.NewRequest(http.MethodPost, "/api/v2/approvals/approval-1/approve", nil)
		r.Header.Set("X-Butler-Actor", "configured-owner")
		r.Header.Set("Authorization", header)
		w := httptest.NewRecorder()
		server.HandleApprove("/api/v2/approvals/").ServeHTTP(w, r)
		expected := http.StatusUnauthorized
		if header == "Bearer "+token {
			expected = http.StatusOK
		}
		if w.Code != expected {
			t.Fatalf("owner response %d, want %d: %s", w.Code, expected, w.Body.String())
		}
	}
	if resolver.calls != 1 || resolver.actor.OwnerID != "configured-owner" {
		t.Fatalf("resolver bypassed owner proof: %#v", resolver)
	}
}
