package localmodels

import (
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
)

// MemorySchema is an immutable data schema, not a policy or authority classifier.
const MemorySchema = "butler-memory-v1"

var memoryFields = []string{"project", "component", "task", "resource", "action", "deadline", "prohibition", "exception", "condition", "temporal_qualifier"}

func MemoryFields() []string { return append([]string(nil), memoryFields...) }

var memoryRelations = map[string]bool{"action_resource": true, "action_deadline": true, "action_exception": true, "project_component": true}

func ValidateMemoryExtraction(text string, out *pb.ExtractOutput) error {
	if out == nil || out.SchemaVersion != MemorySchema || len(out.Spans)+len(out.Relations)+len(out.Records) > 128 {
		return fault(Protocol)
	}
	labels := map[string]bool{}
	for _, field := range memoryFields {
		labels[field] = true
	}
	runes := []rune(text)
	check := func(s *pb.Span) bool {
		return s != nil && labels[s.Type] && s.Start < s.End && int(s.End) <= len(runes) && string(runes[s.Start:s.End]) == s.Text && validScore(s.Confidence)
	}
	for _, s := range out.Spans {
		if !check(s) {
			return fault(Protocol)
		}
	}
	for _, r := range out.Relations {
		if r == nil || !memoryRelations[r.Type] || !check(r.Head) || !check(r.Tail) {
			return fault(Protocol)
		}
	}
	for _, r := range out.Records {
		if r == nil || r.Type != "constraint" || len(r.Fields) < 1 || len(r.Fields) > 10 {
			return fault(Protocol)
		}
		seen := map[string]bool{}
		for _, s := range r.Fields {
			if !check(s) || seen[s.Type] {
				return fault(Protocol)
			}
			seen[s.Type] = true
		}
		if !seen["action"] {
			return fault(Protocol)
		}
		for _, name := range r.UnresolvedFields {
			if !labels[name] || seen[name] {
				return fault(Protocol)
			}
		}
	}
	return nil
}
