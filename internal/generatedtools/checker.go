package generatedtools

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

// Profile is fixed operator deployment configuration. The package cannot
// supply a checker binary, another dependency or alternate acceptance suite.
type Profile struct {
	Dependency Dependency `json:"dependency"`
	Checker    string     `json:"checker"`
}

func (p Profile) Validate() error {
	if p.Dependency.ID != "butler-converter-ir" || !version.MatchString(p.Dependency.Version) || !digestName.MatchString(p.Dependency.SHA256) || p.Checker != Checker {
		return ErrPackage
	}
	return nil
}

type Report struct {
	PackageHash string   `json:"package_hash"`
	NeedHash    string   `json:"need_hash"`
	ProfileHash string   `json:"profile_hash"`
	Checker     string   `json:"checker"`
	Checks      []string `json:"checks"`
	Passed      bool     `json:"passed"`
}

func csvInput(n Need, records [][]string) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Comma = []rune(n.Input.Delimiter)[0]
	if n.Input.Header {
		_ = w.Write(n.Input.Columns)
	}
	for _, row := range records {
		_ = w.Write(row)
	}
	w.Flush()
	return b.Bytes()
}
func jsonEqual(a, b []byte) bool {
	decode := func(raw []byte) (any, error) {
		var v any
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		e := d.Decode(&v)
		if e == nil && d.Decode(new(any)) != io.EOF {
			return nil, ErrChecks
		}
		return v, e
	}
	av, ae := decode(a)
	bv, be := decode(b)
	return ae == nil && be == nil && reflect.DeepEqual(av, bv)
}

// fixture constructs values and expected output independently from Code. Each
// input position is varied separately, including booleans, so a wrong mapping
// cannot pass merely because two synthetic columns happen to have equal data.
func fixture(n Need, focus int, variant int) ([]byte, []byte) {
	row := make([]string, len(n.Input.Columns))
	for i := range row {
		row[i] = fmt.Sprintf("text_%d", i)
	}
	values := make(map[int]any)
	for _, field := range n.Output.Fields {
		i := field.SourceIndex
		switch field.Type {
		case "string":
			row[i] = fmt.Sprintf("text_%d", i)
			if i == focus && variant == 1 {
				row[i] = "a" + n.Input.Delimiter + "\"\nΩ"
			}
			if i == focus && variant == 2 {
				row[i] = "  spaced  "
			}
			v := row[i]
			if n.Input.TrimSpace {
				v = strings.TrimSpace(v)
			}
			values[i] = v
		case "integer":
			v := int64(i + 3)
			if i == focus {
				v = -17 - int64(variant)
			}
			row[i] = fmt.Sprint(v)
			values[i] = v
		case "decimal":
			v := fmt.Sprintf("%d.125", i+3)
			if i == focus {
				v = "-0.25"
			}
			row[i] = v
			values[i] = json.Number(v)
		case "boolean":
			v := i == focus
			row[i] = fmt.Sprint(v)
			values[i] = v
		}
	}
	out := make(map[string]any, len(n.Output.Fields))
	for _, f := range n.Output.Fields {
		out[f.Name] = values[f.SourceIndex]
	}
	want, _ := json.Marshal([]map[string]any{out})
	return csvInput(n, [][]string{row}), want
}

// Check receives the independently persisted Need, never a caller verdict.
// Its report contains only hashes and fixed check names; fixture inputs and
// raw package contents do not enter durable action or approval metadata.
func Check(ctx context.Context, p Package, n Need, profile Profile) (Report, error) {
	r := Report{PackageHash: p.Digest, NeedHash: Hash(n), ProfileHash: Hash(profile), Checker: Checker, Checks: []string{}}
	fresh, e := Parse(p.Raw)
	if e != nil || fresh.Digest != p.Digest || Hash(fresh.Manifest) != Hash(p.Manifest) || n.Validate() != nil || profile.Validate() != nil {
		return r, ErrPackage
	}
	m := p.Manifest
	if m.ID != n.ConverterID || m.NeedID != n.ID || m.Class != n.Class || Hash(m.Input) != Hash(n.Input) || Hash(m.Output) != Hash(n.Output) || m.Limits != n.Limits || m.Dependencies[0] != profile.Dependency {
		return r, ErrChecks
	}
	// The bounded schema disallows interpreting one input column as two types.
	types := map[int]string{}
	for _, f := range n.Output.Fields {
		if old, ok := types[f.SourceIndex]; ok && old != f.Type {
			return r, ErrChecks
		}
		types[f.SourceIndex] = f.Type
	}
	for i := range n.Input.Columns {
		for v := 0; v < 3; v++ {
			input, want := fixture(n, i, v)
			got, e := Convert(ctx, m.Code, n.Limits, input)
			if e != nil || !jsonEqual(got, want) {
				return r, ErrChecks
			}
		}
	}
	r.Checks = append(r.Checks, "independent_typed_mapping_quotes_unicode_whitespace")
	input, _ := fixture(n, 0, 0)
	// Inputs from the package's tests cannot replace these negative fixtures.
	if _, e = Convert(ctx, m.Code, n.Limits, []byte{'x', 0, 'y'}); !errors.Is(e, ErrInput) {
		return r, ErrChecks
	}
	if _, e = Convert(ctx, m.Code, n.Limits, []byte{0xff}); !errors.Is(e, ErrInput) {
		return r, ErrChecks
	}
	if _, e = Convert(ctx, m.Code, n.Limits, nil); !errors.Is(e, ErrInput) {
		return r, ErrChecks
	}
	if _, e = Convert(ctx, m.Code, n.Limits, []byte("\"unterminated")); !errors.Is(e, ErrInput) {
		return r, ErrChecks
	}
	if len(n.Input.Columns) > 1 {
		if _, e = Convert(ctx, m.Code, n.Limits, csvInput(n, [][]string{{"wrong_width"}})); !errors.Is(e, ErrInput) {
			return r, ErrChecks
		}
	}
	if n.Input.Header {
		bad := bytes.Replace(input, []byte(n.Input.Columns[0]), []byte("wrong_header"), 1)
		if _, e = Convert(ctx, m.Code, n.Limits, bad); !errors.Is(e, ErrInput) {
			return r, ErrChecks
		}
	}
	for _, f := range n.Output.Fields {
		if f.Type == "string" {
			continue
		}
		row := make([]string, len(n.Input.Columns))
		for i := range row {
			row[i] = "0"
		}
		for _, of := range n.Output.Fields {
			if of.Type == "boolean" {
				row[of.SourceIndex] = "true"
			}
		}
		row[f.SourceIndex] = "not_a_value"
		if _, e = Convert(ctx, m.Code, n.Limits, csvInput(n, [][]string{row})); !errors.Is(e, ErrInput) {
			return r, ErrChecks
		}
	}
	r.Checks = append(r.Checks, "reject_wrong_shape_header_numeric_boolean_utf8_nul")
	// Every resource cap is tested at the independently specified boundary.
	limited := n.Limits
	limited.InputBytes = int64(len(input) - 1)
	if limited.InputBytes > 0 {
		if _, e = Convert(ctx, m.Code, limited, input); !errors.Is(e, ErrLimit) {
			return r, ErrChecks
		}
	}
	limited = n.Limits
	limited.OutputBytes = 1
	if _, e = Convert(ctx, m.Code, limited, input); !errors.Is(e, ErrLimit) {
		return r, ErrChecks
	}
	limited = n.Limits
	limited.FieldBytes = 1
	if _, e = Convert(ctx, m.Code, limited, input); !errors.Is(e, ErrLimit) {
		return r, ErrChecks
	}
	row := make([]string, len(n.Input.Columns))
	for i := range row {
		row[i] = "1"
	}
	for _, f := range n.Output.Fields {
		if f.Type == "boolean" {
			row[f.SourceIndex] = "true"
		}
	}
	limited = n.Limits
	limited.Rows = 1
	if _, e = Convert(ctx, m.Code, limited, csvInput(n, [][]string{row, row})); !errors.Is(e, ErrLimit) {
		return r, ErrChecks
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = Convert(cancelled, m.Code, n.Limits, input); !errors.Is(e, context.Canceled) {
		return r, ErrChecks
	}
	r.Checks = append(r.Checks, "input_output_row_field_limits_and_cancellation")
	for _, test := range m.Tests {
		got, e := Convert(ctx, m.Code, n.Limits, []byte(test.Input))
		if e != nil || !jsonEqual(got, test.Expected) {
			return r, ErrChecks
		}
	}
	r.Checks = append(r.Checks, "author_tests_additive_only", "no_file_network_process_or_secret_operations")
	r.Passed = true
	return r, nil
}
