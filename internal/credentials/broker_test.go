package credentials

import (
	"context"
	"errors"
	"testing"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type stubMetadataStore struct {
	record Record
	err    error
}

func (s stubMetadataStore) GetByAlias(context.Context, string) (Record, error) {
	if s.err != nil {
		return Record{}, s.err
	}
	return s.record, nil
}

func TestAuthorizeUsage(t *testing.T) {
	t.Parallel()
	broker := NewBroker(stubMetadataStore{record: Record{Alias: "github", Status: StatusActive, AllowedDomains: []string{"api.github.com"}, AllowedTools: []string{"http.request"}, ApprovalPolicy: ApprovalPolicyConfirmOnMutation}})
	decision, err := broker.AuthorizeUsage(context.Background(), AuthorizationRequest{Alias: "github", ToolName: "http.request", TargetURL: "https://api.github.com/repos", Mutating: false, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2})
	if err != nil {
		t.Fatalf("AuthorizeUsage returned error: %v", err)
	}
	if decision.RequiresApproval {
		t.Fatal("expected read-only usage in mode_2 to skip approval")
	}
	if decision.NormalizedDomain != "api.github.com" {
		t.Fatalf("unexpected normalized domain %q", decision.NormalizedDomain)
	}
}

func TestAuthorizeUsageDeniedInModeZero(t *testing.T) {
	t.Parallel()
	broker := NewBroker(stubMetadataStore{record: Record{Alias: "github", Status: StatusActive, ApprovalPolicy: ApprovalPolicyAlwaysConfirm}})
	_, err := broker.AuthorizeUsage(context.Background(), AuthorizationRequest{Alias: "github", ToolName: "http.request", AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_0})
	if err == nil {
		t.Fatal("expected autonomy mode 0 to be denied")
	}
}

func TestAuthorizeUsageRejectsDisallowedDomain(t *testing.T) {
	t.Parallel()
	broker := NewBroker(stubMetadataStore{record: Record{Alias: "github", Status: StatusActive, AllowedDomains: []string{"api.github.com"}, AllowedTools: []string{"http.request"}, ApprovalPolicy: ApprovalPolicyAlwaysConfirm}})
	_, err := broker.AuthorizeUsage(context.Background(), AuthorizationRequest{Alias: "github", ToolName: "http.request", TargetURL: "https://example.com", AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2})
	if err == nil {
		t.Fatal("expected disallowed domain to fail")
	}
}

func TestResolveReference(t *testing.T) {
	t.Parallel()
	broker := NewBroker(stubMetadataStore{record: Record{Alias: "github", Status: StatusActive, AllowedTools: []string{"http.request"}, ApprovalPolicy: ApprovalPolicyAlwaysConfirm}})
	decision, err := broker.ResolveReference(context.Background(), &toolbrokerv1.CredentialRef{Type: "credential_ref", Alias: "github", Field: "token"}, "http.request", "", commonv1.AutonomyMode_AUTONOMY_MODE_2, false)
	if err != nil {
		t.Fatalf("ResolveReference returned error: %v", err)
	}
	if !decision.RequiresApproval {
		t.Fatal("expected always_confirm policy to require approval")
	}
}

func TestResolveReferencePropagatesStoreError(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	broker := NewBroker(stubMetadataStore{err: boom})
	_, err := broker.ResolveReference(context.Background(), &toolbrokerv1.CredentialRef{Type: "credential_ref", Alias: "github", Field: "token"}, "http.request", "", commonv1.AutonomyMode_AUTONOMY_MODE_2, false)
	if !errors.Is(err, boom) {
		t.Fatalf("ResolveReference error = %v, want %v", err, boom)
	}
}
