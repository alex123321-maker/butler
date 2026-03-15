package credentials

import (
	"context"
	"errors"
	"testing"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type stubSecretResolver struct {
	value string
	err   error
}

func (s stubSecretResolver) ResolveSecretRef(context.Context, string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	return s.value, nil
}

func TestResolveToolCall(t *testing.T) {
	t.Parallel()
	audit := &stubAuditLogger{}
	broker := NewToolCallBroker(
		NewBroker(stubMetadataStore{record: Record{Alias: "github", Status: StatusActive, AllowedDomains: []string{"api.github.com"}, AllowedTools: []string{"http.request"}, ApprovalPolicy: ApprovalPolicyAutoReadOnly, SecretRef: "env://GITHUB_TOKEN"}}, audit),
		stubSecretResolver{value: "secret-token"},
	)
	resolved, err := broker.ResolveToolCall(context.Background(), &toolbrokerv1.ToolCall{
		ToolCallId:     "tool-1",
		RunId:          "run-1",
		ToolName:       "http.request",
		ArgsJson:       `{"method":"GET","url":"https://api.github.com/repos"}`,
		CredentialRefs: []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: "github", Field: "token"}},
		AutonomyMode:   commonv1.AutonomyMode_AUTONOMY_MODE_2,
	})
	if err != nil {
		t.Fatalf("ResolveToolCall returned error: %v", err)
	}
	if len(resolved) != 1 || resolved[0].Value != "secret-token" {
		t.Fatalf("unexpected resolved secrets: %+v", resolved)
	}
	if len(audit.entries) != 1 || audit.entries[0].RunID != "run-1" || audit.entries[0].ToolCallID != "tool-1" {
		t.Fatalf("unexpected audit entries: %+v", audit.entries)
	}
}

func TestResolveToolCallRejectsApprovalRequired(t *testing.T) {
	t.Parallel()
	broker := NewToolCallBroker(
		NewBroker(stubMetadataStore{record: Record{Alias: "github", Status: StatusActive, AllowedTools: []string{"http.request"}, ApprovalPolicy: ApprovalPolicyAlwaysConfirm, SecretRef: "env://GITHUB_TOKEN"}}),
		stubSecretResolver{value: "secret-token"},
	)
	_, err := broker.ResolveToolCall(context.Background(), &toolbrokerv1.ToolCall{
		ToolName:       "http.request",
		ArgsJson:       `{"method":"GET","url":"https://api.github.com/repos"}`,
		CredentialRefs: []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: "github", Field: "token"}},
		AutonomyMode:   commonv1.AutonomyMode_AUTONOMY_MODE_2,
	})
	if err == nil {
		t.Fatal("expected approval-required credential usage to be rejected")
	}
}

func TestResolveToolCallPropagatesSecretError(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	broker := NewToolCallBroker(
		NewBroker(stubMetadataStore{record: Record{Alias: "github", Status: StatusActive, AllowedTools: []string{"http.request"}, ApprovalPolicy: ApprovalPolicyAutoReadOnly, SecretRef: "env://GITHUB_TOKEN"}}),
		stubSecretResolver{err: boom},
	)
	_, err := broker.ResolveToolCall(context.Background(), &toolbrokerv1.ToolCall{
		ToolName:       "http.request",
		ArgsJson:       `{"method":"GET","url":"https://api.github.com/repos"}`,
		CredentialRefs: []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: "github", Field: "token"}},
		AutonomyMode:   commonv1.AutonomyMode_AUTONOMY_MODE_2,
	})
	if !errors.Is(err, boom) {
		t.Fatalf("ResolveToolCall error = %v, want %v", err, boom)
	}
}

func TestToolExecutionMetadata(t *testing.T) {
	t.Parallel()
	url, mutating := toolExecutionMetadata(&toolbrokerv1.ToolCall{ToolName: "http.request", ArgsJson: `{"method":"POST","url":"https://api.github.com/repos"}`})
	if url != "https://api.github.com/repos" || !mutating {
		t.Fatalf("unexpected metadata url=%q mutating=%v", url, mutating)
	}
}

func TestResolveToolCallRejectsUnspecifiedAutonomyMode(t *testing.T) {
	t.Parallel()
	broker := NewToolCallBroker(
		NewBroker(stubMetadataStore{record: Record{Alias: "github", Status: StatusActive, AllowedTools: []string{"http.request"}, ApprovalPolicy: ApprovalPolicyAutoReadOnly, SecretRef: "env://GITHUB_TOKEN"}}),
		stubSecretResolver{value: "secret-token"},
	)
	_, err := broker.ResolveToolCall(context.Background(), &toolbrokerv1.ToolCall{
		ToolName:       "http.request",
		ArgsJson:       `{"method":"GET","url":"https://api.github.com/repos"}`,
		CredentialRefs: []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: "github", Field: "token"}},
		AutonomyMode:   commonv1.AutonomyMode_AUTONOMY_MODE_UNSPECIFIED,
	})
	if err == nil {
		t.Fatal("expected unspecified autonomy mode to be rejected")
	}
}
