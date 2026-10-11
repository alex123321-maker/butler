package localmodels

import (
	"encoding/json"
	"io"
	"net/url"
	"strings"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
)

const FunctionCatalogVersion = "butler-read-diagnostics-v1"
const FunctionModel = "functiongemma:270m-it-q8_0"
const FunctionProfile = "functiongemma-270m-q8-cpu-v1"
const FunctionRevision = "f6078f93a342963cabae694fd53e09d885801a6e82157c73f0edf67a7e879020"
const FunctionRuntime = "ollama0.40.2/cpu-q8_0"

// A small immutable experiment catalog. Runtime never executes these functions.
// Dynamic/self-created tools and schema changes require another reviewed version.
func FunctionTools() []map[string]any {
	empty := map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}
	return []map[string]any{
		{"type": "function", "function": map[string]any{"name": "check_system", "description": "Check the health of Butler system. Проверить состояние системы Butler.", "parameters": empty}},
		{"type": "function", "function": map[string]any{"name": "check_database", "description": "Check the health of Butler database. Проверить состояние базы данных Butler.", "parameters": empty}},
		{"type": "function", "function": map[string]any{"name": "read_webpage", "description": "Read one public webpage by its explicit absolute HTTPS URL. Прочитать одну публичную веб-страницу по явному HTTPS URL.", "parameters": map[string]any{"type": "object", "required": []string{"url"}, "properties": map[string]any{"url": map[string]any{"type": "string", "description": "Explicit public HTTPS URL from the command"}}, "additionalProperties": false}}},
	}
}

// NormalizeFunction rejects the complete set on any unsupported call/argument.
// No nearest-function matching, partial batch, generated dialogue or execution.
func NormalizeFunction(calls []*pb.FunctionCall) (*pb.FunctionOutput, error) {
	out := &pb.FunctionOutput{CatalogVersion: FunctionCatalogVersion, InsufficientData: len(calls) == 0}
	if len(calls) > 1 {
		return nil, fault(Protocol)
	} // only one independent command in this experiment
	for _, c := range calls {
		if c == nil || len(c.ArgumentsJson) > 4096 {
			return nil, fault(Protocol)
		}
		args, e := functionArguments(c.ArgumentsJson)
		if e != nil {
			return nil, e
		}
		switch c.Name {
		case "check_system", "check_database":
			if len(args) != 0 {
				return nil, fault(Protocol)
			}
		case "read_webpage":
			var raw string
			if len(args) != 1 || json.Unmarshal(args["url"], &raw) != nil || raw == "" || len(raw) > 2048 {
				return nil, fault(Protocol)
			}
			u, e := url.Parse(raw)
			if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" || strings.TrimSpace(raw) != raw {
				return nil, fault(Protocol)
			}
		default:
			return nil, fault(Protocol)
		}
		data, _ := json.Marshal(args)
		out.Calls = append(out.Calls, &pb.FunctionCall{Name: c.Name, ArgumentsJson: string(data)})
	}
	return out, nil
}

func functionArguments(raw string) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(strings.NewReader(raw))
	tok, e := d.Token()
	if e != nil || tok != json.Delim('{') {
		return nil, fault(Protocol)
	}
	values := map[string]json.RawMessage{}
	for d.More() {
		tok, e := d.Token()
		key, ok := tok.(string)
		if e != nil || !ok {
			return nil, fault(Protocol)
		}
		if _, duplicate := values[key]; duplicate {
			return nil, fault(Protocol)
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return nil, fault(Protocol)
		}
		values[key] = value
	}
	if tok, e = d.Token(); e != nil || tok != json.Delim('}') {
		return nil, fault(Protocol)
	}
	if _, e = d.Token(); e != io.EOF {
		return nil, fault(Protocol)
	}
	return values, nil
}
