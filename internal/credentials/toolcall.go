package credentials

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type ToolCallBroker struct {
	authorizer *Broker
	resolver   SecretResolver
}

func NewToolCallBroker(authorizer *Broker, resolver SecretResolver) *ToolCallBroker {
	return &ToolCallBroker{authorizer: authorizer, resolver: resolver}
}

func (b *ToolCallBroker) ResolveToolCall(ctx context.Context, call *toolbrokerv1.ToolCall) ([]ResolvedSecret, error) {
	if call == nil || len(call.GetCredentialRefs()) == 0 {
		return nil, nil
	}
	if b == nil || b.authorizer == nil || b.resolver == nil {
		return nil, fmt.Errorf("credential resolution is not configured")
	}
	targetURL, mutating := toolExecutionMetadata(call)
	resolved := make([]ResolvedSecret, 0, len(call.GetCredentialRefs()))
	for _, ref := range call.GetCredentialRefs() {
		decision, err := b.authorizer.AuthorizeUsage(ctx, AuthorizationRequest{
			RunID:        call.GetRunId(),
			ToolCallID:   call.GetToolCallId(),
			Alias:        ref.GetAlias(),
			Field:        ref.GetField(),
			ToolName:     call.GetToolName(),
			TargetURL:    targetURL,
			Mutating:     mutating,
			AutonomyMode: call.GetAutonomyMode(),
		})
		if err != nil {
			return nil, err
		}
		if decision.RequiresApproval {
			return nil, fmt.Errorf("credential alias %q requires approval: %s", decision.Record.Alias, decision.ApprovalReason)
		}
		value, err := b.resolver.ResolveSecretRef(ctx, decision.Record.SecretRef)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, ResolvedSecret{Alias: decision.Record.Alias, Field: strings.TrimSpace(ref.GetField()), Value: value})
	}
	return resolved, nil
}

func toolExecutionMetadata(call *toolbrokerv1.ToolCall) (targetURL string, mutating bool) {
	if call == nil || strings.TrimSpace(call.GetArgsJson()) == "" {
		return "", false
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(call.GetArgsJson()), &args); err != nil {
		return "", false
	}
	if rawURL, ok := args["url"].(string); ok {
		targetURL = strings.TrimSpace(rawURL)
	}
	switch strings.TrimSpace(call.GetToolName()) {
	case "http.request":
		method, _ := args["method"].(string)
		method = strings.ToUpper(strings.TrimSpace(method))
		if method == "" {
			method = "GET"
		}
		mutating = method != "GET"
	default:
		mutating = false
	}
	return targetURL, mutating
}
