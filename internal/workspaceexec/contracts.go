package workspaceexec

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/workspace"
	"google.golang.org/protobuf/proto"
)

const (
	CreateTool       = "workspace.create"
	ReadTool         = "workspace.read"
	WriteTool        = "workspace.write"
	ContractRevision = "1"
	MaxCodeBytes     = 64 * 1024
)

func Protected(name string) bool { return strings.HasPrefix(name, "workspace.") }

// The target is immutable operator composition, never a model argument.
func Contracts(target string) []*toolbrokerv1.ToolContract {
	snapshot := `{"type":"object","required":["path","version","sha256","bytes"],"properties":{"path":{"type":"string","minLength":1,"maxLength":512},"version":{"type":"integer","minimum":0},"sha256":{"type":"string","pattern":"^([a-f0-9]{64})?$"},"bytes":{"type":"integer","minimum":0,"maximum":65536}},"additionalProperties":false}`
	id := `{"type":"string","pattern":"^[a-f0-9]{32}$"}`
	readInput := `{"type":"object","required":["workspace_id","path"],"properties":{"workspace_id":` + id + `,"path":{"type":"string","minLength":1,"maxLength":512}},"additionalProperties":false}`
	writeInput := `{"type":"object","required":["workspace_id","path","expected_version","expected_sha256","expected_bytes","content"],"properties":{"workspace_id":` + id + `,"path":{"type":"string","minLength":1,"maxLength":512},"expected_version":{"type":"integer","minimum":0},"expected_sha256":{"type":"string","pattern":"^([a-f0-9]{64})?$"},"expected_bytes":{"type":"integer","minimum":0,"maximum":65536},"content":{"type":"string","maxLength":65536}},"additionalProperties":false}`
	base := func(name, description, input, output, retry, verification string) *toolbrokerv1.ToolContract {
		return &toolbrokerv1.ToolContract{ToolName: name, Description: description, ToolClass: "workspace", InputSchemaJson: input, OutputSchemaJson: output, RuntimeTarget: target, ContractRevision: ContractRevision, Status: "enabled", RiskLevel: "medium", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: retry, VerificationMethod: verification}}
	}
	return []*toolbrokerv1.ToolContract{
		base(CreateTool, "Prepare one empty isolated code copy for this task. This grants no shell, credential or execution permission.", `{"type":"object","properties":{},"additionalProperties":false}`, `{"type":"object","required":["workspace_id"],"properties":{"workspace_id":`+id+`},"additionalProperties":false}`, "unsafe", "local_runtime_ack"),
		base(ReadTool, "Read bounded current text and its file version/hash from this task's code copy, under current original-source access.", readInput, `{"type":"object","required":["workspace_id","snapshot","content"],"properties":{"workspace_id":`+id+`,"snapshot":`+snapshot+`,"content":{"type":"string","maxLength":65536}},"additionalProperties":false}`, "read_only", "read_result"),
		base(WriteTool, "Prepare one bounded text artifact using the exact previously observed file version/hash/size. Conflicting changes are denied; no code executes.", writeInput, `{"type":"object","required":["workspace_id","snapshot"],"properties":{"workspace_id":`+id+`,"snapshot":`+snapshot+`},"additionalProperties":false}`, "unsafe", "local_runtime_ack"),
	}
}
func ExactContract(c *toolbrokerv1.ToolContract) bool {
	if c == nil || c.GetRuntimeTarget() == "" {
		return false
	}
	for _, expected := range Contracts(c.GetRuntimeTarget()) {
		if proto.Equal(c, expected) {
			return true
		}
	}
	return false
}

// Args contains only DATA and file CAS inputs. Identity/scope, host root, lease,
// original provenance, ticket, credential and successful verdict are private.
type Args struct {
	WorkspaceID, Path string
	Expected          workspace.Snapshot
	Content           []byte
}

func fields(raw string, keys ...string) (map[string]json.RawMessage, error) {
	if len(raw) > 256*1024 || !utf8.ValidString(raw) {
		return nil, ErrDenied
	}
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	d := json.NewDecoder(strings.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrDenied
	}
	out := map[string]json.RawMessage{}
	for d.More() {
		token, err = d.Token()
		k, ok := token.(string)
		if err != nil || !ok || !want[k] || out[k] != nil {
			return nil, ErrDenied
		}
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(value, []byte("null")) {
			return nil, ErrDenied
		}
		out[k] = value
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
func opaqueID(id string) bool {
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16 && hex.EncodeToString(decoded) == id
}
func sha256Digest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && hex.EncodeToString(decoded) == value
}

func DecodeArgs(tool, raw string) (Args, error) {
	var keys []string
	switch tool {
	case CreateTool:
		_, err := fields(raw)
		return Args{}, err
	case ReadTool:
		keys = []string{"workspace_id", "path"}
	case WriteTool:
		keys = []string{"workspace_id", "path", "expected_version", "expected_sha256", "expected_bytes", "content"}
	default:
		return Args{}, ErrDenied
	}
	o, err := fields(raw, keys...)
	if err != nil {
		return Args{}, err
	}
	var a Args
	if json.Unmarshal(o["workspace_id"], &a.WorkspaceID) != nil || !opaqueID(a.WorkspaceID) || json.Unmarshal(o["path"], &a.Path) != nil || !workspace.ValidCodePath(a.Path) {
		return Args{}, ErrDenied
	}
	a.Expected.Path = a.Path
	if tool == ReadTool {
		return a, nil
	}
	var content string
	if json.Unmarshal(o["expected_version"], &a.Expected.Version) != nil || a.Expected.Version < 0 || json.Unmarshal(o["expected_sha256"], &a.Expected.SHA256) != nil || json.Unmarshal(o["expected_bytes"], &a.Expected.Bytes) != nil || a.Expected.Bytes < 0 || a.Expected.Bytes > MaxCodeBytes || json.Unmarshal(o["content"], &content) != nil || len(content) > MaxCodeBytes || !utf8.ValidString(content) || strings.ContainsRune(content, 0) || sanitize.Text(content) != content {
		return Args{}, ErrDenied
	}
	if a.Expected.Version == 0 {
		if a.Expected.SHA256 != "" || a.Expected.Bytes != 0 {
			return Args{}, ErrDenied
		}
	} else if !sha256Digest(a.Expected.SHA256) {
		return Args{}, ErrDenied
	}
	a.Content = []byte(content)
	return a, nil
}
