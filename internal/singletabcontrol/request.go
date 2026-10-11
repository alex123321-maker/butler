package singletabcontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

const RequestTarget = "broker://guarded-tab-request-v1"

// RequestEnvelope is private Broker traffic. Identity is proved from the
// permanent dispatch, never from model args or an owner HTTP request.
type RequestEnvelope struct {
	RunID        string `json:"run_id"`
	ToolCallID   string `json:"tool_call_id"`
	EffectID     string `json:"effect_id"`
	ArgsJSON     string `json:"args_json"`
	ContractHash string `json:"contract_hash"`
}
type RequestReceipt struct {
	ApprovalID string `json:"approval_id"`
	State      string `json:"state"`
}
type RequestService interface {
	RequestDispatched(context.Context, RequestEnvelope) (RequestReceipt, error)
}

func RequestContract() *toolbrokerv1.ToolContract {
	return &toolbrokerv1.ToolContract{ToolName: BindTool, Description: "Ask the current owner to select one browser tab. Execution pauses for a finite owner choice. This does not perform a browser action or authorize one.", ToolClass: "task_control", InputSchemaJson: `{"type":"object","properties":{},"additionalProperties":false}`, OutputSchemaJson: `{"type":"object","required":["approval_id","state"],"properties":{"approval_id":{"type":"string"},"state":{"type":"string"}},"additionalProperties":false}`, RuntimeTarget: RequestTarget, ContractRevision: "1", Status: "active", RiskLevel: "medium", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_runtime_ack"}}
}

func ValidRequestArgs(raw string) bool {
	if len(raw) > 8192 {
		return false
	}
	var fields map[string]json.RawMessage
	return json.Unmarshal([]byte(raw), &fields) == nil && fields != nil && len(fields) == 0
}

// Fixed flat string objects reject null, aliases, duplicate/escaped keys,
// trailing JSON and nested values before any authority or metadata operation.
func decodeRequestObject(raw []byte, keys ...string) (map[string]string, error) {
	if len(raw) > 8192 {
		return nil, ErrDenied
	}
	expected := map[string]bool{}
	for _, k := range keys {
		expected[k] = true
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	start, err := d.Token()
	if err != nil || start != json.Delim('{') {
		return nil, ErrDenied
	}
	out := map[string]string{}
	for d.More() {
		token, err := d.Token()
		k, ok := token.(string)
		if err != nil || !ok || !expected[k] {
			return nil, ErrDenied
		}
		if _, duplicate := out[k]; duplicate {
			return nil, ErrDenied
		}
		var v string
		if d.Decode(&v) != nil || v == "" || len(v) > 512 {
			return nil, ErrDenied
		}
		out[k] = v
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') || len(out) != len(keys) {
		return nil, ErrDenied
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, ErrDenied
	}
	return out, nil
}
func decodeRequestEnvelope(raw []byte) (RequestEnvelope, error) {
	o, err := decodeRequestObject(raw, "run_id", "tool_call_id", "effect_id", "args_json", "contract_hash")
	if err != nil || !ValidRequestArgs(o["args_json"]) {
		return RequestEnvelope{}, ErrDenied
	}
	return RequestEnvelope{o["run_id"], o["tool_call_id"], o["effect_id"], o["args_json"], o["contract_hash"]}, nil
}
func decodeRequestReceipt(raw []byte) (RequestReceipt, error) {
	o, err := decodeRequestObject(raw, "approval_id", "state")
	if err != nil || o["state"] != "awaiting_owner_choice" || len(o["approval_id"]) != 48 {
		return RequestReceipt{}, ErrDenied
	}
	for _, c := range o["approval_id"] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return RequestReceipt{}, ErrDenied
		}
	}
	return RequestReceipt{o["approval_id"], o["state"]}, nil
}
