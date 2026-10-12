package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCloudOfflineCLIExportsOpaqueInputsAndFrozenPolicyWithoutProvider(t *testing.T) {
	var out bytes.Buffer
	root := filepath.Join("..", "..")
	if err := run([]string{"-root", root, "-mode", "inputs"}, &out); err != nil {
		t.Fatal(err)
	}
	var input struct {
		Cases   []json.RawMessage `json:"cases"`
		Blocked []string          `json:"blocked_case_ids"`
	}
	if err := json.Unmarshal(out.Bytes(), &input); err != nil || len(input.Cases) != 236 || len(input.Blocked) != 4 {
		t.Fatal(err, len(input.Cases), len(input.Blocked))
	}
	for _, forbidden := range []string{`"golden"`, `-important`, `-quiet`, `-ambiguous`} {
		if strings.Contains(out.String(), forbidden) {
			t.Fatal("expected labels leaked", forbidden)
		}
	}
	out.Reset()
	artifact := filepath.Join(t.TempDir(), "policy.json")
	if err := run([]string{"-root", root, "-mode", "policy", "-output", artifact}, &out); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(artifact)
	if err := run([]string{"-root", root, "-mode", "policy", "-output", artifact}, &out); err == nil {
		t.Fatal("frozen policy overwritten")
	}
	after, _ := os.ReadFile(artifact)
	if !bytes.Equal(before, after) {
		t.Fatal("old policy changed")
	}
	if err := run([]string{"-root", root, "-mode", "quality"}, &out); err == nil {
		t.Fatal("unreviewed/unpinned measurements accepted")
	}
	out.Reset()
	if err := run([]string{"-root", root, "-mode", "window-audit", "-observations", "untrusted"}, &out); err == nil || out.Len() != 0 {
		t.Fatal("untrusted window exported without external expectations")
	}
}
