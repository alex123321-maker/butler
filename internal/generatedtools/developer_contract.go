package generatedtools

import (
	"encoding/json"
	"math/big"
	"strconv"
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
	input := `{"type":"object","required":["need_source","contract_source","budget","deadline","dependencies"],"properties":{"need_source":` + referenceSchema + `,"contract_source":` + referenceSchema + `,"budget":` + units + `,"deadline":{"type":"string","description":"Absolute future RFC3339 timestamp with explicit timezone, using the current parent deadline; use YYYY-MM-DDTHH:MM:SSZ or an explicit UTC offset. A date without a time or an arbitrary string is invalid."},"dependencies":{"type":"array","items":{"type":"string"}}},"additionalProperties":false}`
	output := `{"type":"object","required":["task_id","input_id","state","contract_revision"],"properties":{"task_id":{"type":"string"},"input_id":{"type":"string"},"state":{"type":"string"},"contract_revision":{"type":"integer"}},"additionalProperties":false}`
	return &toolbrokerv1.ToolContract{ToolName: DeveloperAdmissionTool, Description: "Admit one isolated converter developer from an exact current Need and synthetic contract DATA after the parent's actual converter discovery. Child rights are derived by Butler; no document, trial, activation or credentials are delegated. A receipt confirms admission only. After a queued admission receipt, finish this parent attempt without claiming the task is complete. Collect only in a fresh parent attempt after the child's tested immutable final/report exists; owned automatic continuations, when enabled, schedule that attempt.", ToolClass: "task_control", InputSchemaJson: input, OutputSchemaJson: output, RuntimeTarget: DeveloperAdmissionTarget, RiskLevel: "medium", Status: "active", ContractRevision: "1", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_runtime_ack"}}
}

// This is syntax validation, never capability or budget authorization. The
// separately enabled private admission reader repeats current PG proofs.
func DecodeDeveloperArgs(raw string) (DeveloperArgs, error) {
	// Syntax decoding preserves exact ArgsJSON and the authorization fingerprint.
	// Only these declared integer fields get exact, bounded semantic decoding;
	// the unchanged current parent/source/budget checks still grant authority.
	var wire struct {
		NeedSource     json.RawMessage `json:"need_source"`
		ContractSource json.RawMessage `json:"contract_source"`
		Budget         json.RawMessage `json:"budget"`
		Deadline       time.Time       `json:"deadline"`
		Dependencies   []string        `json:"dependencies"`
	}
	if Decode([]byte(raw), &wire) != nil {
		return DeveloperArgs{}, ErrPackage
	}
	need, err := decodeDeveloperSource(wire.NeedSource)
	if err != nil {
		return DeveloperArgs{}, ErrPackage
	}
	contract, err := decodeDeveloperSource(wire.ContractSource)
	if err != nil {
		return DeveloperArgs{}, ErrPackage
	}
	var fields struct {
		ModelTurns         json.RawMessage `json:"model_turns"`
		Tokens             json.RawMessage `json:"tokens"`
		CostMicros         json.RawMessage `json:"cost_micros"`
		ExternalCalls      json.RawMessage `json:"external_calls"`
		Children           json.RawMessage `json:"children"`
		ActiveMillis       json.RawMessage `json:"active_millis"`
		EgressBytes        json.RawMessage `json:"egress_bytes"`
		ProviderInputBytes json.RawMessage `json:"provider_input_bytes"`
	}
	if Decode(wire.Budget, &fields) != nil {
		return DeveloperArgs{}, ErrPackage
	}
	args := DeveloperArgs{NeedSource: need, ContractSource: contract, Deadline: wire.Deadline, Dependencies: wire.Dependencies}
	pairs := []struct {
		raw   json.RawMessage
		value *int64
	}{{fields.ModelTurns, &args.Budget.ModelTurns}, {fields.Tokens, &args.Budget.Tokens}, {fields.CostMicros, &args.Budget.CostMicros}, {fields.ExternalCalls, &args.Budget.ExternalCalls}, {fields.Children, &args.Budget.Children}, {fields.ActiveMillis, &args.Budget.ActiveMillis}, {fields.EgressBytes, &args.Budget.EgressBytes}, {fields.ProviderInputBytes, &args.Budget.ProviderInputBytes}}
	for _, pair := range pairs {
		n, e := decodeDeveloperInteger(pair.raw)
		if e != nil || n < 0 {
			return DeveloperArgs{}, ErrPackage
		}
		*pair.value = n
	}
	if !developerSource(args.NeedSource) || !developerSource(args.ContractSource) || args.NeedSource.ID == args.ContractSource.ID || args.Deadline.IsZero() || args.Dependencies == nil || len(args.Dependencies) > 8 {
		return DeveloperArgs{}, ErrPackage
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
func decodeDeveloperSource(raw json.RawMessage) (catalog.SourceRef, error) {
	var wire struct {
		ID             string          `json:"source_id"`
		Revision       json.RawMessage `json:"revision"`
		AccessRevision json.RawMessage `json:"access_revision"`
		Hash           string          `json:"content_hash"`
	}
	if Decode(raw, &wire) != nil {
		return catalog.SourceRef{}, ErrPackage
	}
	revision, err := decodeDeveloperInteger(wire.Revision)
	if err != nil {
		return catalog.SourceRef{}, ErrPackage
	}
	access, err := decodeDeveloperInteger(wire.AccessRevision)
	if err != nil {
		return catalog.SourceRef{}, ErrPackage
	}
	return catalog.SourceRef{ID: wire.ID, Revision: revision, AccessRevision: access, Hash: wire.Hash}, nil
}
func decodeDeveloperInteger(raw json.RawMessage) (int64, error) {
	text := strings.TrimSpace(string(raw))
	if len(text) == 0 || len(text) > 512 || !(text[0] == '-' || text[0] >= '0' && text[0] <= '9') {
		return 0, ErrPackage
	}
	var number json.Number
	if json.Unmarshal(raw, &number) != nil {
		return 0, ErrPackage
	}
	if at := strings.IndexAny(text, "eE"); at >= 0 {
		exponent, err := strconv.Atoi(text[at+1:])
		if err != nil || exponent < -1024 || exponent > 1024 {
			return 0, ErrPackage
		}
	}
	rational, ok := new(big.Rat).SetString(text)
	if !ok || !rational.IsInt() || !rational.Num().IsInt64() {
		return 0, ErrPackage
	}
	return rational.Num().Int64(), nil
}

func developerSource(ref catalog.SourceRef) bool {
	return ref.ID != "" && len(ref.ID) <= 160 && strings.TrimSpace(ref.ID) == ref.ID && !strings.ContainsAny(ref.ID, "\x00\r\n") && ref.Revision > 0 && ref.AccessRevision > 0 && digestName.MatchString(ref.Hash)
}
