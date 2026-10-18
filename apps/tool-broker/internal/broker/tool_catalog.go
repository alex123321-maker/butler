package broker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/toolcatalog"
	"google.golang.org/protobuf/proto"
)

// Discovery executes inside the ordinary authorized/claimed Broker path. It
// returns DATA refs only; the Orchestrator owns the provider's selected schemas.
type ToolCatalogExecutor struct {
	Catalog ScopedCatalog
	Next    RuntimeExecutor
}

func (r *ToolCatalogExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if call == nil {
		return nil, fmt.Errorf("catalog call required")
	}
	if !toolcatalog.IsTool(call.ToolName) {
		if r == nil || r.Next == nil {
			return nil, fmt.Errorf("tool execution unavailable")
		}
		return r.Next.Execute(ctx, call, contract, secrets)
	}
	id := actionauth.DispatchIdentityFromContext(ctx)
	if r == nil || r.Catalog == nil || !proto.Equal(contract, toolcatalog.Contract(call.ToolName)) || len(secrets) > 0 || len(call.CredentialRefs) > 0 || id.ActionID == "" || id.Fingerprint == "" {
		return nil, fmt.Errorf("catalog dispatch denied")
	}
	a, err := toolcatalog.Decode(call.ToolName, call.ArgsJson)
	if err != nil {
		return nil, err
	}
	contracts, err := r.Catalog.ListForRun(ctx, call.RunId, "", 0)
	if err != nil {
		return nil, err
	}
	result := toolcatalog.Result{}
	if call.ToolName == toolcatalog.Search {
		result = toolcatalog.SearchPage(contracts, a)
	} else {
		selected, ok := toolcatalog.Resolve(contracts, a.Refs)
		if !ok {
			result.Status = "unavailable_or_stale"
		} else {
			result.Status = "ok"
			for _, c := range selected {
				result.Loaded = append(result.Loaded, toolcatalog.Ref(c))
			}
		}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: string(raw)}, nil
}
