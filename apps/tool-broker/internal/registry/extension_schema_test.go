package registry

import "testing"

func TestOneOfRejectsAmbiguousValidation(t *testing.T) {
	schema := `{"oneOf":[{"type":"number"},{"type":"integer"}]}`
	if ValidateArgs(schema, `1`) == nil {
		t.Fatal("ambiguous integer matched two schema alternatives")
	}
	if err := ValidateArgs(schema, `1.5`); err != nil {
		t.Fatal(err)
	}
}
