package generatedtools

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/butler/butler/internal/budget"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
)

const DeveloperAdmissionTool = "task.delegate_converter"
const DeveloperAdmissionTarget = "broker://converter-delegation-v1"
const DeveloperFormat = "butler.converter.developer.v1"

// DeveloperSpec is the entire selected synthetic contract body. Free-form
// narrative, document copies, permissions, code and author-defined acceptance
// tests are unrepresentable here; independent cases remain compiled in core.
type DeveloperSpec struct {
	Format   string `json:"format"`
	NeedHash string `json:"need_hash"`
	Version  string `json:"version"`
	Checker  string `json:"checker"`
}

func DecodeDeveloperSpec(raw []byte, needHash string) (DeveloperSpec, error) {
	var spec DeveloperSpec
	if Decode(raw, &spec) != nil || spec.Format != DeveloperFormat || !digestName.MatchString(needHash) || spec.NeedHash != needHash || !version.MatchString(spec.Version) || len(spec.Version) > 32 || spec.Checker != Checker {
		return DeveloperSpec{}, ErrPackage
	}
	return spec, nil
}

// DeveloperArgs selects only current DATA and a finite proposed allocation.
// The approved parent dispatch and independently retained discovery receipt,
// not this envelope, determine child identity and its exact three capabilities.
type DeveloperArgs struct {
	NeedSource     catalog.SourceRef `json:"need_source"`
	ContractSource catalog.SourceRef `json:"contract_source"`
	Budget         budget.Units      `json:"budget"`
	Deadline       time.Time         `json:"deadline"`
	Dependencies   []string          `json:"dependencies"`
}

func DeveloperAdmissionContract() *toolbrokerv1.ToolContract {
	units := `{"type":"object","required":["model_turns","tokens","cost_micros","external_calls","children","active_millis","egress_bytes","provider_input_bytes"],"properties":{"model_turns":{"type":"integer"},"tokens":{"type":"integer"},"cost_micros":{"type":"integer"},"external_calls":{"type":"integer"},"children":{"type":"integer"},"active_millis":{"type":"integer"},"egress_bytes":{"type":"integer"},"provider_input_bytes":{"type":"integer"}},"additionalProperties":false}`
	input := `{"type":"object","required":["need_source","contract_source","budget","deadline","dependencies"],"properties":{"need_source":` + referenceSchema + `,"contract_source":` + referenceSchema + `,"budget":` + units + `,"deadline":{"type":"string"},"dependencies":{"type":"array","items":{"type":"string"}}},"additionalProperties":false}`
	output := `{"type":"object","required":["task_id","input_id","state","contract_revision"],"properties":{"task_id":{"type":"string"},"input_id":{"type":"string"},"state":{"type":"string"},"contract_revision":{"type":"integer"}},"additionalProperties":false}`
	return &toolbrokerv1.ToolContract{ToolName: DeveloperAdmissionTool, Description: "Admit one isolated converter developer from an exact current Need and synthetic contract DATA after the parent's actual converter discovery. Child rights are derived by Butler; no document, trial, activation or credentials are delegated. A receipt confirms admission only.", ToolClass: "task_control", InputSchemaJson: input, OutputSchemaJson: output, RuntimeTarget: DeveloperAdmissionTarget, RiskLevel: "medium", Status: "active", ContractRevision: "1", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_runtime_ack"}}
}

// This is syntax validation, never capability or budget authorization. The
// separately enabled private admission reader repeats current PG proofs.
func DecodeDeveloperArgs(raw string) (DeveloperArgs, error) {
	var args DeveloperArgs
	if Decode([]byte(raw), &args) != nil || !developerSource(args.NeedSource) || !developerSource(args.ContractSource) || args.NeedSource.ID == args.ContractSource.ID || args.Deadline.IsZero() || args.Dependencies == nil || len(args.Dependencies) > 8 {
		return DeveloperArgs{}, ErrPackage
	}
	var object map[string]json.RawMessage
	var units map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &object) != nil || json.Unmarshal(object["budget"], &units) != nil {
		return DeveloperArgs{}, ErrPackage
	}
	for _, name := range []string{"model_turns", "tokens", "cost_micros", "external_calls", "children", "active_millis", "egress_bytes", "provider_input_bytes"} {
		var amount int64
		if json.Unmarshal(units[name], &amount) != nil || strings.TrimSpace(string(units[name])) == "null" || amount < 0 {
			return DeveloperArgs{}, ErrPackage
		}
	}
	seen := map[string]bool{}
	for _, id := range args.Dependencies {
		if id == "" || len(id) > 160 || strings.TrimSpace(id) != id || strings.ContainsAny(id, "\x00\r\n* ") || seen[id] {
			return DeveloperArgs{}, ErrPackage
		}
		seen[id] = true
	}
	args.Deadline = args.Deadline.UTC().Truncate(time.Microsecond)
	return args, nil
}

func developerSource(ref catalog.SourceRef) bool {
	return ref.ID != "" && len(ref.ID) <= 160 && strings.TrimSpace(ref.ID) == ref.ID && !strings.ContainsAny(ref.ID, "\x00\r\n") && ref.Revision > 0 && ref.AccessRevision > 0 && digestName.MatchString(ref.Hash)
}
