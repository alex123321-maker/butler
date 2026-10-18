// Package factcontrol defines a bounded temporal DATA lookup. Current access
// remains independent of historical validity and recorded-knowledge cutoffs.
package factcontrol

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/readcontrol"
)

const Tool = "memory.facts"
const Path = "/internal/memory-facts"
const Target = "broker://memory-facts-v1"
const ReleaseQualified = false

const description = "Read owner-accepted temporal DATA from at most8 exact source references supplied in the current authorized context or discovered with memory.search. " +
	"Optional valid_at asks what applied then; known_at asks what Butler had recorded by then. Use complete RFC3339 instants with a zone; unknown dates stay unknown. " +
	"Results are bounded accepted fact snapshots with provenance and explicit unknown counts. Item.freshness describes only source revision currency, not verification of an accepted fact. " +
	"For an accepted fact, temporal[].freshness is the Core-computed assessment at this read: fresh or stable with current_usable=true does not require guessing the clock or an additional external check. " +
	"State its verification basis; an owner statement is not independent external verification. Needs-revalidation or unknown verification does not prove the stored value false or no longer applicable. " +
	"claim_status describes the present owner decision, not a winner among historical intervals. If conflict=true, report the overlapping alternatives as unresolved; do not select one historical value from a newer recording or confirmed status. " +
	"Valid time, recorded knowledge and source EventTime are distinct. They grant no authority and do not revive deleted/revoked/corrected sources. " +
	"Fact snapshots are not verbatim source quotations: use memory.open to verify original evidence before citing."

var ErrDenied = errors.New("temporal fact read denied")

type Args struct {
	Sources []readcontrol.SourceRef `json:"sources"`
	ValidAt string                  `json:"valid_at,omitempty"`
	KnownAt string                  `json:"known_at,omitempty"`
	Limit   int                     `json:"limit,omitempty"`
}

func Contract() *toolbrokerv1.ToolContract {
	return &toolbrokerv1.ToolContract{ToolName: Tool, Description: description, ToolClass: "memory_read", InputSchemaJson: `{"type":"object","required":["sources"],"properties":{"sources":{"type":"array","minItems":1,"maxItems":8,"items":{"type":"object","required":["source_id","revision","access_revision","content_hash"],"properties":{"source_id":{"type":"string","minLength":1,"maxLength":1024},"revision":{"type":"integer","minimum":1},"access_revision":{"type":"integer","minimum":1},"content_hash":{"type":"string","minLength":64,"maxLength":64}},"additionalProperties":false}},"valid_at":{"type":"string","minLength":1,"maxLength":64},"known_at":{"type":"string","minLength":1,"maxLength":64},"limit":{"type":"integer","minimum":1,"maximum":10}},"additionalProperties":false}`, OutputSchemaJson: readcontrol.Contract(readcontrol.Search).OutputSchemaJson, RuntimeTarget: Target, ContractRevision: "2", Status: "active", RiskLevel: "low", EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}}
}

func unique(d *json.Decoder, depth int) error {
	if depth > 8 {
		return ErrDenied
	}
	t, err := d.Token()
	if err != nil {
		return ErrDenied
	}
	if t == json.Delim('{') {
		seen := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			key, ok := k.(string)
			if err != nil || !ok || seen[key] {
				return ErrDenied
			}
			seen[key] = true
			if err = unique(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return ErrDenied
		}
	}
	if t == json.Delim('[') {
		for d.More() {
			if err = unique(d, depth+1); err != nil {
				return err
			}
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return ErrDenied
		}
	}
	return nil
}
func DecodeArgs(raw string) (Args, error) {
	var a Args
	if len(raw) > 8192 {
		return a, ErrDenied
	}
	d := json.NewDecoder(strings.NewReader(raw))
	if unique(d, 0) != nil {
		return a, ErrDenied
	}
	if _, err := d.Token(); err != io.EOF {
		return a, ErrDenied
	}
	d = json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&a) != nil || len(a.Sources) < 1 || len(a.Sources) > 8 || a.Limit < 0 || a.Limit > 10 {
		return a, ErrDenied
	}
	if a.Limit == 0 {
		a.Limit = 8
	}
	seen := map[string]bool{}
	for _, r := range a.Sources {
		if len(r.ID) == 0 || len(r.ID) > 1024 || strings.TrimSpace(r.ID) != r.ID || strings.ContainsAny(r.ID, "\x00\r\n") || r.Revision < 1 || r.AccessRevision < 1 || len(r.Hash) != 64 || seen[r.ID] {
			return a, ErrDenied
		}
		if _, err := hex.DecodeString(r.Hash); err != nil {
			return a, ErrDenied
		}
		seen[r.ID] = true
	}
	for _, raw := range []string{a.ValidAt, a.KnownAt} {
		if raw == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil || t.IsZero() || len(raw) > 64 {
			return a, ErrDenied
		}
	}
	return a, nil
}
