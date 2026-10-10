package credentials

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type ToolCallBroker struct {
	authorizer *Broker
	resolver   SecretResolver
}

type secretResolutionError struct{ cause error }

func (e secretResolutionError) Error() string { return "secret resolution failed" }
func (e secretResolutionError) Unwrap() error { return e.cause }

func NewToolCallBroker(authorizer *Broker, resolver SecretResolver) *ToolCallBroker {
	return &ToolCallBroker{authorizer: authorizer, resolver: resolver}
}

func (b *ToolCallBroker) ResolveToolCall(ctx context.Context, call *toolbrokerv1.ToolCall) ([]ResolvedSecret, error) {
	return b.resolve(ctx, call, nil)
}

// ActionProof is constructed by Tool Broker only after verifying the durable
// action grant. It stays out of model args, transcripts and runtime arguments.
type ActionProof struct {
	RunID, ToolCallID, ToolName, ArgsJSON, MetadataHash string
	Approved                                            bool
	Revalidate                                          func(context.Context) error
}

func (b *ToolCallBroker) ResolveAuthorizedToolCall(ctx context.Context, call *toolbrokerv1.ToolCall, proof ActionProof) ([]ResolvedSecret, error) {
	return b.resolve(ctx, call, &proof)
}

func (b *ToolCallBroker) InspectToolCall(ctx context.Context, call *toolbrokerv1.ToolCall) (string, bool, error) {
	if call == nil || len(call.GetCredentialRefs()) == 0 {
		return "", false, nil
	}
	if b == nil || b.authorizer == nil {
		return "", false, fmt.Errorf("credential policy is not configured")
	}
	target, mutating := toolExecutionMetadata(call)
	type version struct{ Alias, Field, Hash string }
	versions := []version{}
	approval := false
	for _, ref := range call.GetCredentialRefs() {
		decision, err := b.authorizer.AuthorizeUsage(ctx, AuthorizationRequest{RunID: call.GetRunId(), ToolCallID: call.GetToolCallId(), Alias: ref.GetAlias(), Field: ref.GetField(), ToolName: call.GetToolName(), TargetURL: target, Mutating: mutating, AutonomyMode: call.GetAutonomyMode()})
		if err != nil {
			return "", false, err
		}
		// Metadata includes the private reference locator, never its resolved value.
		// Only its digest crosses the action approval boundary.
		encoded, err := json.Marshal(decision.Record)
		if err != nil {
			return "", false, err
		}
		hash := sha256.Sum256(encoded)
		versions = append(versions, version{ref.GetAlias(), ref.GetField(), hex.EncodeToString(hash[:])})
		approval = approval || decision.RequiresApproval
	}
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].Alias == versions[j].Alias {
			return versions[i].Field < versions[j].Field
		}
		return versions[i].Alias < versions[j].Alias
	})
	encoded, _ := json.Marshal(versions)
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:]), approval, nil
}

func (b *ToolCallBroker) resolve(ctx context.Context, call *toolbrokerv1.ToolCall, proof *ActionProof) ([]ResolvedSecret, error) {
	if call == nil || len(call.GetCredentialRefs()) == 0 {
		return nil, nil
	}
	if b == nil || b.authorizer == nil || b.resolver == nil {
		return nil, fmt.Errorf("credential resolution is not configured")
	}
	if proof != nil {
		if proof.Revalidate == nil {
			return nil, fmt.Errorf("live action authorization verifier is required")
		}
		if proof.RunID != call.GetRunId() || proof.ToolCallID != call.GetToolCallId() || proof.ToolName != call.GetToolName() || proof.ArgsJSON != call.GetArgsJson() || proof.MetadataHash == "" {
			return nil, fmt.Errorf("credential action proof does not match the exact call")
		}
		hash, _, err := b.InspectToolCall(ctx, call)
		if err != nil || hash != proof.MetadataHash {
			return nil, fmt.Errorf("credential metadata changed after action authorization")
		}
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
		if decision.RequiresApproval && (proof == nil || !proof.Approved) {
			return nil, fmt.Errorf("credential alias %q requires approval: %s", decision.Record.Alias, decision.ApprovalReason)
		}
		if proof != nil {
			if err := proof.Revalidate(ctx); err != nil {
				return nil, fmt.Errorf("action authority changed before secret release")
			}
		}
		value, err := b.resolver.ResolveSecretRef(ctx, decision.Record.SecretRef)
		if err != nil {
			return nil, secretResolutionError{cause: err}
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
	if strings.HasPrefix(call.GetToolName(), "browser.") {
		if observedURL, ok := args["expected_url"].(string); ok {
			targetURL = strings.TrimSpace(observedURL)
		}
	}
	switch strings.TrimSpace(call.GetToolName()) {
	case "http.request":
		method, _ := args["method"].(string)
		method = strings.ToUpper(strings.TrimSpace(method))
		if method == "" {
			method = "GET"
		}
		mutating = method != "GET" && method != "HEAD" && method != "OPTIONS"
	case "browser.open", "browser.navigate", "browser.click", "browser.fill", "browser.type", "browser.set_cookie", "browser.restore_storage_state":
		mutating = true
	default:
		// Unknown runtime effects never receive read-only credential authority.
		mutating = call.GetToolName() != "browser.read" && call.GetToolName() != "browser.snapshot" && call.GetToolName() != "browser.extract_text" && call.GetToolName() != "browser.wait_for"
	}
	return targetURL, mutating
}
