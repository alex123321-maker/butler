// Package communicationcontrol defines credential-free publication contracts.
// Channel recipients and execution authority never come from model arguments.
package communicationcontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/butler/butler/internal/research"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

const Path = "/internal/communication"
const Target = "broker://communication-v1"
const Epoch = 1
const Revision = "2"

var ErrDenied = errors.New("communication unavailable, stale or outside policy")
var ErrConflict = errors.New("communication identity conflicts")

type Image struct {
	ArtifactRef string `json:"artifact_ref"`
	Alt         string `json:"alt"`
	Caption     string `json:"caption,omitempty"`
}
type Option struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	ImageRef    string `json:"image_ref,omitempty"`
}
type QuestionInput struct {
	Type        string   `json:"type"`
	Options     []Option `json:"options,omitempty"`
	AllowText   bool     `json:"allow_text_answer,omitempty"`
	Min         int      `json:"min_selections,omitempty"`
	Max         int      `json:"max_selections,omitempty"`
	MinLength   int      `json:"min_length,omitempty"`
	MaxLength   int      `json:"max_length,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
}
type Block struct {
	Type        string     `json:"type"`
	Text        string     `json:"text,omitempty"`
	Items       []string   `json:"items,omitempty"`
	Rows        [][]string `json:"rows,omitempty"`
	ArtifactRef string     `json:"artifact_ref,omitempty"`
	Alt         string     `json:"alt,omitempty"`
}
type Args struct {
	SchemaVersion int                 `json:"schema_version"`
	Purpose       string              `json:"purpose,omitempty"`
	Text          string              `json:"text,omitempty"`
	Images        []Image             `json:"images,omitempty"`
	Title         string              `json:"title,omitempty"`
	Summary       string              `json:"summary,omitempty"`
	Blocks        []Block             `json:"blocks,omitempty"`
	Attachments   []string            `json:"attachments,omitempty"`
	Question      string              `json:"question,omitempty"`
	Input         *QuestionInput      `json:"input,omitempty"`
	Wait          *bool               `json:"wait_for_response,omitempty"`
	ExpiresAt     *time.Time          `json:"expires_at,omitempty"`
	ReplyTo       string              `json:"reply_to_publication_ref,omitempty"`
	SourceRefs    []string            `json:"source_refs,omitempty"`
	Evidence      []research.Evidence `json:"evidence,omitempty"`
	Observation   *Observation        `json:"observation,omitempty"`
}

// Observation belongs only to a checked scheduled occurrence, never a timer.
type Observation struct {
	Status     string     `json:"status"`
	EventKey   string     `json:"event_key"`
	ExpectedAt *time.Time `json:"expected_at,omitempty"`
}
type Answer struct {
	OptionIDs []string `json:"option_ids,omitempty"`
	Text      string   `json:"text,omitempty"`
}
type Envelope struct {
	RunID        string `json:"run_id"`
	ToolCallID   string `json:"tool_call_id"`
	EffectID     string `json:"effect_id"`
	Tool         string `json:"tool"`
	ArgsJSON     string `json:"args_json"`
	ContractHash string `json:"contract_hash"`
	Lookup       bool   `json:"lookup,omitempty"`
}
type Delivery struct {
	Channel string `json:"channel"`
	State   string `json:"state"`
}
type Receipt struct {
	Status         string     `json:"status"`
	PublicationRef string     `json:"publication_ref"`
	InteractionRef string     `json:"interaction_ref,omitempty"`
	Delivery       []Delivery `json:"delivery"`
}
type Service interface {
	ExecuteDispatched(context.Context, Envelope) (Receipt, error)
}

func IsTool(n string) bool {
	return n == "communication.message" || n == "communication.photo" || n == "communication.report" || n == "communication.widget"
}
func Contract(n string) *toolbrokerv1.ToolContract {
	if !IsTool(n) {
		return nil
	}
	props := map[string]any{
		"schema_version":           map[string]any{"type": "integer", "enum": []int{1}},
		"reply_to_publication_ref": map[string]string{"type": "string", "description": "Optional publication_ref returned by a communication receipt. Never use interaction_ref or a Catalog source_ref here."},
		"source_refs":              map[string]any{"type": "array", "description": "Optional exact source_ref values returned by selected Catalog observations in this run, including message:response:interaction:... for an owner answer. Never invent message references from event or turn IDs, or put browser/tool-call IDs, URLs, interaction_ref or publication_ref here. For browser-only observations leave this array empty; include the page URL in the answer text when useful.", "items": map[string]string{"type": "string"}},
	}
	str := func() map[string]string { return map[string]string{"type": "string"} }
	arr := func(item any) map[string]any { return map[string]any{"type": "array", "items": item} }
	obj := func(p map[string]any, req ...string) map[string]any {
		return map[string]any{"type": "object", "properties": p, "required": req, "additionalProperties": false}
	}
	req := []string{"schema_version"}
	if n != "communication.widget" {
		props["purpose"] = map[string]any{"type": "string", "enum": []string{"progress", "result", "blocked"}}
		req = append(req, "purpose")
		props["evidence"] = arr(obj(map[string]any{"url": str(), "quote": str(), "role": map[string]any{"type": "string", "enum": []string{"primary", "secondary"}}}, "url", "quote", "role"))
		props["observation"] = obj(map[string]any{"status": map[string]any{"type": "string", "enum": []string{"confirmed", "postponed", "pending", "unknown"}}, "event_key": str(), "expected_at": str()}, "status", "event_key")
	}
	switch n {
	case "communication.message":
		props["text"] = str()
		req = append(req, "text")
	case "communication.photo":
		props["text"] = str()
		props["images"] = arr(obj(map[string]any{"artifact_ref": str(), "alt": str(), "caption": str()}, "artifact_ref", "alt"))
		req = append(req, "text", "images")
	case "communication.report":
		props["title"] = str()
		props["summary"] = str()
		props["blocks"] = arr(obj(map[string]any{"type": str(), "text": str(), "items": arr(str()), "rows": arr(arr(str())), "artifact_ref": str(), "alt": str()}, "type"))
		props["attachments"] = arr(str())
		req = append(req, "title", "summary", "blocks")
	case "communication.widget":
		props["question"] = str()
		props["wait_for_response"] = map[string]any{"type": "boolean", "enum": []bool{true}}
		props["expires_at"] = str()
		props["input"] = obj(map[string]any{"type": map[string]any{"type": "string", "enum": []string{"single_choice", "multi_choice", "text"}}, "options": arr(obj(map[string]any{"id": str(), "label": str(), "description": str(), "image_ref": str()}, "id", "label")), "allow_text_answer": map[string]string{"type": "boolean"}, "min_selections": map[string]string{"type": "integer"}, "max_selections": map[string]string{"type": "integer"}, "min_length": map[string]string{"type": "integer"}, "max_length": map[string]string{"type": "integer"}, "placeholder": str()}, "type")
		req = append(req, "question", "input")
	}
	raw, _ := json.Marshal(obj(props, req...))
	return &toolbrokerv1.ToolContract{ToolName: n, Description: "Publish a concise owner answer. For researched facts include direct source links and evidence entries: actual page URL, exact short quote, primary/secondary attribution. First-party status requires checking publisher and current version, not a page saying official. For a checked monitor include observation status and stable event_key; the kernel suppresses unchanged/pending observations. accepted means saved, not delivered or task completion. Widgets suspend execution until one validated owner response.", ToolClass: "communication_control", RuntimeTarget: Target, Status: "active", ContractRevision: Revision, RiskLevel: "medium", InputSchemaJson: string(raw), OutputSchemaJson: `{"type":"object","properties":{"status":{"type":"string"},"publication_ref":{"type":"string"},"interaction_ref":{"type":"string"},"delivery":{"type":"array","items":{"type":"object"}}},"required":["status","publication_ref","delivery"],"additionalProperties":false}`, EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "unsafe", VerificationMethod: "local_runtime_ack"}}
}
func Contracts() []*toolbrokerv1.ToolContract {
	var out []*toolbrokerv1.ToolContract
	for _, n := range []string{"communication.message", "communication.photo", "communication.report", "communication.widget"} {
		out = append(out, Contract(n))
	}
	return out
}

// StrictJSON checks duplicate keys as well as unknown members. Intent hashing
// and typed decoding must never disagree about which duplicate value is used.
func StrictJSON(raw []byte, out any) error {
	if len(raw) > 256*1024 || !utf8.Valid(raw) {
		return ErrDenied
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 16 {
			return ErrDenied
		}
		t, e := d.Token()
		if e != nil {
			return e
		}
		if t == json.Delim('{') {
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				if e != nil {
					return e
				}
				s, ok := k.(string)
				if !ok || seen[s] {
					return ErrDenied
				}
				seen[s] = true
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
			_, e = d.Token()
			return e
		}
		if t == json.Delim('[') {
			for d.More() {
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
			_, e = d.Token()
			return e
		}
		if _, ok := t.(json.Delim); ok {
			return ErrDenied
		}
		return nil
	}
	if walk(0) != nil {
		return ErrDenied
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrDenied
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrDenied
	}
	return nil
}
func bounded(s string, n int) bool {
	return utf8.ValidString(s) && strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= n
}
func DecodeArgs(tool, raw string) (Args, error) {
	var a Args
	if !IsTool(tool) || StrictJSON([]byte(raw), &a) != nil || a.SchemaVersion != 1 || len(a.SourceRefs) > 64 {
		return a, ErrDenied
	}
	// Decode against the exact method's top-level field set, not a permissive union.
	var fields map[string]json.RawMessage
	_ = json.Unmarshal([]byte(raw), &fields)
	var schema struct {
		Properties map[string]any `json:"properties"`
	}
	_ = json.Unmarshal([]byte(Contract(tool).InputSchemaJson), &schema)
	for k := range fields {
		if _, ok := schema.Properties[k]; !ok {
			return a, ErrDenied
		}
	}
	for _, r := range a.SourceRefs {
		if !bounded(r, 512) {
			return a, ErrDenied
		}
	}
	if len(a.Evidence) > 6 {
		return a, ErrDenied
	}
	for _, e := range a.Evidence {
		if !bounded(e.URL, 2048) || research.CanonicalURL(e.URL) == "" || !bounded(e.Quote, 600) || (e.Role != "primary" && e.Role != "secondary") {
			return a, ErrDenied
		}
	}
	if o := a.Observation; o != nil {
		if !bounded(o.EventKey, 256) || (o.Status != "confirmed" && o.Status != "postponed" && o.Status != "pending" && o.Status != "unknown") || tool != "communication.message" {
			return a, ErrDenied
		}
	}
	if tool != "communication.widget" && a.Purpose != "progress" && a.Purpose != "result" && a.Purpose != "blocked" {
		return a, ErrDenied
	}
	switch tool {
	case "communication.message":
		if !bounded(a.Text, 4000) {
			return a, ErrDenied
		}
	case "communication.photo":
		if !bounded(a.Text, 4000) || len(a.Images) < 1 || len(a.Images) > 10 {
			return a, ErrDenied
		}
		for _, i := range a.Images {
			if !strings.HasPrefix(i.ArtifactRef, "artifact:") || !bounded(i.Alt, 1000) {
				return a, ErrDenied
			}
		}
	case "communication.report":
		if !bounded(a.Title, 300) || !bounded(a.Summary, 4000) || len(a.Blocks) < 1 || len(a.Blocks) > 2000 {
			return a, ErrDenied
		}
		size := 0
		for _, b := range a.Blocks {
			switch b.Type {
			case "paragraph", "heading", "list", "quote", "code", "table", "divider", "image", "document":
			default:
				return a, ErrDenied
			}
			size += utf8.RuneCountInString(b.Text)
			if size > 200000 {
				return a, ErrDenied
			}
			if b.Type == "image" || b.Type == "document" {
				if !strings.HasPrefix(b.ArtifactRef, "artifact:") {
					return a, ErrDenied
				}
			}
			for _, row := range b.Rows {
				if len(row) > 20 {
					return a, ErrDenied
				}
			}
		}
	case "communication.widget":
		if value, exists := fields["wait_for_response"]; exists && string(bytes.TrimSpace(value)) != "true" {
			return a, ErrDenied
		}
		if !bounded(a.Question, 4000) || a.Input == nil || a.Wait != nil && !*a.Wait {
			return a, ErrDenied
		}
		q := a.Input
		if q.Type != "single_choice" && q.Type != "multi_choice" && q.Type != "text" {
			return a, ErrDenied
		}
		if q.Min < 0 || q.Max < 0 || q.MinLength < 0 || q.MaxLength < 0 || q.MaxLength > 4000 || q.MinLength > 4000 {
			return a, ErrDenied
		}
		if q.Type == "text" {
			if len(q.Options) != 0 || q.AllowText || q.Min != 0 || q.Max != 0 || q.MaxLength > 0 && q.MinLength > q.MaxLength {
				return a, ErrDenied
			}
		} else {
			if len(q.Options) < 1 || len(q.Options) > 20 || q.MinLength != 0 || q.MaxLength != 0 || q.Placeholder != "" {
				return a, ErrDenied
			}
			seen := map[string]bool{}
			for _, o := range q.Options {
				if !bounded(o.ID, 64) || !bounded(o.Label, 300) || seen[o.ID] || o.Description != "" && !bounded(o.Description, 2000) || o.ImageRef != "" && (!strings.HasPrefix(o.ImageRef, "artifact:") || !bounded(o.ImageRef, 512)) {
					return a, ErrDenied
				}
				seen[o.ID] = true
			}
			if q.Type == "single_choice" && (q.Min != 0 || q.Max != 0) {
				return a, ErrDenied
			}
			if q.Type == "multi_choice" && (q.AllowText || q.Min > len(q.Options) || q.Max > len(q.Options) || q.Max > 0 && q.Min > q.Max) {
				return a, ErrDenied
			}
		}
	}
	return a, nil
}
func ValidateAnswer(q QuestionInput, a Answer) error {
	if a.Text != "" {
		if len(a.OptionIDs) != 0 || q.Type != "text" && !q.AllowText {
			return ErrDenied
		}
		limit := q.MaxLength
		if limit == 0 {
			limit = 4000
		}
		if !bounded(a.Text, limit) || utf8.RuneCountInString(a.Text) < q.MinLength {
			return ErrDenied
		}
		return nil
	}
	if q.Type == "text" {
		return ErrDenied
	}
	if q.Type == "single_choice" && len(a.OptionIDs) != 1 {
		return ErrDenied
	}
	max := q.Max
	if max == 0 {
		max = len(q.Options)
	}
	if len(a.OptionIDs) < q.Min || len(a.OptionIDs) > max {
		return ErrDenied
	}
	seen := map[string]bool{}
	for _, id := range a.OptionIDs {
		valid := false
		for _, o := range q.Options {
			valid = valid || o.ID == id
		}
		if !valid || seen[id] {
			return ErrDenied
		}
		seen[id] = true
	}
	return nil
}
