package generatedtools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func example(t *testing.T) (Need, Package, Profile) {
	t.Helper()
	n := Need{ID: "convert-inventory", ConverterID: "inventory", Class: Class,
		Input:  InputSchema{MediaType: "text/csv", Delimiter: ",", Header: true, Columns: []string{"name", "count", "price", "enabled"}, TrimSpace: true},
		Output: OutputSchema{MediaType: "application/json", Fields: []OutputField{{"label", "string", 0}, {"quantity", "integer", 1}, {"amount", "decimal", 2}, {"active", "boolean", 3}}},
		Limits: Limits{InputBytes: 8192, OutputBytes: 32768, Rows: 128, FieldBytes: 1024, Millis: 1000}}
	profile := Profile{Dependency: Dependency{"butler-converter-ir", "1.0.0", strings.Repeat("a", 64)}, Checker: Checker}
	m := Manifest{Format: Format, ID: n.ConverterID, Version: "1.0.0", NeedID: n.ID, Class: Class, Input: n.Input, Output: n.Output, Dependencies: []Dependency{profile.Dependency}, Limits: n.Limits,
		Code:         Program{Format: IRFormat, Operation: "delimited_to_objects", Delimiter: ",", Header: true, TrimSpace: true, InputColumns: n.Input.Columns, Columns: []Column{{0, "label", "string"}, {1, "quantity", "integer"}, {2, "amount", "decimal"}, {3, "active", "boolean"}}},
		Tests:        []Test{{"author-example", "name,count,price,enabled\nwidget,2,1.25,true\n", json.RawMessage(`[{"label":"widget","quantity":2,"amount":1.25,"active":true}]`)}},
		Instructions: "Pass a current non-secret Catalog input source; returns a derived JSON source.", Limitations: []string{"No files, network, processes, credentials or expressions."}}
	raw, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	p, e := Parse(raw)
	if e != nil {
		t.Fatal(e)
	}
	return n, p, profile
}
func mutate(t *testing.T, p Package, f func(*Manifest)) Package {
	t.Helper()
	var clone Manifest
	if Decode(p.Raw, &clone) != nil {
		t.Fatal("invalid initial package")
	}
	p.Manifest = clone
	f(&p.Manifest)
	raw, e := json.Marshal(p.Manifest)
	if e != nil {
		t.Fatal(e)
	}
	next, e := Parse(raw)
	if e != nil {
		t.Fatal(e)
	}
	return next
}
func TestIndependentCheckerRejectsAuthorMappingAndAcceptanceChanges(t *testing.T) {
	n, p, profile := example(t)
	report, e := Check(context.Background(), p, n, profile)
	if e != nil || !report.Passed || report.NeedHash != Hash(n) || report.ProfileHash != Hash(profile) || len(report.Checks) != 5 {
		t.Fatalf("valid: %#v %v", report, e)
	}
	for _, tc := range []struct {
		name   string
		change func(*Manifest)
	}{
		{"mapping", func(m *Manifest) { m.Code.Columns[0].InputIndex = 1 }},
		{"header", func(m *Manifest) { m.Code.Header = false }},
		{"dialect", func(m *Manifest) { m.Code.Delimiter = ";" }},
		{"type", func(m *Manifest) { m.Code.Columns[1].Type = "string" }},
		{"schema", func(m *Manifest) { m.Output.Fields[0].Name = "renamed" }},
		{"limits", func(m *Manifest) { m.Limits.Rows++ }},
		{"runtime", func(m *Manifest) { m.Dependencies[0].SHA256 = strings.Repeat("b", 64) }},
		{"author-test", func(m *Manifest) { m.Tests[0].Expected = json.RawMessage(`[]`) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := mutate(t, p, tc.change)
			r, e := Check(context.Background(), changed, n, profile)
			if e == nil || r.Passed {
				t.Fatal("changed acceptance admitted")
			}
		})
	}
}
func TestPackageRejectsExecutableAuthorityAndDuplicateMembers(t *testing.T) {
	_, p, _ := example(t)
	for _, key := range []string{"script", "url", "path", "process", "credential_ref", "env", "checker", "permissions", "imports"} {
		raw := append([]byte(`{"`+key+`":"arbitrary",`), p.Raw[1:]...)
		if _, e := Parse(raw); !errors.Is(e, ErrPackage) {
			t.Fatalf("key %s admitted", key)
		}
	}
	raw := strings.Replace(string(p.Raw), `"format":`, `"format":"other","format":`, 1)
	if _, e := Parse([]byte(raw)); !errors.Is(e, ErrPackage) {
		t.Fatal("duplicate admitted")
	}
	if _, e := Parse(append(p.Raw, []byte(` {}`)...)); !errors.Is(e, ErrPackage) {
		t.Fatal("trailing JSON admitted")
	}
	oversize := append(append([]byte{}, p.Raw...), []byte(strings.Repeat(" ", MaxPackageBytes))...)
	if _, e := Parse(oversize); !errors.Is(e, ErrPackage) {
		t.Fatal("oversized package admitted")
	}
}
func TestConverterExactValuesBoundedErrorsAndNoPartialOutput(t *testing.T) {
	n, p, _ := example(t)
	got, e := Convert(context.Background(), p.Manifest.Code, n.Limits, []byte("name,count,price,enabled\n\"a,b\",9223372036854775807,-0.250,true\n"))
	if e != nil || !jsonEqual(got, []byte(`[{"label":"a,b","quantity":9223372036854775807,"amount":-0.250,"active":true}]`)) {
		t.Fatalf("%s %v", got, e)
	}
	for _, input := range []string{
		"name,count,price,enabled\nknown,1,2.5,true\nprivate-marker,not-integer,2.5,true\n",
		"name,count,price,enabled\nprivate-marker,9223372036854775808,2.5,true\n",
		"name,count,price,enabled\nprivate-marker,01,2.5,true\n",
		"name,count,price,enabled\nprivate-marker,1,2e5,true\n",
		"name,count,price,enabled\nprivate-marker,1,2.5,TRUE\n",
	} {
		got, e := Convert(context.Background(), p.Manifest.Code, n.Limits, []byte(input))
		if !errors.Is(e, ErrInput) || len(got) != 0 || strings.Contains(e.Error(), "private-marker") {
			t.Fatalf("input error leaked partial: %s %v", got, e)
		}
	}
}
func TestDeveloperFreshTemporaryWorkcopyRemoved(t *testing.T) {
	n, p, profile := example(t)
	base := t.TempDir()
	report, e := (Developer{TempDir: base, Profile: profile}).Test(context.Background(), p.Raw, n)
	if e != nil || !report.Passed {
		t.Fatalf("%#v %v", report, e)
	}
	entries, e := os.ReadDir(base)
	if e != nil || len(entries) != 0 {
		t.Fatalf("workcopy retained: %v %v", entries, e)
	}
	changed := mutate(t, p, func(m *Manifest) { m.Code.Columns[0].OutputName = "wrong" })
	if r, e := (Developer{TempDir: base, Profile: profile}).Test(context.Background(), changed.Raw, n); e == nil || r.Passed {
		t.Fatal("bad developer result admitted")
	}
	entries, _ = os.ReadDir(base)
	if len(entries) != 0 {
		t.Fatal("failed workcopy retained")
	}
}
