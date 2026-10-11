package resourceexec

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/resources"
	"github.com/jackc/pgx/v5"

	"google.golang.org/grpc/metadata"
)

type ackBackend struct {
	fixtureBackend
	acks int
}

func TestSingleTabResourceLegacyPinnedContractDenied(t *testing.T) {
	for _, revision := range []string{"1", SingleTabContractRevision} {
		c := &toolbrokerv1.ToolContract{ToolName: "single_tab.status", ContractRevision: revision, RuntimeTarget: "fixed-private-target", Status: "enabled", InputSchemaJson: `{"type":"object"}`}
		call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "observe", ToolName: c.ToolName, ArgsJson: `{"session_id":"approved"}`}
		ch, err := actionauth.ContractHash(c)
		if err != nil {
			t.Fatal(err)
		}
		ah, err := actionauth.ArgumentsHash(call.ArgsJson)
		if err != nil {
			t.Fatal(err)
		}
		g := actionauth.Grant{ID: "authorization", Binding: actionauth.ActionBinding{RunID: call.RunId, ToolCallID: call.ToolCallId, ToolName: call.ToolName, ContractHash: ch, ArgsHash: ah}}
		store := &Store{profile: Profile{ID: "fixed", RuntimeTarget: c.RuntimeTarget, ContractHashes: map[string]string{c.ToolName: ch}}, singleTab: func(context.Context, pgx.Tx, resources.Binding, string) (SingleTabSession, error) {
			return SingleTabSession{}, nil
		}}
		_, e := store.singleSelection(call, c, g)
		if (e == nil) != (revision == SingleTabContractRevision) {
			t.Fatal("legacy hash activated unsupported contract", revision, e)
		}
	}
}

func (b *ackBackend) ConfirmRuntime(context.Context, RuntimeSpec) error { b.acks++; return nil }
func TestRuntimePrivateACKRequiresDurableFinish(t *testing.T) {
	token := strings.Repeat("fixture", 8)
	authority := &fixtureAuthority{fence: 1, deadline: time.Now().Add(time.Second), finishErr: true}
	backend := &ackBackend{}
	runtime, e := NewRuntime(backend, authority, token)
	if e != nil {
		t.Fatal(e)
	}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token, ticketHeader, strings.Repeat("a", 48)))
	result, _ := runtime.Execute(ctx, fixtureRequest())
	if result.GetResult().GetStatus() != "effect_unknown" || backend.acks != 0 {
		t.Fatal("extension ACK preceded durable outcome")
	}
	authority.fence = 2
	authority.finishErr = false
	result, _ = runtime.Execute(ctx, fixtureRequest())
	if result.GetResult().GetStatus() != "completed" || backend.acks != 1 {
		t.Fatal("durably finished lane not acknowledged")
	}
}
func TestSingleTabResourceIdentityUsesActualHostTabNotBindingID(t *testing.T) {
	first := SingleTabSession{ID: "approved-first", BrowserInstance: "fixed", TabRef: "42", Origin: "https://example.invalid", ExpiresAt: time.Now().Add(time.Minute)}
	second := first
	second.ID = "approved-second"
	a, e := singleResource("profile", first)
	if e != nil {
		t.Fatal(e)
	}
	b, e := singleResource("profile", second)
	if e != nil || a != b || strings.Contains(a, "approved") {
		t.Fatal("rebinding partitioned actual tab resource")
	}
	second.Origin = "https://other.invalid"
	b, e = singleResource("profile", second)
	if e != nil || a != b {
		t.Fatal("origin switch partitioned actual tab resource")
	}
	second.BrowserInstance = "other-fixed-host"
	b, e = singleResource("profile", second)
	if e != nil || a == b {
		t.Fatal("independent hosts incorrectly share tab lane")
	}
	second.TabRef = "42suffix"
	if _, e = singleResource("profile", second); e == nil {
		t.Fatal("malformed actual tab accepted")
	}
}

func TestSingleTabResourceOriginStripsPrivateURIFields(t *testing.T) {
	for raw, want := range map[string]string{"https://Example.invalid:443/private?query=omit#omit": "https://example.invalid", "http://127.0.0.1:8080/path": "http://127.0.0.1:8080", "http://[::1]:80/path": "http://[::1]"} {
		got, e := SingleTabOrigin(raw)
		if e != nil || got != want {
			t.Fatal("selected origin mismatch", got, e)
		}
	}
	for _, raw := range []string{"https://user:password@example.invalid/", "file:///private", "https://", "https://example.invalid:99999/", "http://[fe80::1%25eth0]/"} {
		if _, e := SingleTabOrigin(raw); e == nil {
			t.Fatal("unsafe selected origin accepted")
		}
	}
}
