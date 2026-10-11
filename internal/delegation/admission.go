package delegation

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

const AdmissionTool = "task.delegate"
const AdmissionTarget = "broker://task-delegation-v1"

// AdmissionArgs contains proposed DATA references and finite allocation only.
// Owner, task, worker and capability identity are derived by the kernel.
type AdmissionArgs struct {
	ContractSource SourceRef   `json:"contract_source"`
	Sources        []SourceRef `json:"source_refs"`
	Budget         Limits      `json:"budget"`
	Deadline       time.Time   `json:"deadline"`
	Dependencies   []string    `json:"dependencies"`
}

// AdmissionEnvelope is private Broker control traffic, not a model tool schema.
// Every field is checked against the actual claimed action on admission.
type AdmissionEnvelope struct {
	RunID        string `json:"run_id"`
	ToolCallID   string `json:"tool_call_id"`
	EffectID     string `json:"effect_id"`
	ArgsJSON     string `json:"args_json"`
	ContractHash string `json:"contract_hash"`
}

// AdmissionReceipt acknowledges one committed child admission, not child work
// or semantic completion. It contains no source body or reusable authority.
type AdmissionReceipt struct {
	TaskID           string `json:"task_id"`
	InputID          string `json:"input_id"`
	State            string `json:"state"`
	ContractRevision int64  `json:"contract_revision"`
}

func AdmissionContract() *toolbrokerv1.ToolContract {
	ref := `{"type":"object","required":["source_id","revision","access_revision","content_hash"],"properties":{"source_id":{"type":"string"},"revision":{"type":"integer"},"access_revision":{"type":"integer"},"content_hash":{"type":"string"}},"additionalProperties":false}`
	units := `{"type":"object","required":["model_turns","tokens","cost_micros","external_calls","children","active_millis","egress_bytes","provider_input_bytes"],"properties":{"model_turns":{"type":"integer"},"tokens":{"type":"integer"},"cost_micros":{"type":"integer"},"external_calls":{"type":"integer"},"children":{"type":"integer"},"active_millis":{"type":"integer"},"egress_bytes":{"type":"integer"},"provider_input_bytes":{"type":"integer"}},"additionalProperties":false}`
	schema := `{"type":"object","required":["contract_source","source_refs","budget","deadline","dependencies"],"properties":{"contract_source":` + ref + `,"source_refs":{"type":"array","items":` + ref + `},"budget":` + units + `,"deadline":{"type":"string"},"dependencies":{"type":"array","items":{"type":"string"}}},"additionalProperties":false}`
	return &toolbrokerv1.ToolContract{ToolName: AdmissionTool, Description: "Admit one bounded independent research task from exact current Catalog DATA references. Child tools are denied. A receipt confirms admission only.", ToolClass: "task_control", InputSchemaJson: schema, OutputSchemaJson: `{"type":"object","required":["task_id","input_id","state","contract_revision"],"properties":{"task_id":{"type":"string"},"input_id":{"type":"string"},"state":{"type":"string"},"contract_revision":{"type":"integer"}},"additionalProperties":false}`, RuntimeTarget: AdmissionTarget, Status: "active", ContractRevision: "1", RiskLevel: "medium", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_runtime_ack"}}
}

// DecodeAdmissionArgs rejects ambiguous JSON and validates the bounded shape.
// Current authority and the exact canonical action hash are checked separately.
func DecodeAdmissionArgs(raw string) (AdmissionArgs, error) {
	var a AdmissionArgs
	if len(raw) > 128*1024 {
		return a, ErrDenied
	}
	if strictAdmissionJSON([]byte(raw), &a) != nil || !ValidSource(a.ContractSource) || a.Sources == nil || len(a.Sources) > 64 || a.Dependencies == nil || len(a.Dependencies) > 8 || a.Deadline.IsZero() {
		return AdmissionArgs{}, ErrDenied
	}
	var object map[string]json.RawMessage
	_ = json.Unmarshal([]byte(raw), &object)
	var units map[string]json.RawMessage
	if json.Unmarshal(object["budget"], &units) != nil {
		return AdmissionArgs{}, ErrDenied
	}
	for _, key := range []string{"model_turns", "tokens", "cost_micros", "external_calls", "children", "active_millis", "egress_bytes", "provider_input_bytes"} {
		if _, ok := units[key]; !ok {
			return AdmissionArgs{}, ErrDenied
		}
	}
	seen := map[string]bool{a.ContractSource.ID: true}
	for _, r := range a.Sources {
		if !ValidSource(r) || seen[r.ID] {
			return AdmissionArgs{}, ErrDenied
		}
		seen[r.ID] = true
	}
	seen = map[string]bool{}
	for _, id := range a.Dependencies {
		if !validID(id) || seen[id] {
			return AdmissionArgs{}, ErrDenied
		}
		seen[id] = true
	}
	for _, v := range []int64{a.Budget.ModelTurns, a.Budget.Tokens, a.Budget.CostMicros, a.Budget.ExternalCalls, a.Budget.Children, a.Budget.ActiveMillis, a.Budget.EgressBytes, a.Budget.ProviderInputBytes} {
		if v < 0 {
			return AdmissionArgs{}, ErrDenied
		}
	}
	a.Deadline = a.Deadline.UTC().Truncate(time.Microsecond)
	return a, nil
}

func strictAdmissionJSON(raw []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	var unique func(int) error
	unique = func(depth int) error {
		if depth > 16 {
			return ErrDenied
		}
		token, err := d.Token()
		if err != nil {
			return ErrDenied
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return ErrDenied
				}
				k, ok := key.(string)
				if !ok || seen[k] {
					return ErrDenied
				}
				seen[k] = true
				if unique(depth+1) != nil {
					return ErrDenied
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return ErrDenied
			}
		case json.Delim('['):
			for d.More() {
				if unique(depth+1) != nil {
					return ErrDenied
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return ErrDenied
			}
		}
		return nil
	}
	if unique(0) != nil {
		return ErrDenied
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrDenied
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		return ErrDenied
	}
	return nil
}
