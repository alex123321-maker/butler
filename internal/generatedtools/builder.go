package generatedtools

import "encoding/json"

// Build creates code for the single supported finite IR from a trusted Need.
// It neither tests nor activates it. Registration is a separate independently
// checked catalog transition under the current owner's exact shape permission.
func Build(n Need, v string, profile Profile) (Package, error) {
	if n.Validate() != nil || profile.Validate() != nil || !version.MatchString(v) {
		return Package{}, ErrPackage
	}
	p := Program{Format: IRFormat, Operation: "delimited_to_objects", Delimiter: n.Input.Delimiter, Header: n.Input.Header, TrimSpace: n.Input.TrimSpace, InputColumns: append([]string(nil), n.Input.Columns...)}
	for _, f := range n.Output.Fields {
		p.Columns = append(p.Columns, Column{InputIndex: f.SourceIndex, OutputName: f.Name, Type: f.Type})
	}
	input, expected := fixture(n, 0, 0)
	m := Manifest{Format: Format, ID: n.ConverterID, Version: v, NeedID: n.ID, Class: Class, Code: p, Input: n.Input, Output: n.Output, Dependencies: []Dependency{profile.Dependency}, Tests: []Test{{Name: "synthetic-example", Input: string(input), Expected: expected}}, Instructions: "Supply a current Catalog DATA input matching input_schema. The Broker publishes a derived application/json source after the fixed checks and exact permission pass.", Limitations: []string{"No file, network, process, credentials, arbitrary expressions or custom validators.", "Strict column count/header, JSON integer/decimal/boolean formats and finite byte/row/time bounds; blank CSV lines are ignored."}, Limits: n.Limits}
	raw, e := json.Marshal(m)
	if e != nil {
		return Package{}, ErrPackage
	}
	return Parse(raw)
}
