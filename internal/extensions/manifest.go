// Package extensions owns reviewed, declarative capability manifests and their
// durable lifecycle. A package is data, not executable code or authorization.
package extensions

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

const (
	ManifestFormat   = "butler.extension.v1"
	RuntimeABI       = "butler.http-json.v1"
	MaxManifestBytes = 512 * 1024
)

var (
	ErrManifest = errors.New("extension manifest rejected")
	identifier  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	versionName = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[a-z0-9.-]+)?$`)
	digestName  = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// Manifest declares requested capabilities only. Operator review, installation,
// enablement and current task authority are separate durable records.
type Manifest struct {
	Format        string        `json:"format"`
	ID            string        `json:"id"`
	Version       string        `json:"version"`
	Source        Source        `json:"source"`
	Description   string        `json:"description"`
	Runtime       Runtime       `json:"runtime"`
	Compatibility Compatibility `json:"compatibility"`
	Configuration []Setting     `json:"configuration"`
	Resources     []Resource    `json:"resources"`
	Credentials   []Credential  `json:"credentials"`
	Operations    []Operation   `json:"operations"`
	Readiness     Readiness     `json:"readiness"`
	Dependencies  []Dependency  `json:"dependencies"`
	Limits        Limits        `json:"limits"`
	Rollback      Rollback      `json:"rollback"`
}

type Source struct {
	URI      string `json:"uri"`
	Revision string `json:"revision"`
}
type Runtime struct {
	ABI     string `json:"abi"`
	Profile string `json:"profile"`
}
type Compatibility struct {
	CoreContract string `json:"core_contract"`
	ToolContract string `json:"tool_contract"`
}
type Setting struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required"`
}
type Resource struct {
	OriginSetting string `json:"origin_setting"`
	PathPrefix    string `json:"path_prefix"`
}
type Credential struct {
	Input    string `json:"input"`
	Field    string `json:"field"`
	Required bool   `json:"required"`
}
type Operation struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	Method       string          `json:"method"`
	Effect       string          `json:"effect"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
	Tags         []string        `json:"tags"`
	Receipt      *Receipt        `json:"receipt,omitempty"`
}

// Receipt declares a reviewed recipient protocol, not a runtime success claim.
// The Broker owns keys/fingerprints and verifies matching recipient evidence.
type Receipt struct {
	Protocol          string `json:"protocol"`
	IdempotencyHeader string `json:"idempotency_header"`
	VerificationPath  string `json:"verification_path"`
}
type Readiness struct {
	Method string `json:"method"`
	Path   string `json:"path"`
}
type Dependency struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Source  string `json:"source"`
	SHA256  string `json:"sha256"`
}
type Limits struct {
	TimeoutMillis int64 `json:"timeout_ms"`
	EgressBytes   int64 `json:"egress_bytes"`
	OutputBytes   int64 `json:"output_bytes"`
}
type Rollback struct {
	Supported          bool   `json:"supported"`
	StateCompatibility string `json:"state_compatibility"`
}
type Package struct {
	Manifest Manifest
	Digest   string
	Raw      []byte
}

// Parse never fetches sources, launches a process or accepts an archive path.
// Exact raw bytes are hashed and retained; JSONB reformatting must not change
// an installed package identity or make a changed version look reviewed.
func Parse(reader io.Reader) (Package, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, MaxManifestBytes+1))
	if err != nil || len(raw) > MaxManifestBytes || strictJSON(raw) != nil {
		return Package{}, ErrManifest
	}
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil || validateManifest(manifest) != nil {
		return Package{}, ErrManifest
	}
	sum := sha256.Sum256(raw)
	return Package{Manifest: manifest, Digest: hex.EncodeToString(sum[:]), Raw: append([]byte(nil), raw...)}, nil
}

func validateManifest(m Manifest) error {
	if m.Format != ManifestFormat || !identifier.MatchString(m.ID) || !versionName.MatchString(m.Version) || !sourceURI(m.Source.URI) || strings.TrimSpace(m.Source.Revision) == "" || len(m.Source.Revision) > 160 || len(m.Description) > 4000 {
		return ErrManifest
	}
	if m.Runtime.ABI != RuntimeABI || !identifier.MatchString(m.Runtime.Profile) || m.Compatibility.CoreContract != "1" || m.Compatibility.ToolContract != "1" {
		return ErrManifest
	}
	if len(m.Operations) == 0 || len(m.Operations) > 32 || len(m.Configuration) > 16 || len(m.Resources) == 0 || len(m.Resources) > 8 || len(m.Credentials) > 8 || len(m.Dependencies) == 0 || len(m.Dependencies) > 32 {
		return ErrManifest
	}
	if m.Limits.TimeoutMillis < 1 || m.Limits.TimeoutMillis > 30000 || m.Limits.EgressBytes < 1 || m.Limits.EgressBytes > 4*1024*1024 || m.Limits.OutputBytes < 1 || m.Limits.OutputBytes > 256*1024 || !m.Rollback.Supported || m.Rollback.StateCompatibility != "stateless" {
		return ErrManifest
	}
	settings := map[string]Setting{}
	for _, setting := range m.Configuration {
		if !identifier.MatchString(setting.Name) || (setting.Type != "public_origin" && setting.Type != "text") {
			return ErrManifest
		}
		if _, exists := settings[setting.Name]; exists {
			return ErrManifest
		}
		settings[setting.Name] = setting
	}
	for _, resource := range m.Resources {
		setting, ok := settings[resource.OriginSetting]
		if !ok || setting.Type != "public_origin" || !setting.Required || !canonicalPath(resource.PathPrefix) {
			return ErrManifest
		}
	}
	if m.Readiness.Method != "GET" || !canonicalPath(m.Readiness.Path) {
		return ErrManifest
	}
	probeWithin := false
	for _, resource := range m.Resources {
		if pathWithin(resource.PathPrefix, m.Readiness.Path) {
			probeWithin = true
		}
	}
	if !probeWithin {
		return ErrManifest
	}
	names := map[string]bool{}
	for _, operation := range m.Operations {
		prefix := "extension." + m.ID + "."
		if !strings.HasPrefix(operation.Name, prefix) || !identifier.MatchString(strings.TrimPrefix(operation.Name, prefix)) || names[operation.Name] || len(operation.Description) > 4000 || len(operation.Tags) > 16 {
			return ErrManifest
		}
		names[operation.Name] = true
		if operation.Method == "GET" || operation.Method == "HEAD" {
			if operation.Effect != "read" {
				return ErrManifest
			}
		} else if operation.Method != "POST" && operation.Method != "PUT" && operation.Method != "PATCH" && operation.Method != "DELETE" || operation.Effect != "commit" {
			return ErrManifest
		}
		if validateSchema(operation.InputSchema, true) != nil || validateSchema(operation.OutputSchema, false) != nil {
			return ErrManifest
		}
		if receipt := operation.Receipt; receipt != nil {
			if operation.Effect != "commit" || receipt.Protocol != "external_receipt_v1" || (receipt.IdempotencyHeader != "Idempotency-Key" && receipt.IdempotencyHeader != "X-Idempotency-Key") || strings.Count(receipt.VerificationPath, "{idempotency_key}") != 1 || !canonicalPath(strings.ReplaceAll(receipt.VerificationPath, "{idempotency_key}", "key")) {
				return ErrManifest
			}
			origin := m.Resources[0].OriginSetting
			within := false
			for _, resource := range m.Resources {
				if resource.OriginSetting != origin {
					return ErrManifest
				}
				within = within || pathWithin(resource.PathPrefix, strings.ReplaceAll(receipt.VerificationPath, "{idempotency_key}", "key"))
			}
			if !within {
				return ErrManifest
			}
		}
		var input struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if json.Unmarshal(operation.InputSchema, &input) != nil {
			return ErrManifest
		}
		if (operation.Method == "GET" || operation.Method == "HEAD") && input.Properties["body"] != nil {
			return ErrManifest
		}
		var output struct {
			Type       string `json:"type"`
			Properties map[string]struct {
				Type string `json:"type"`
			} `json:"properties"`
			Required             []string `json:"required"`
			AdditionalProperties *bool    `json:"additionalProperties"`
		}
		if json.Unmarshal(operation.OutputSchema, &output) != nil || output.Type != "object" || len(output.Properties) != 2 || output.Properties["status_code"].Type != "integer" || output.Properties["body"].Type != "string" || len(output.Required) != 2 || !has(output.Required, "status_code") || !has(output.Required, "body") || output.AdditionalProperties == nil || *output.AdditionalProperties {
			return ErrManifest
		}
		for _, tag := range operation.Tags {
			if len(tag) > 80 || strings.TrimSpace(tag) == "" {
				return ErrManifest
			}
		}
	}
	credentialInputs := map[string]bool{}
	for _, credential := range m.Credentials {
		// The initial runtime supports one fixed Authorization header slot. A
		// manifest cannot turn arbitrary argument/header names into secret sinks.
		if credential.Input != "authorization" || !identifier.MatchString(credential.Field) || credentialInputs[credential.Input] {
			return ErrManifest
		}
		credentialInputs[credential.Input] = true
	}
	dependencies := map[string]bool{}
	for _, dependency := range m.Dependencies {
		if !identifier.MatchString(dependency.ID) || strings.TrimSpace(dependency.Version) == "" || len(dependency.Version) > 160 || !sourceURI(dependency.Source) || !digestName.MatchString(dependency.SHA256) || dependencies[dependency.ID] {
			return ErrManifest
		}
		dependencies[dependency.ID] = true
	}
	return nil
}

func sourceURI(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Host != "" && u.Scheme == "https"
}
func canonicalPath(path string) bool {
	if !strings.HasPrefix(path, "/") || len(path) > 1024 || strings.ContainsAny(path, "%?#\\\x00") || strings.Contains(path, "//") {
		return false
	}
	for _, part := range strings.Split(path, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return path == "/" || !strings.HasSuffix(path, "/")
}
func pathWithin(prefix, path string) bool {
	return prefix == "/" || path == prefix || strings.HasPrefix(path, prefix+"/")
}

// The admitted schema dialect is deliberately finite and matches the Broker
// validator. Unknown JSON Schema keywords must not be silently ignored.
func validateSchema(raw []byte, input bool) error {
	if strictJSON(raw) != nil {
		return ErrManifest
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil {
		return ErrManifest
	}
	if input {
		var typ string
		var additional bool
		if json.Unmarshal(root["type"], &typ) != nil || typ != "object" || json.Unmarshal(root["additionalProperties"], &additional) != nil || additional {
			return ErrManifest
		}
		var properties map[string]json.RawMessage
		var required []string
		if json.Unmarshal(root["properties"], &properties) != nil || properties["url"] == nil || json.Unmarshal(root["required"], &required) != nil {
			return ErrManifest
		}
		var urlSchema struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(properties["url"], &urlSchema) != nil || urlSchema.Type != "string" {
			return ErrManifest
		}
		for name := range properties {
			if name != "url" && name != "body" && name != "authorization" {
				return ErrManifest
			}
		}
		found := false
		for _, name := range required {
			if name == "url" {
				found = true
			}
		}
		if !found {
			return ErrManifest
		}
	}
	var visit func(map[string]json.RawMessage, int) error
	visit = func(node map[string]json.RawMessage, depth int) error {
		if node == nil || depth > 16 {
			return ErrManifest
		}
		for key := range node {
			switch key {
			case "type", "properties", "required", "enum", "items", "oneOf", "additionalProperties", "description":
			default:
				return ErrManifest
			}
		}
		var typ string
		if value, ok := node["type"]; ok {
			if json.Unmarshal(value, &typ) != nil {
				return ErrManifest
			}
			switch typ {
			case "object", "string", "boolean", "number", "integer", "array":
			default:
				return ErrManifest
			}
		}
		var properties map[string]json.RawMessage
		if value, ok := node["properties"]; ok {
			if typ != "object" || json.Unmarshal(value, &properties) != nil || len(properties) > 64 {
				return ErrManifest
			}
			for name, child := range properties {
				if len(name) > 160 || strings.TrimSpace(name) == "" {
					return ErrManifest
				}
				var object map[string]json.RawMessage
				if json.Unmarshal(child, &object) != nil || visit(object, depth+1) != nil {
					return ErrManifest
				}
			}
		}
		if value, ok := node["required"]; ok {
			var names []string
			if typ != "object" || json.Unmarshal(value, &names) != nil || len(names) > 64 {
				return ErrManifest
			}
			sort.Strings(names)
			for i, name := range names {
				if properties[name] == nil || i > 0 && name == names[i-1] {
					return ErrManifest
				}
			}
		}
		if value, ok := node["additionalProperties"]; ok {
			var allowed bool
			if typ != "object" || json.Unmarshal(value, &allowed) != nil {
				return ErrManifest
			}
		}
		if value, ok := node["enum"]; ok {
			var values []any
			if json.Unmarshal(value, &values) != nil || len(values) == 0 || len(values) > 64 {
				return ErrManifest
			}
		}
		if value, ok := node["items"]; ok {
			var child map[string]json.RawMessage
			if typ != "array" || json.Unmarshal(value, &child) != nil || visit(child, depth+1) != nil {
				return ErrManifest
			}
		}
		if value, ok := node["oneOf"]; ok {
			var children []map[string]json.RawMessage
			if json.Unmarshal(value, &children) != nil || len(children) < 2 || len(children) > 8 {
				return ErrManifest
			}
			for _, child := range children {
				if visit(child, depth+1) != nil {
					return ErrManifest
				}
			}
		}
		if value, ok := node["description"]; ok {
			var text string
			if json.Unmarshal(value, &text) != nil || len(text) > 4000 {
				return ErrManifest
			}
		}
		if typ == "" && node["oneOf"] == nil {
			return ErrManifest
		}
		return nil
	}
	return visit(root, 0)
}

func strictJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 32 {
			return ErrManifest
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			keys := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				name, ok := key.(string)
				if err != nil || !ok || keys[name] {
					return ErrManifest
				}
				keys[name] = true
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		case '[':
			for decoder.More() {
				if err := walk(depth + 1); err != nil {
					return err
				}
			}
		default:
			return ErrManifest
		}
		_, err = decoder.Token()
		return err
	}
	if err := walk(0); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON", ErrManifest)
	}
	return nil
}
