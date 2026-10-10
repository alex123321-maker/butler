package actionauth

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

const maxArgumentBytes = 256 * 1024

// ArgumentsHash uses the same unambiguous representation as prepared actions.
func ArgumentsHash(raw string) (string, error) {
	canonical, err := canonicalJSON(raw)
	if err != nil {
		return "", err
	}
	return digest(canonical), nil
}

func canonicalJSON(raw string) (string, error) {
	if len(raw) > maxArgumentBytes {
		return "", fmt.Errorf("action arguments exceed the supported limit")
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeUnique(decoder, 0)
	if err != nil {
		return "", fmt.Errorf("action arguments must be valid JSON")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return "", fmt.Errorf("action arguments must contain one JSON value")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("action arguments cannot be normalized")
	}
	return string(encoded), nil
}

// Reject duplicate keys before parsing can discard one interpretation. The
// runtime, schema validator and authorization hash must see the same object.
func decodeUnique(d *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("JSON nesting exceeds limit")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		object := map[string]any{}
		for d.More() {
			keyToken, err := d.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, fmt.Errorf("invalid JSON key")
			}
			if _, exists := object[key]; exists {
				return nil, fmt.Errorf("duplicate JSON key")
			}
			value, err := decodeUnique(d, depth+1)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return nil, fmt.Errorf("invalid JSON object")
		}
		return object, nil
	case json.Delim('['):
		values := []any{}
		for d.More() {
			value, err := decodeUnique(d, depth+1)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return nil, fmt.Errorf("invalid JSON array")
		}
		return values, nil
	}
	if _, ok := token.(json.Delim); ok {
		return nil, fmt.Errorf("invalid JSON delimiter")
	}
	return token, nil
}

func BindCredentialMetadata(binding ActionBinding, hash string) ActionBinding {
	binding.CredentialMetadataHash = hash
	binding.ActionHash = ""
	encoded, _ := json.Marshal(binding)
	binding.ActionHash = digest(string(encoded))
	return binding
}
func digest(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}

func ContractHash(contract *toolbrokerv1.ToolContract) (string, error) {
	if contract == nil || contract.GetToolName() == "" || (contract.GetStatus() != "active" && contract.GetStatus() != "enabled") {
		return "", fmt.Errorf("active compatible tool contract is required")
	}
	input, err := canonicalJSON(contract.GetInputSchemaJson())
	if err != nil {
		return "", fmt.Errorf("tool input contract is invalid")
	}
	output := "{}"
	if contract.GetOutputSchemaJson() != "" {
		output, err = canonicalJSON(contract.GetOutputSchemaJson())
		if err != nil {
			return "", fmt.Errorf("tool output contract is invalid")
		}
	}
	domains := append([]string(nil), contract.GetAllowedDomains()...)
	for i, value := range domains {
		domains[i] = strings.ToLower(strings.TrimSpace(value))
	}
	sort.Strings(domains)
	tools := append([]string(nil), contract.GetAllowedTools()...)
	sort.Strings(tools)
	encoded, _ := json.Marshal(struct {
		Name, Class, Input, Output, Target, Risk, Status string
		CredentialRefs, Approval                         bool
		Domains, Tools                                   []string
		Revision                                         string                       `json:",omitempty"`
		EffectContract                                   *toolbrokerv1.EffectContract `json:",omitempty"`
	}{contract.GetToolName(), contract.GetToolClass(), input, output, contract.GetRuntimeTarget(), contract.GetRiskLevel(), contract.GetStatus(), contract.GetSupportsCredentialRefs(), contract.GetRequiresApproval(), domains, tools, contract.GetContractRevision(), contract.GetEffectContract()})
	return digest(string(encoded)), nil
}

func BuildBinding(actionID string, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract, context ContextSnapshot) (ActionBinding, string, error) {
	if call == nil || actionID == "" || call.GetRunId() == "" || call.GetToolCallId() == "" || context.RunID != call.GetRunId() || context.OwnerID == "" {
		return ActionBinding{}, "", fmt.Errorf("trusted action identity and owner are required")
	}
	contractHash, err := ContractHash(contract)
	if err != nil {
		return ActionBinding{}, "", err
	}
	if contract.GetToolName() != call.GetToolName() {
		return ActionBinding{}, "", fmt.Errorf("tool contract does not match the action")
	}
	args, err := canonicalJSON(call.GetArgsJson())
	if err != nil {
		return ActionBinding{}, "", err
	}
	var object map[string]any
	if json.Unmarshal([]byte(args), &object) != nil || object == nil {
		return ActionBinding{}, "", fmt.Errorf("action arguments must be an object")
	}
	refs, err := argumentRefs(object)
	if err != nil {
		return ActionBinding{}, "", err
	}
	provided := make([]CredentialRef, 0, len(call.GetCredentialRefs()))
	for _, ref := range call.GetCredentialRefs() {
		if ref == nil || ref.GetType() != "credential_ref" {
			return ActionBinding{}, "", fmt.Errorf("invalid credential reference")
		}
		provided = append(provided, CredentialRef{ref.GetAlias(), ref.GetField()})
	}
	provided = normalizeRefs(provided)
	want, _ := json.Marshal(refs)
	got, _ := json.Marshal(provided)
	if string(want) != string(got) {
		return ActionBinding{}, "", fmt.Errorf("credential references do not match action arguments")
	}
	if len(refs) > 0 && !contract.GetSupportsCredentialRefs() {
		return ActionBinding{}, "", fmt.Errorf("tool contract does not support credential references")
	}
	resource, err := actionResource(call.GetToolName(), object)
	if err != nil {
		return ActionBinding{}, "", err
	}
	binding := ActionBinding{ActionID: actionID, RunID: call.GetRunId(), ToolCallID: call.GetToolCallId(), ToolName: call.GetToolName(), ContractHash: contractHash, ContractRevision: contract.GetContractRevision(), ArgsHash: digest(args), Resource: resource, CredentialRefs: refs, TaskID: context.TaskID, TaskRevision: context.TaskRevision, ConstraintsRevision: context.ConstraintsRevision, OwnerID: context.OwnerID, Effect: classifyEffect(call.GetToolName(), object)}
	if a := context.Authority; a != nil {
		if a.TaskID != context.TaskID || a.OwnerID != context.OwnerID {
			return ActionBinding{}, "", ErrOwnerAuthority
		}
		binding.GoalID, binding.GoalRevision, binding.MandateID, binding.MandateRevision, binding.Purpose = a.GoalID, a.GoalRevision, a.MandateID, a.MandateRevision, a.Purpose
	}
	encoded, _ := json.Marshal(binding)
	binding.ActionHash = digest(string(encoded))
	return binding, args, nil
}

func argumentRefs(value any) ([]CredentialRef, error) {
	refs := []CredentialRef{}
	var walk func(any) error
	walk = func(value any) error {
		switch value := value.(type) {
		case map[string]any:
			if value["type"] == "credential_ref" {
				alias, _ := value["alias"].(string)
				field, _ := value["field"].(string)
				if alias == "" || field == "" {
					return fmt.Errorf("credential reference requires alias and field")
				}
				refs = append(refs, CredentialRef{alias, field})
				return nil
			}
			for _, nested := range value {
				if err := walk(nested); err != nil {
					return err
				}
			}
		case []any:
			for _, nested := range value {
				if err := walk(nested); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(value); err != nil {
		return nil, err
	}
	return normalizeRefs(refs), nil
}
func normalizeRefs(refs []CredentialRef) []CredentialRef {
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Alias == refs[j].Alias {
			return refs[i].Field < refs[j].Field
		}
		return refs[i].Alias < refs[j].Alias
	})
	result := []CredentialRef{}
	for _, ref := range refs {
		if len(result) == 0 || result[len(result)-1] != ref {
			result = append(result, ref)
		}
	}
	return result
}
func actionResource(tool string, args map[string]any) (string, error) {
	raw, _ := args["url"].(string)
	if strings.HasPrefix(tool, "browser.") {
		if observed, _ := args["expected_url"].(string); observed != "" {
			raw = observed
		}
	}
	if raw != "" {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return "", fmt.Errorf("action resource must be an HTTP(S) URL without credentials")
		}
		raw = u.String()
	}
	if id, _ := args["browser_session_id"].(string); id != "" {
		return "browser:" + id + ":" + raw, nil
	}
	if id, _ := args["session_id"].(string); id != "" && strings.HasPrefix(tool, "single_tab.") {
		return "single_tab:" + id + ":" + raw, nil
	}
	if raw != "" {
		return raw, nil
	}
	return "tool:" + tool, nil
}
func classifyEffect(tool string, args map[string]any) Effect {
	switch tool {
	case "http.request":
		method, _ := args["method"].(string)
		method = strings.ToUpper(method)
		if method == "" || method == "GET" || method == "HEAD" || method == "OPTIONS" {
			return EffectRead
		}
		if method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE" {
			return EffectCommit
		}
		return EffectUnknown
	case "browser.snapshot", "browser.extract_text", "browser.wait_for", "browser.close", "single_tab.status", "single_tab.capture_visible", "single_tab.extract_text", "doctor.check_system", "doctor.check_container", "doctor.check_database", "doctor.check_provider", "web.fetch", "web.fetch_batch", "web.extract", "http.download", "http.parse_html":
		return EffectRead
	case "single_tab.bind", "browser.set_cookie", "browser.restore_storage_state", "doctor.generate_report":
		return EffectPrepare
	// Live-page interactions can submit, purchase or autosave through site JS;
	// their effect is opaque even when the tool name looks like preparation.
	default:
		return EffectUnknown
	}
}
