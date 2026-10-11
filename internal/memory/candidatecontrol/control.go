// Package candidatecontrol defines one private owning control for neutral
// candidate proposals. It makes no acceptance, extraction or access decision.
package candidatecontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

const Tool = "memory.propose_candidate"
const Target = "broker://memory-candidates-v1"
const Path = "/internal/memory-candidate"
const maxEnvelope = 256 * 1024

var ErrDenied = errors.New("memory candidate proposal denied")

type Args struct {
	SourceID       string
	SourceRevision int64
	Key            string
	Value          json.RawMessage
	Summary        string
}
type Envelope struct {
	RunID        string `json:"run_id"`
	ToolCallID   string `json:"tool_call_id"`
	EffectID     string `json:"effect_id"`
	ArgsJSON     string `json:"args_json"`
	ContractHash string `json:"contract_hash"`
}
type Receipt struct {
	ClaimID string `json:"claim_id"`
	Status  string `json:"status"`
}
type Service interface {
	ProposeDispatched(context.Context, Envelope) (Receipt, error)
}

func Contract() *toolbrokerv1.ToolContract {
	return &toolbrokerv1.ToolContract{ToolName: Tool, Description: "Propose one DATA candidate from an exact current source already pinned by this run. The owner separately reviews acceptance and conflicts. This never changes accepted memory, rules or permissions.", ToolClass: "memory_control", InputSchemaJson: `{"type":"object","required":["source_id","source_revision","fact_key","value_json","summary"],"properties":{"source_id":{"type":"string","minLength":1,"maxLength":128},"source_revision":{"type":"integer","minimum":1},"fact_key":{"type":"string","minLength":1,"maxLength":512},"value_json":{"type":"string","minLength":1,"maxLength":32768},"summary":{"type":"string","maxLength":8192}},"additionalProperties":false}`, OutputSchemaJson: `{"type":"object","required":["claim_id","status"],"properties":{"claim_id":{"type":"string"},"status":{"type":"string","enum":["candidate"]}},"additionalProperties":false}`, RuntimeTarget: Target, ContractRevision: "1", Status: "active", RiskLevel: "medium", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_runtime_ack"}}
}

func flat(raw []byte, limit int, keys ...string) (map[string]json.RawMessage, error) {
	if len(raw) > limit {
		return nil, ErrDenied
	}
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrDenied
	}
	out := map[string]json.RawMessage{}
	for d.More() {
		token, err := d.Token()
		k, ok := token.(string)
		if err != nil || !ok || !want[k] || out[k] != nil {
			return nil, ErrDenied
		}
		var v json.RawMessage
		if d.Decode(&v) != nil || bytes.Equal(v, []byte("null")) {
			return nil, ErrDenied
		}
		out[k] = v
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') || len(out) != len(want) {
		return nil, ErrDenied
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, ErrDenied
	}
	return out, nil
}
func textField(raw json.RawMessage, max int, empty bool) (string, error) {
	var s string
	if json.Unmarshal(raw, &s) != nil || len(s) > max || !empty && strings.TrimSpace(s) == "" {
		return "", ErrDenied
	}
	return s, nil
}
func DecodeArgs(raw string) (Args, error) {
	o, err := flat([]byte(raw), 64*1024, "source_id", "source_revision", "fact_key", "value_json", "summary")
	if err != nil {
		return Args{}, err
	}
	var a Args
	if a.SourceID, err = textField(o["source_id"], 128, false); err != nil {
		return Args{}, err
	}
	if json.Unmarshal(o["source_revision"], &a.SourceRevision) != nil || a.SourceRevision < 1 {
		return Args{}, ErrDenied
	}
	if a.Key, err = textField(o["fact_key"], 512, false); err != nil {
		return Args{}, err
	}
	value, err := textField(o["value_json"], 32*1024, false)
	if err != nil || !json.Valid([]byte(value)) {
		return Args{}, ErrDenied
	}
	a.Value = json.RawMessage(value)
	if a.Summary, err = textField(o["summary"], 8192, true); err != nil {
		return Args{}, err
	}
	return a, nil
}
func decodeEnvelope(raw []byte) (Envelope, error) {
	o, err := flat(raw, maxEnvelope, "run_id", "tool_call_id", "effect_id", "args_json", "contract_hash")
	if err != nil {
		return Envelope{}, err
	}
	var e Envelope
	for k, dst := range map[string]*string{"run_id": &e.RunID, "tool_call_id": &e.ToolCallID, "effect_id": &e.EffectID, "args_json": &e.ArgsJSON, "contract_hash": &e.ContractHash} {
		max := 512
		if k == "args_json" {
			max = 64 * 1024
		}
		if *dst, err = textField(o[k], max, false); err != nil {
			return Envelope{}, err
		}
	}
	if _, err = DecodeArgs(e.ArgsJSON); err != nil || len(e.ContractHash) != 64 {
		return Envelope{}, ErrDenied
	}
	return e, nil
}
func decodeReceipt(raw []byte) (Receipt, error) {
	o, err := flat(raw, 1024, "claim_id", "status")
	if err != nil {
		return Receipt{}, err
	}
	id, err := textField(o["claim_id"], 46, false)
	if err != nil || len(id) != 46 || !strings.HasPrefix(id, "claim_") {
		return Receipt{}, ErrDenied
	}
	for _, c := range id[6:] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return Receipt{}, ErrDenied
		}
	}
	status, err := textField(o["status"], 16, false)
	if err != nil || status != "candidate" {
		return Receipt{}, ErrDenied
	}
	return Receipt{id, status}, nil
}
