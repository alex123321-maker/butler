package workspaceexec

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWorkspaceContractsKeepOnlyDataAndFileCAS(t *testing.T) {
	for _, c := range Contracts("127.0.0.1:50055") {
		if !ExactContract(c) || !json.Valid([]byte(c.InputSchemaJson)) || !json.Valid([]byte(c.OutputSchemaJson)) {
			t.Fatal("invalid fixed workspace contract")
		}
		c.RequiresApproval = true
		if ExactContract(c) {
			t.Fatal("changed contract accepted")
		}
	}
	const id = "0123456789abcdef0123456789abcdef"
	read := `{"workspace_id":"` + id + `","path":"src/main.go"}`
	if a, err := DecodeArgs(ReadTool, read); err != nil || a.WorkspaceID != id || a.Path != "src/main.go" {
		t.Fatal("bounded read denied", err)
	}
	write := `{"workspace_id":"` + id + `","path":"src/main.go","expected_version":0,"expected_sha256":"","expected_bytes":0,"content":"package main"}`
	if a, err := DecodeArgs(WriteTool, write); err != nil || string(a.Content) != "package main" || a.Expected.Path != a.Path {
		t.Fatal("new-file CAS denied", err)
	}
	for _, raw := range []string{
		strings.Replace(write, `"expected_version":0`, `"expected_version":1`, 1),
		strings.Replace(write, `"expected_bytes":0`, `"expected_bytes":1`, 1),
		strings.Replace(write, `"expected_version":0`, `"expected_version":-1`, 1),
		strings.Replace(write, `"expected_version":0`, `"expected_version":1.0`, 1),
		strings.Replace(write, `"content":"package main"`, `"content":null`, 1),
		strings.Replace(write, `"content":"package main"`, `"content":"package\u0000main"`, 1),
		strings.Replace(write, `"path":"src/main.go"`, `"path":".env"`, 1),
		strings.Replace(write, `"path":"src/main.go"`, `"path":"private/tmp/file"`, 1),
		strings.Replace(write, `"path":"src/main.go"`, `"path":"../main.go"`, 1),
		strings.Replace(write, `"workspace_id"`, `"Workspace_ID"`, 1),
		strings.Replace(write, `"content":"package main"`, `"content":"package main","content":"package alternate"`, 1),
		strings.Replace(write, `"content":"package main"`, `"content":"package main","lease_id":"untrusted"`, 1),
		strings.Replace(write, `"content":"package main"`, `"content":"package main","credential_ref":"untrusted"`, 1),
		write + ` {}`,
	} {
		if _, err := DecodeArgs(WriteTool, raw); err == nil {
			t.Fatal("ambiguous/private/stale workspace input admitted")
		}
	}
	for _, raw := range []string{`{"owner_id":"foreign"}`, `{"root":"/private"}`, `{"source_id":"untrusted"}`, `{"ticket":"untrusted"}`, `null`, `[]`, `{} {}`} {
		if _, err := DecodeArgs(CreateTool, raw); err == nil {
			t.Fatal("model selected creation authority")
		}
	}
	if _, err := DecodeArgs(CreateTool, `{}`); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeArgs("workspace.execute", write); err == nil {
		t.Fatal("code execution tool invented")
	}
}
