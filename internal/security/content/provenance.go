// Package content keeps instruction authority separate from observed data.
package content

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/butler/butler/internal/transport"
)

type Kind string
type Sensitivity string

const (
	KernelInstruction   Kind        = "kernel_instruction"
	OperatorInstruction Kind        = "operator_instruction"
	OwnerPolicy         Kind        = "owner_policy"
	OwnerInput          Kind        = "owner_input"
	ToolOutput          Kind        = "tool_output"
	Document            Kind        = "document"
	MemoryDerived       Kind        = "memory_derived"
	SubagentOutput      Kind        = "subagent_output"
	LegacyUnknown       Kind        = "legacy_unknown"
	Public              Sensitivity = "public"
	Private             Sensitivity = "private"
	Secret              Sensitivity = "secret"
)

// Origin is assigned by the subsystem that reads the source, never parsed from
// a model response, document or tool result as a grant of instruction authority.
type Origin struct {
	Kind        Kind        `json:"kind"`
	SourceRefs  []string    `json:"source_refs"`
	ParentRefs  []string    `json:"parent_refs,omitempty"`
	Sensitivity Sensitivity `json:"sensitivity"`
}

var ErrSecret = errors.New("secret-class content cannot enter model context")

func (o Origin) IsInstruction() bool {
	return o.Kind == KernelInstruction || o.Kind == OperatorInstruction || o.Kind == OwnerPolicy
}

func (o Origin) normalized() Origin {
	if o.Kind == "" {
		o.Kind = LegacyUnknown
	}
	if o.Sensitivity == "" {
		o.Sensitivity = Private
	}
	o.SourceRefs = unique(o.SourceRefs)
	o.ParentRefs = unique(o.ParentRefs)
	return o
}

// Derive always lowers instruction authority to data, including summaries of
// authentic owner messages. Original source references survive delegation.
func Derive(kind Kind, parents ...Origin) Origin {
	o := Origin{Kind: kind, Sensitivity: Public}
	if o.IsInstruction() || kind == OwnerInput || kind == "" {
		o.Kind = MemoryDerived
	}
	for _, parent := range parents {
		parent = parent.normalized()
		o.SourceRefs = append(o.SourceRefs, parent.SourceRefs...)
		o.ParentRefs = append(o.ParentRefs, parent.ParentRefs...)
		if parent.Sensitivity == Secret {
			o.Sensitivity = Secret
		} else if parent.Sensitivity != Public && o.Sensitivity != Secret {
			o.Sensitivity = Private
		}
	}
	if len(parents) == 0 {
		o.Sensitivity = Private
	}
	return o.normalized()
}

func Instruction(text string, origin Origin) transport.InputItem {
	origin = origin.normalized()
	if !origin.IsInstruction() || len(origin.SourceRefs) == 0 {
		return Data(text, Derive(MemoryDerived, origin))
	}
	return transport.InputItem{Role: "system", Content: text, ContentType: "text/plain", Metadata: metadata(origin)}
}

func CurrentOwnerMessage(text, inputRef string) transport.InputItem {
	return transport.InputItem{Role: "user", Content: text, ContentType: "text/plain", Metadata: metadata(Origin{Kind: OwnerInput, SourceRefs: []string{inputRef}, Sensitivity: Private}.normalized())}
}

func Data(value any, origin Origin) transport.InputItem {
	origin = origin.normalized()
	if origin.IsInstruction() || origin.Kind == OwnerInput {
		origin = Derive(MemoryDerived, origin)
	}
	encoded, err := json.Marshal(struct {
		FormatVersion int    `json:"format_version"`
		Origin        Origin `json:"origin"`
		Data          any    `json:"data"`
	}{1, origin, value})
	if err != nil {
		encoded = []byte(`{"format_version":1,"data":"source unavailable"}`)
	}
	return transport.InputItem{Role: "user", Content: string(encoded), ContentType: "application/json", Metadata: metadata(origin)}
}

func metadata(origin Origin) map[string]any { return map[string]any{"butler_provenance": origin} }

func ItemOrigin(item transport.InputItem) Origin {
	value, ok := item.Metadata["butler_provenance"]
	if !ok {
		return Origin{Kind: LegacyUnknown, Sensitivity: Private}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return Origin{Kind: LegacyUnknown, Sensitivity: Private}
	}
	var origin Origin
	if json.Unmarshal(encoded, &origin) != nil {
		return Origin{Kind: LegacyUnknown, Sensitivity: Private}
	}
	return origin.normalized()
}

// Normalize is also the upgrade boundary: an old prepared system message with
// no typed source proof becomes historical data instead of promoted memory.
func Normalize(items []transport.InputItem) ([]transport.InputItem, error) {
	result := make([]transport.InputItem, 0, len(items))
	for _, item := range items {
		origin := ItemOrigin(item)
		if origin.Sensitivity == Secret {
			return nil, ErrSecret
		}
		if origin.IsInstruction() && len(origin.SourceRefs) > 0 && item.Role == "system" {
			result = append(result, item)
			continue
		}
		if origin.Kind == OwnerInput && len(origin.SourceRefs) > 0 && item.Role == "user" {
			result = append(result, item)
			continue
		}
		if item.ContentType == "application/json" && !origin.IsInstruction() && origin.Kind != LegacyUnknown {
			item.Role = "user"
			result = append(result, item)
			continue
		}
		result = append(result, Data(map[string]any{"original_role": item.Role, "content": item.Content}, Derive(origin.Kind, origin)))
	}
	return result, nil
}

func unique(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			seen[value] = true
		}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
