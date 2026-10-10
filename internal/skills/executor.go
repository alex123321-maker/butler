package skills

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/credentials"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/protobuf/proto"
	"strings"
)

// Fixed catalog operations run in the Broker-owned catalog module. They have
// no network/file/process execution or secret resolver. Other tools still route
// to their isolated runtime. This does not install an arbitrary in-core runtime.
type Executor interface {
	Execute(context.Context, *toolbrokerv1.ToolCall, *toolbrokerv1.ToolContract, []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error)
}
type RoutedExecutor struct {
	Store *Store
	Next  Executor
}

func Contracts() []*toolbrokerv1.ToolContract {
	result := []*toolbrokerv1.ToolContract{}
	for _, d := range []struct {
		Name, Description, Schema string
		Prepare                   bool
	}{
		{"skills.search", "List compatible active procedural DATA for the current task. Descriptions only; does not grant capabilities.", `{"type":"object","properties":{"query":{"type":"string"}},"additionalProperties":false}`, false},
		{"skills.load", "Load a selected exact skill digest as bounded procedural DATA. Tool permissions and task constraints remain independently enforced.", `{"type":"object","required":["digest"],"properties":{"digest":{"type":"string"},"max_bytes":{"type":"integer"}},"additionalProperties":false}`, false},
		{"skills.propose", "Propose a version from independently owner-verified task evidence. Stores a tested candidate; only an exact existing correction grant can activate it.", `{"type":"object","required":["manifest_text"],"properties":{"manifest_text":{"type":"string"}},"additionalProperties":false}`, true},
	} {
		e := &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}
		risk := "low"
		if d.Prepare {
			e = &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_runtime_ack"}
			risk = "medium"
		}
		result = append(result, &toolbrokerv1.ToolContract{ToolName: d.Name, Description: d.Description, ToolClass: "skill_catalog", InputSchemaJson: d.Schema, OutputSchemaJson: `{"type":"object"}`, RuntimeTarget: "broker://skill-catalog-v1", Status: "active", ContractRevision: "1", RiskLevel: risk, EffectContract: e})
	}
	return result
}
func (r *RoutedExecutor) Execute(ctx context.Context, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, secrets []credentials.ResolvedSecret) (*toolbrokerv1.ToolResult, error) {
	if call == nil {
		return nil, ErrManifest
	}
	if !strings.HasPrefix(call.GetToolName(), "skills.") {
		if r.Next == nil {
			return nil, ErrUnavailable
		}
		return r.Next.Execute(ctx, call, contract, secrets)
	}
	result := &toolbrokerv1.ToolResult{RunId: call.GetRunId(), ToolCallId: call.GetToolCallId(), ToolName: call.GetToolName(), Status: "failed", ResultJson: "{}"}
	fail := func(err error) (*toolbrokerv1.ToolResult, error) {
		result.Error = &toolbrokerv1.ToolError{ErrorClass: commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, Message: safeError(err), Retryable: false}
		return result, nil
	}
	exact := false
	for _, c := range Contracts() {
		if proto.Equal(c, contract) {
			exact = true
		}
	}
	if !exact || r.Store == nil || len(secrets) != 0 || len(call.GetCredentialRefs()) != 0 {
		return fail(ErrAccess)
	}
	var args struct {
		Query        string `json:"query"`
		Digest       string `json:"digest"`
		MaxBytes     int    `json:"max_bytes"`
		ManifestText string `json:"manifest_text"`
	}
	if Decode([]byte(call.GetArgsJson()), &args) != nil {
		return fail(ErrManifest)
	}
	var value any
	var err error
	switch call.GetToolName() {
	case "skills.search":
		var index []Descriptor
		index, err = r.Store.Index(ctx, call.GetRunId(), args.Query, 12)
		value = map[string]any{"format": "butler.skill.index.v1", "skills": index}
	case "skills.load":
		var p Package
		p, err = r.Store.Load(ctx, call.GetRunId(), args.Digest, args.MaxBytes)
		value = map[string]any{"format": "butler.skill.data.v1", "digest": p.Digest, "manifest": p.Manifest, "instruction_authority": false}
	case "skills.propose":
		var p Package
		p, err = Parse([]byte(args.ManifestText))
		if err == nil {
			var record Record
			record, err = r.Store.Propose(ctx, call.GetRunId(), p)
			value = map[string]any{"skill_id": record.ID, "digest": record.Digest, "status": record.Status, "active": record.Active, "instruction_authority": false}
		}
	default:
		err = ErrManifest
	}
	if err != nil {
		return fail(err)
	}
	b, err := json.Marshal(value)
	if err != nil || len(b) > 32*1024 {
		return fail(ErrManifest)
	}
	result.Status = "completed"
	result.ResultJson = string(b)
	return result, nil
}
