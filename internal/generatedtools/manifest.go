// Package generatedtools admits bounded declarative converter programs. An IR
// has no file, network, process, credential, import or arbitrary expression op.
package generatedtools

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"

	"github.com/butler/butler/internal/actionauth"
)

const (
	Format          = "butler.converter.v1"
	IRFormat        = "butler.converter.ir.v1"
	Class           = "delimited_json_v1"
	Checker         = "butler.delimited-json.checks.v1"
	MaxPackageBytes = 32 * 1024
	MaxInputBytes   = 64 * 1024
	MaxOutputBytes  = 256 * 1024
	MaxRows         = 512
	MaxColumns      = 16
	MaxFieldBytes   = 4096
	MaxMillis       = 2000
)

var (
	ErrPackage = errors.New("bounded converter package rejected")
	ErrInput   = errors.New("converter input rejected")
	ErrLimit   = errors.New("converter resource limit reached")
	ErrChecks  = errors.New("independent converter checks failed")
	ErrAccess  = errors.New("current converter authority required")
	identifier = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	fieldName  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)
	version    = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	digestName = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// Limits are independently enforced in the interpreter and independently
// bounded by the operator and the owner's converter permission at admission.
type Limits struct {
	InputBytes  int64 `json:"input_bytes"`
	OutputBytes int64 `json:"output_bytes"`
	Rows        int64 `json:"rows"`
	FieldBytes  int64 `json:"field_bytes"`
	Millis      int64 `json:"millis"`
}

type InputSchema struct {
	MediaType string   `json:"media_type"`
	Delimiter string   `json:"delimiter"`
	Header    bool     `json:"header"`
	Columns   []string `json:"columns"`
	TrimSpace bool     `json:"trim_space"`
}
type OutputField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	SourceIndex int    `json:"source_index"`
}
type OutputSchema struct {
	MediaType string        `json:"media_type"`
	Fields    []OutputField `json:"fields"`
}

// Need is a typed finite acceptance contract supplied by trusted task admission.
// The author cannot replace this with the schemas or tests in its package.
type Need struct {
	ID          string       `json:"need_id"`
	ConverterID string       `json:"converter_id"`
	Class       string       `json:"class"`
	Input       InputSchema  `json:"input_schema"`
	Output      OutputSchema `json:"output_schema"`
	Limits      Limits       `json:"limits"`
}
type Column struct {
	InputIndex int    `json:"input_index"`
	OutputName string `json:"output_name"`
	Type       string `json:"type"`
}

// Program is the entire executable instruction set. Unknown JSON keys are
// rejected, including scripts, regexes, URLs, paths, env or custom validators.
type Program struct {
	Format       string   `json:"format"`
	Operation    string   `json:"operation"`
	Delimiter    string   `json:"delimiter"`
	Header       bool     `json:"header"`
	TrimSpace    bool     `json:"trim_space"`
	InputColumns []string `json:"input_columns"`
	Columns      []Column `json:"columns"`
}
type Dependency struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}
type Test struct {
	Name     string          `json:"name"`
	Input    string          `json:"input"`
	Expected json.RawMessage `json:"expected"`
}
type Manifest struct {
	Format       string       `json:"format"`
	ID           string       `json:"id"`
	Version      string       `json:"version"`
	NeedID       string       `json:"need_id"`
	Class        string       `json:"class"`
	Code         Program      `json:"code"`
	Input        InputSchema  `json:"input_schema"`
	Output       OutputSchema `json:"output_schema"`
	Dependencies []Dependency `json:"dependencies"`
	Tests        []Test       `json:"tests"`
	Instructions string       `json:"instructions"`
	Limitations  []string     `json:"limitations"`
	Limits       Limits       `json:"limits"`
}
type Package struct {
	Manifest Manifest `json:"manifest"`
	Digest   string   `json:"digest"`
	Raw      []byte   `json:"-"`
}

func Hash(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func HashBytes(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func Decode(raw []byte, destination any) error {
	if len(raw) == 0 || len(raw) > MaxPackageBytes {
		return ErrPackage
	}
	if _, e := actionauth.ArgumentsHash(string(raw)); e != nil {
		return ErrPackage
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(destination) != nil || d.Decode(new(any)) != io.EOF {
		return ErrPackage
	}
	return nil
}
func Parse(raw []byte) (Package, error) {
	var m Manifest
	if Decode(raw, &m) != nil || validateManifest(m) != nil {
		return Package{}, ErrPackage
	}
	return Package{m, HashBytes(raw), append([]byte(nil), raw...)}, nil
}
func validType(t string) bool {
	return t == "string" || t == "integer" || t == "decimal" || t == "boolean"
}
func validDelimiter(v string) bool { return v == "," || v == "\t" || v == ";" || v == "|" }
func (l Limits) Validate() error {
	if l.InputBytes < 1 || l.InputBytes > MaxInputBytes || l.OutputBytes < 1 || l.OutputBytes > MaxOutputBytes || l.Rows < 1 || l.Rows > MaxRows || l.FieldBytes < 1 || l.FieldBytes > MaxFieldBytes || l.Millis < 1 || l.Millis > MaxMillis {
		return ErrPackage
	}
	return nil
}
func validateSchemas(i InputSchema, o OutputSchema) error {
	if !validDelimiter(i.Delimiter) || len(i.Columns) == 0 || len(i.Columns) > MaxColumns || o.MediaType != "application/json" || len(o.Fields) == 0 || len(o.Fields) > MaxColumns {
		return ErrPackage
	}
	if i.MediaType != "text/csv" && i.MediaType != "text/tab-separated-values" && i.MediaType != "text/x-delimited" {
		return ErrPackage
	}
	if i.MediaType == "text/csv" && i.Delimiter != "," || i.MediaType == "text/tab-separated-values" && i.Delimiter != "\t" {
		return ErrPackage
	}
	seen := map[string]bool{}
	for _, c := range i.Columns {
		if !fieldName.MatchString(c) || seen[c] {
			return ErrPackage
		}
		seen[c] = true
	}
	seen = map[string]bool{}
	types := map[int]string{}
	for _, c := range o.Fields {
		if !fieldName.MatchString(c.Name) || seen[c.Name] || !validType(c.Type) || c.SourceIndex < 0 || c.SourceIndex >= len(i.Columns) {
			return ErrPackage
		}
		seen[c.Name] = true
		if old, ok := types[c.SourceIndex]; ok && old != c.Type {
			return ErrPackage
		}
		types[c.SourceIndex] = c.Type
	}
	return nil
}
func (n Need) Validate() error {
	if !identifier.MatchString(n.ID) || !identifier.MatchString(n.ConverterID) || n.Class != Class || validateSchemas(n.Input, n.Output) != nil || n.Limits.Validate() != nil {
		return ErrPackage
	}
	return nil
}
func validateProgram(p Program) error {
	if p.Format != IRFormat || p.Operation != "delimited_to_objects" || !validDelimiter(p.Delimiter) || len(p.InputColumns) == 0 || len(p.InputColumns) > MaxColumns || len(p.Columns) == 0 || len(p.Columns) > MaxColumns {
		return ErrPackage
	}
	seen := map[string]bool{}
	for _, c := range p.InputColumns {
		if !fieldName.MatchString(c) || seen[c] {
			return ErrPackage
		}
		seen[c] = true
	}
	seen = map[string]bool{}
	for _, c := range p.Columns {
		if c.InputIndex < 0 || c.InputIndex >= len(p.InputColumns) || !fieldName.MatchString(c.OutputName) || seen[c.OutputName] || !validType(c.Type) {
			return ErrPackage
		}
		seen[c.OutputName] = true
	}
	return nil
}
func validateManifest(m Manifest) error {
	if m.Format != Format || !identifier.MatchString(m.ID) || !identifier.MatchString(m.NeedID) || !version.MatchString(m.Version) || m.Class != Class || validateProgram(m.Code) != nil || validateSchemas(m.Input, m.Output) != nil || m.Limits.Validate() != nil || len(m.Dependencies) != 1 || m.Dependencies[0].ID != "butler-converter-ir" || !version.MatchString(m.Dependencies[0].Version) || !digestName.MatchString(m.Dependencies[0].SHA256) || len(m.Tests) < 1 || len(m.Tests) > 4 || strings.TrimSpace(m.Instructions) == "" || len(m.Instructions) > 4096 || len(m.Limitations) < 1 || len(m.Limitations) > 8 {
		return ErrPackage
	}
	for _, t := range m.Tests {
		if !identifier.MatchString(t.Name) || len(t.Input) > 4096 || len(t.Expected) > 8192 || !json.Valid(t.Expected) {
			return ErrPackage
		}
	}
	for _, s := range m.Limitations {
		if strings.TrimSpace(s) == "" || len(s) > 1024 {
			return ErrPackage
		}
	}
	return nil
}
