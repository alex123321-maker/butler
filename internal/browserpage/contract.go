// Package browserpage defines the credential-free, same-origin page execution
// contract shared by Action Service, Broker and the browser executor.
package browserpage

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"strings"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

const Open = "browser.open_pages"
const List = "browser.pages"
const Snapshot = "browser.page_snapshot"
const Click = "browser.page_click"
const Fill = "browser.page_fill"
const Type = "browser.page_type"
const Close = "browser.page_close"
const Revision = "1"

var ErrDenied = errors.New("exact browser page unavailable; refresh pages before acting")

type Ref struct {
	ID         string `json:"page_id"`
	DocumentID string `json:"document_id"`
	Version    int64  `json:"observation_version"`
}

type Args struct {
	SessionID string `json:"browser_session_id,omitempty"`
	URL       string `json:"url,omitempty"`
	Page      *Ref   `json:"page,omitempty"`
	Selector  string `json:"selector,omitempty"`
	Text      string `json:"text,omitempty"`
	Value     string `json:"value,omitempty"`
}

// Binding is durable Broker metadata, never model-selected URL/epoch/runtime
// state. All fields are pinned in the ordinary action hash and rechecked.
type Binding struct {
	Ref
	URL            string `json:"url"`
	Epoch          int64  `json:"epoch"`
	RuntimeVersion int64  `json:"runtime_version"`
}

func (b Binding) Valid(bound bool) bool {
	u, e := url.Parse(b.URL)
	if e != nil || u.User != nil || u.Hostname() == "" || u.Scheme != "http" && u.Scheme != "https" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || b.Epoch < 0 {
		return false
	}
	if bound {
		return b.Ref.Valid() && b.RuntimeVersion > 0
	}
	return b.Ref == (Ref{}) && b.RuntimeVersion == 0
}

// Report is private executor metadata. Broker translates runtime versions to
// durable observation versions before any model/transcript release.
type Report struct {
	ID             string `json:"page_id"`
	DocumentID     string `json:"document_id"`
	RuntimeVersion int64  `json:"runtime_version"`
	OpenerID       string `json:"opener_page_id,omitempty"`
	URL            string `json:"url"`
	State          string `json:"state"`
}

type View struct {
	Ref
	OpenerID string `json:"opener_page_id,omitempty"`
	URL      string `json:"url"`
	State    string `json:"state"`
}

func ID(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 16 && hex.EncodeToString(decoded) == value
}
func (r Ref) Valid() bool { return ID(r.ID) && ID(r.DocumentID) && r.Version > 0 }
func IsTool(tool string) bool {
	switch tool {
	case Open, List, Snapshot, Click, Fill, Type, Close:
		return true
	}
	return false
}
func Read(tool string) bool  { return tool == Open || tool == List || tool == Snapshot }
func Bound(tool string) bool { return IsTool(tool) && tool != Open && tool != List }
func Legacy(tool string) string {
	switch tool {
	case Open:
		return "browser.open"
	case Snapshot:
		return "browser.snapshot"
	case Click:
		return "browser.click"
	case Fill:
		return "browser.fill"
	case Type:
		return "browser.type"
	case Close:
		return "browser.page_close"
	}
	return tool
}

func Schema(tool string) string {
	properties := map[string]any{}
	required := []string{}
	str := map[string]string{"type": "string"}
	if tool == Open {
		properties["url"] = str
		required = append(required, "url")
	} else {
		properties["browser_session_id"] = str
		required = append(required, "browser_session_id")
	}
	if Bound(tool) {
		properties["page"] = map[string]any{"type": "object", "additionalProperties": false, "required": []string{"page_id", "document_id", "observation_version"}, "properties": map[string]any{"page_id": str, "document_id": str, "observation_version": map[string]string{"type": "integer"}}}
		required = append(required, "page")
	}
	if tool == Click || tool == Fill || tool == Type {
		properties["selector"] = str
		required = append(required, "selector")
	}
	if tool == Fill {
		properties["value"] = str
		required = append(required, "value")
	}
	if tool == Type {
		properties["text"] = str
		required = append(required, "text")
	}
	raw, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": properties})
	return string(raw)
}
func sameJSON(a, b string) bool {
	var x, y any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	p, _ := json.Marshal(x)
	q, _ := json.Marshal(y)
	return bytes.Equal(p, q)
}
func ValidContract(c *toolbrokerv1.ToolContract) bool {
	if c == nil || !IsTool(c.ToolName) || c.ContractRevision != Revision || c.SupportsCredentialRefs || !sameJSON(c.InputSchemaJson, Schema(c.ToolName)) {
		return false
	}
	e := c.GetEffectContract()
	if e == nil || e.IdempotencyHeader != "" || e.ScopeOrigin != "" || e.VerificationUrl != "" {
		return false
	}
	if Read(c.ToolName) {
		return e.RetrySafety == "read_only" && e.VerificationMethod == "read_result"
	}
	return c.RequiresApproval && e.RetrySafety == "unsafe" && e.VerificationMethod == "none"
}
func Parse(tool, raw string) (Args, error) {
	var a Args
	if !IsTool(tool) || len(raw) > 16384 {
		return a, ErrDenied
	}
	if !uniqueJSON([]byte(raw)) {
		return a, ErrDenied
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&a) != nil || d.Decode(new(any)) != io.EOF {
		return Args{}, ErrDenied
	}
	// Schema validation is also done by Broker. Reject missing/extra fields at
	// the independent executor boundary, including credential_ref-shaped values.
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &fields) != nil {
		return Args{}, ErrDenied
	}
	var schema struct {
		Required   []string       `json:"required"`
		Properties map[string]any `json:"properties"`
	}
	_ = json.Unmarshal([]byte(Schema(tool)), &schema)
	if len(fields) != len(schema.Required) {
		return Args{}, ErrDenied
	}
	for _, key := range schema.Required {
		if _, ok := fields[key]; !ok {
			return Args{}, ErrDenied
		}
	}
	if tool == Open {
		if a.URL == "" || len(a.URL) > 8192 {
			return Args{}, ErrDenied
		}
	} else if !ID(a.SessionID) {
		return Args{}, ErrDenied
	}
	if Bound(tool) && (a.Page == nil || !a.Page.Valid()) {
		return Args{}, ErrDenied
	}
	if Bound(tool) {
		var ref map[string]json.RawMessage
		if json.Unmarshal(fields["page"], &ref) != nil || len(ref) != 3 {
			return Args{}, ErrDenied
		}
		for _, key := range []string{"page_id", "document_id", "observation_version"} {
			if _, ok := ref[key]; !ok {
				return Args{}, ErrDenied
			}
		}
	}
	if (tool == Click || tool == Fill || tool == Type) && (a.Selector == "" || len(a.Selector) > 1024) {
		return Args{}, ErrDenied
	}
	if len(a.Value) > 4096 || len(a.Text) > 4096 {
		return Args{}, ErrDenied
	}
	return a, nil
}

func uniqueJSON(raw []byte) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(int) bool
	visit = func(depth int) bool {
		if depth > 16 {
			return false
		}
		token, e := d.Token()
		if e != nil {
			return false
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				s, ok := key.(string)
				if e != nil || !ok || seen[s] {
					return false
				}
				seen[s] = true
				if !visit(depth + 1) {
					return false
				}
			}
			end, e := d.Token()
			return e == nil && end == json.Delim('}')
		case json.Delim('['):
			for d.More() {
				if !visit(depth + 1) {
					return false
				}
			}
			end, e := d.Token()
			return e == nil && end == json.Delim(']')
		default:
			_, delimiter := token.(json.Delim)
			return !delimiter
		}
	}
	if !visit(0) {
		return false
	}
	_, e := d.Token()
	return e == io.EOF
}
