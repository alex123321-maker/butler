package extraction

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/security/content"
)

func digest(v any) string {
	b, _ := json.Marshal(v)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func field(s *pb.Span, offset uint32) Field {
	return Field{Type: s.Type, Text: s.Text, Start: s.Start + offset, End: s.End + offset, Score: s.Confidence, Method: "model_span"}
}
func records(src catalog.Source, out *pb.ExtractOutput, offset uint32, zone string) []Record {
	var all []Record
	add := func(kind string, fields []Field, unresolved []string) {
		sort.Slice(fields, func(i, j int) bool {
			if fields[i].Start != fields[j].Start {
				return fields[i].Start < fields[j].Start
			}
			return fields[i].Type < fields[j].Type
		})
		r := Record{Kind: kind, Fields: fields, Unresolved: append([]string(nil), unresolved...), SourceID: src.ID, SourceRevision: src.Revision, SourceHash: src.Hash, SourceKind: src.Kind, SourceAuthor: src.Author, SourceLocator: append(json.RawMessage(nil), src.Locator...), EventTime: src.EventTime, IngestedAt: src.ExtractedAt, Model: Model, Revision: Revision, Runtime: Runtime, Schema: Schema, Normalizer: Normalizer, Score: 1, CandidateOnly: true}
		r.Quote, r.QuoteStart, r.QuoteEnd, r.QuoteClipped = quotedClause(src.Body, fields)
		r.Origin = content.Derive(content.MemoryDerived, src.Origin)
		r.Cues = literalCues(r.Quote, r.QuoteStart)
		seen := map[string]bool{}
		for _, f := range fields {
			seen[f.Type] = true
			r.Score = min(r.Score, f.Score)
			if f.Type == "deadline" {
				r.Times = append(r.Times, NormalizeTime(f.Text, src.EventTime, zone))
			}
		}
		if kind == "constraint" {
			for _, name := range []string{"action", "resource"} {
				if !seen[name] {
					r.Unresolved = append(r.Unresolved, name)
				}
			}
		}
		sort.Strings(r.Unresolved)
		all = append(all, r)
	}
	for _, s := range out.Spans {
		add("entity:"+s.Type, []Field{field(s, offset)}, nil)
	}
	// Preserve annotated source restrictions even when the model fails to group
	// its entity spans into a record. This is a fragment, not an inferred action
	// relation: action polarity/exception scope remain explicitly unresolved.
	groups := map[string][]Field{}
	for _, s := range out.Spans {
		f := field(s, offset)
		_, left, right, _ := quotedClause(src.Body, []Field{f})
		key := digest([]uint32{left, right})
		groups[key] = append(groups[key], f)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fields := groups[k]
		quote, left, _, _ := quotedClause(src.Body, fields)
		cues := literalCues(quote, left)
		action := false
		for _, f := range fields {
			action = action || f.Type == "action"
		}
		if action && (cues.Negated || cues.Conditional || cues.Exception || cues.Historical) {
			add("constraint_fragment", fields, []string{"action_polarity", "exception_scope"})
		}
	}
	for _, rel := range out.Relations {
		add("relation:"+rel.Type, []Field{field(rel.Head, offset), field(rel.Tail, offset)}, nil)
	}
	for _, rec := range out.Records {
		var fields []Field
		for _, f := range rec.Fields {
			fields = append(fields, field(f, offset))
		}
		add(rec.Type, fields, rec.UnresolvedFields)
	}
	return all
}
func recordKey(r Record) string {
	// Confidence is metadata, not semantic identity or a truth probability.
	fields := append([]Field(nil), r.Fields...)
	for i := range fields {
		fields[i].Score = 0
	}
	return digest([]any{r.SourceHash, r.Kind, fields, r.Unresolved, r.Schema, r.Revision, r.Normalizer})
}
func uniqueRecords(all []Record) []Record {
	m := map[string]Record{}
	for _, r := range all {
		k := recordKey(r)
		prior, ok := m[k]
		if !ok || r.Score > prior.Score {
			m[k] = r
		}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make([]Record, 0, len(keys))
	for _, k := range keys {
		result = append(result, m[k])
	}
	return result
}

type window struct {
	Text  string
	Start uint32
}

func windows(text string) ([]window, error) {
	runes := []rune(text)
	var out []window
	for start := 0; start < len(runes); {
		end := start
		bytes := 0
		for end < len(runes) {
			n := len(string(runes[end]))
			if bytes+n > 1200 {
				break
			}
			bytes += n
			end++
		}
		if end == start || len(out) >= 32 {
			return nil, ErrInvalid
		}
		out = append(out, window{Text: string(runes[start:end]), Start: uint32(start)})
		if end == len(runes) {
			break
		}
		start = max(start+1, end-80)
	}
	return out, nil
}

func setTime(all []Record, at time.Time) {
	for i := range all {
		all[i].ExtractedAt = at
	}
}
