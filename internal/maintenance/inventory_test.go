package maintenance

import (
	"encoding/json"
	"strings"
	"testing"
)

func publicSource(name, body string) SourceFile {
	return SourceFile{Path: name, SHA256: Digest([]byte(body)), Bytes: int64(len(body)), Mode: "100644"}
}

func TestMeasuredInventoryChanges(t *testing.T) {
	old := []SourceFile{publicSource("modules/weather/main.go", "before"), publicSource("modules/weather/deleted.go", "deleted"), publicSource("go.mod", "module")}
	current := []SourceFile{publicSource("go.mod", "module"), publicSource("modules/weather/main.go", "after"), publicSource("modules/weather/new.go", "new")}
	rules := []Boundary{{Prefix: "modules/weather/", Class: Application}}
	diff, err := CompareInventory(old, current, rules)
	if err != nil || len(diff.Changes) != 3 || diff.Class != Application {
		t.Fatal("actual changed/add/deleted inventory not measured", diff, err)
	}
	if diff.Changes[0].Path != "modules/weather/deleted.go" || diff.Changes[0].Before == nil || diff.Changes[0].After != nil || diff.Changes[2].Before != nil || diff.Changes[2].After == nil {
		t.Fatal("addition/deletion provenance lost", diff)
	}
	reordered, err := CompareInventory([]SourceFile{old[2], old[1], old[0]}, []SourceFile{current[2], current[0], current[1]}, rules)
	one, _ := json.Marshal(diff)
	two, _ := json.Marshal(reordered)
	if err != nil || string(one) != string(two) {
		t.Fatal("inventory order changed the measured binding", err)
	}
	current[0] = publicSource("go.mod", "changed dependency")
	diff, err = CompareInventory(old, current, rules)
	if err != nil || diff.Class != Kernel {
		t.Fatal("mixed dependency change escaped protection", err)
	}
}

func TestProtectedAndUnknownInventory(t *testing.T) {
	rules := []Boundary{{Prefix: "modules/weather/", Class: Tool}}
	for _, name := range []string{"internal/actionauth/policy.go", "apps/orchestrator/internal/new-policy/handler.go", "deploy/compose.yaml", "scripts/checker.go", "modules/unknown/main.go", "MODULES/weather/main.go"} {
		before := []SourceFile{publicSource(name, "before")}
		after := []SourceFile{publicSource(name, "after")}
		diff, err := CompareInventory(before, after, rules)
		if err != nil || diff.Class != Kernel {
			t.Fatal("unmatched/protected path admitted as ordinary", name, err)
		}
	}
	for _, prefix := range []string{"internal/", "apps/orchestrator/", "deploy/", "scripts/", "../modules/", "/modules/", "C:/modules/", "modules\\weather/"} {
		if _, err := CompareInventory([]SourceFile{publicSource("go.mod", "old")}, []SourceFile{publicSource("go.mod", "new")}, []Boundary{{Prefix: prefix, Class: Application}}); err == nil {
			t.Fatal("operator boundary weakened a protected/invalid path", prefix)
		}
	}
}

func TestNonregularAndModeChangeInventory(t *testing.T) {
	rules := []Boundary{{Prefix: "modules/weather/", Class: Application}}
	before := []SourceFile{publicSource("modules/weather/worker", "same")}
	after := append([]SourceFile(nil), before...)
	after[0].Mode = "100755"
	diff, err := CompareInventory(before, after, rules)
	if err != nil || len(diff.Changes) != 1 || diff.Class != Application {
		t.Fatal("executable-mode change missing", err)
	}
	for _, mode := range []string{"120000", "160000"} {
		after[0].Mode = mode
		diff, err := CompareInventory(before, after, rules)
		if err != nil || diff.Class != Kernel {
			t.Fatal("link/submodule change became ordinary", mode, err)
		}
	}
}

func TestAmbiguousInventoryDenied(t *testing.T) {
	valid := publicSource("modules/weather/main.go", "before")
	for _, bad := range []SourceFile{
		publicSource("../main.go", "bad"), publicSource("/main.go", "bad"), publicSource("modules//main.go", "bad"),
		publicSource("modules/.git/config", "bad"), publicSource("modules/main.go:stream", "bad"), publicSource("modules/\nmain.go", "bad"),
		{Path: valid.Path, SHA256: strings.ToUpper(valid.SHA256), Bytes: valid.Bytes, Mode: valid.Mode},
		{Path: valid.Path, SHA256: valid.SHA256, Bytes: -1, Mode: valid.Mode},
		{Path: valid.Path, SHA256: valid.SHA256, Bytes: valid.Bytes, Mode: "directory"},
	} {
		if _, err := CompareInventory([]SourceFile{valid}, []SourceFile{bad}, nil); err == nil {
			t.Fatal("ambiguous inventory accepted", bad.Path)
		}
	}
	for _, duplicate := range []SourceFile{valid, publicSource("MODULES/weather/main.go", "alias")} {
		if _, err := CompareInventory([]SourceFile{valid}, []SourceFile{valid, duplicate}, nil); err == nil {
			t.Fatal("duplicate or case-aliased inventory accepted")
		}
	}
	if _, err := CompareInventory([]SourceFile{valid}, []SourceFile{valid}, nil); err == nil {
		t.Fatal("no-op source inventory accepted as repair")
	}
	if _, err := CompareInventory([]SourceFile{valid}, []SourceFile{publicSource(valid.Path, "after")}, []Boundary{{"modules/", Tool}, {"modules/weather/", Application}}); err == nil {
		t.Fatal("overlapping classification boundaries accepted")
	}
}
