package transport

import "testing"

func TestToolSetRejectsChangedRevisionAndDuplicateMethods(t *testing.T) {
	s := NewToolSetSnapshot([]ToolDefinition{{Name: "one", SchemaJSON: `{"type":"object"}`}})
	if s.Validate() != nil {
		t.Fatal("valid set rejected")
	}
	s.Definitions[0].Name = "two"
	if s.Validate() == nil {
		t.Fatal("mutated revision accepted")
	}
	s = NewToolSetSnapshot([]ToolDefinition{{Name: "one", SchemaJSON: `{}`}, {Name: "one", SchemaJSON: `{}`}})
	if s.Validate() == nil {
		t.Fatal("ambiguous set accepted")
	}
}
