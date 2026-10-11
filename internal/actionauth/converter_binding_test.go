package actionauth

import (
	"strings"
	"testing"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/protobuf/proto"
)

func TestGeneratedConverterBindingPinsCatalogAndEveryDataRevision(t *testing.T) {
	meta := &toolbrokerv1.ConverterContract{ConverterId: "inventory", PackageHash: strings.Repeat("a", 64), NeedHash: strings.Repeat("b", 64), Version: "1.0.0", Generation: 2, ProfilePin: strings.Repeat("c", 64), Class: "delimited_json_v1", InputSchemaHash: strings.Repeat("d", 64), OutputSchemaHash: strings.Repeat("e", 64), InputBytes: 8192, OutputBytes: 32768, Rows: 128, FieldBytes: 1024, Millis: 1000}
	c := &toolbrokerv1.ToolContract{ToolName: "converter.inventory.convert", ToolClass: "pure_converter", RuntimeTarget: "broker://pure-converters-v1", Status: "active", ContractRevision: "1", InputSchemaJson: `{"type":"object"}`, OutputSchemaJson: `{"type":"object"}`, Converter: meta}
	call := &toolbrokerv1.ToolCall{ToolName: c.ToolName, RunId: "run", ToolCallId: "call", ArgsJson: `{"input_ref":{"source_id":"source_input","revision":1,"access_revision":1,"content_hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`, AutonomyMode: commonv1.AutonomyMode(3)}
	ctx := ContextSnapshot{RunID: "run", OwnerID: "owner", TaskID: "task", TaskRevision: 1, ConstraintsRevision: 1}
	b, _, e := BuildBinding("action", call, c, ctx)
	if e != nil || b.Effect != EffectPrepare || b.ConverterGeneration != 2 {
		t.Fatal("invalid binding", e)
	}
	refs := []DataRef{{ID: "source_program", Revision: 1, AccessRevision: 2, Hash: strings.Repeat("a", 64)}, {ID: "source_input", Revision: 3, AccessRevision: 4, Hash: strings.Repeat("b", 64)}}
	pinned, e := BindDataSources(b, refs)
	if e != nil {
		t.Fatal(e)
	}
	reordered, e := BindDataSources(b, []DataRef{refs[1], refs[0]})
	if e != nil || reordered.ActionHash != pinned.ActionHash {
		t.Fatal("source ordering changed exact intent", e)
	}
	for _, mutate := range []func(*DataRef){func(r *DataRef) { r.Revision++ }, func(r *DataRef) { r.AccessRevision++ }, func(r *DataRef) { r.Hash = strings.Repeat("f", 64) }, func(r *DataRef) { r.ID = "source_other" }} {
		next := append([]DataRef(nil), refs...)
		mutate(&next[0])
		changed, e := BindDataSources(b, next)
		if e != nil || changed.ActionHash == pinned.ActionHash {
			t.Fatal("source mutation not pinned", e)
		}
	}
	for _, mutate := range []func(*toolbrokerv1.ConverterContract){func(m *toolbrokerv1.ConverterContract) { m.Generation++ }, func(m *toolbrokerv1.ConverterContract) { m.ProfilePin = strings.Repeat("f", 64) }, func(m *toolbrokerv1.ConverterContract) { m.PackageHash = strings.Repeat("f", 64) }, func(m *toolbrokerv1.ConverterContract) { m.Rows++ }} {
		next := proto.Clone(c).(*toolbrokerv1.ToolContract)
		mutate(next.Converter)
		changed, _, e := BuildBinding("action", call, next, ctx)
		if e != nil || changed.ActionHash == b.ActionHash {
			t.Fatal("catalog permission mutation not pinned", e)
		}
	}
	for _, invalid := range [][]DataRef{{refs[0], refs[0]}, {{ID: " source", Revision: 1, AccessRevision: 1, Hash: refs[0].Hash}}, {{ID: "source", Revision: 0, AccessRevision: 1, Hash: refs[0].Hash}}} {
		if _, e = BindDataSources(b, invalid); e == nil {
			t.Fatal("ambiguous source metadata accepted")
		}
	}
	next := proto.Clone(c).(*toolbrokerv1.ToolContract)
	next.Converter.Class = "shell"
	if _, _, e = BuildBinding("action", call, next, ctx); e == nil {
		t.Fatal("new process powers from name")
	}
}
