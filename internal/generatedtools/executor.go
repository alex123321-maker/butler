package generatedtools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/butler/butler/internal/credentials"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

type Executor interface {
	Execute(context.Context, *toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract, []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error)
}
type RoutedExecutor struct {
	Store *Store
	Next  Executor
}

func SafeError(e error) string {
	switch {
	case errors.Is(e, ErrChecks):
		return "independent converter checks failed"
	case errors.Is(e, ErrLimit):
		return "converter resource or trial allowance reached"
	case errors.Is(e, ErrInput):
		return "converter input rejected"
	case errors.Is(e, ErrPackage):
		return "bounded converter package rejected"
	default:
		return "current converter source, catalog and owner authority required"
	}
}

// All document/program bytes are resolved only after Claim, under current
// owned access. Model results contain content-free identities and source refs.
func (r *RoutedExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if call == nil {
		return nil, ErrPackage
	}
	if !strings.HasPrefix(call.ToolName, "converters.") && !strings.HasPrefix(call.ToolName, "converter.") {
		if r.Next == nil {
			return nil, ErrAccess
		}
		return r.Next.Execute(ctx, call, c, secrets)
	}
	result := &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "failed", ResultJson: "{}"}
	fail := func(e error) (*toolbrokerv1.ToolResult, error) {
		result.Error = &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, Message: SafeError(e), Retryable: false}
		return result, nil
	}
	if r.Store == nil || c == nil || len(secrets) != 0 || len(call.CredentialRefs) != 0 || c.RuntimeTarget != "broker://pure-converters-v1" {
		return fail(ErrAccess)
	}
	if strings.HasPrefix(call.ToolName, "converters.") && !staticContract(c) {
		return fail(ErrAccess)
	}
	var args Arguments
	if Decode([]byte(call.ArgsJson), &args) != nil {
		return fail(ErrPackage)
	}
	var value any
	var e error
	switch call.ToolName {
	case "converters.discover":
		var n Need
		var d Discovery
		n, d, e = r.Store.Discover(ctx, call, c, args.Source)
		value = map[string]any{"need_id": n.ID, "need_hash": Hash(n), "discovery": d, "instruction_authority": false}
	case "converters.develop":
		value, e = r.Store.Develop(ctx, call, c, args.Source, args.Version)
	case "converters.test":
		value, e = r.Store.Test(ctx, call, c, args.Digest)
	case "converters.trial":
		value, e = r.Store.Apply(ctx, call, c, args.Digest, args.Input, true)
	case "converters.activate", "converters.rollback":
		value, e = r.Store.Activate(ctx, call, c, args.Digest, args.Generation)
	default:
		if c.GetConverter() == nil || call.ToolName != "converter."+c.GetConverter().ConverterId+".convert" {
			return fail(ErrAccess)
		}
		value, e = r.Store.Apply(ctx, call, c, c.GetConverter().PackageHash, args.Input, false)
	}
	if e != nil {
		return fail(e)
	}
	b, e := json.Marshal(value)
	if e != nil || len(b) > 32*1024 {
		return fail(ErrLimit)
	}
	result.Status = "completed"
	result.ResultJson = string(b)
	return result, nil
}
