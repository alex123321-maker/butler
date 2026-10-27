package app

import (
	"encoding/json"
	"regexp"
	"unicode/utf8"

	"github.com/butler/butler/internal/transport"
)

const behavior76LocalResponseMode = "whole_fixture_after_exact_response_rejection_v1"

var behavior76ResponseVersion = regexp.MustCompile(`^gemini-[a-zA-Z0-9][a-zA-Z0-9._-]{0,110}$`)

func behavior76ResponseReason(reason string) bool {
	switch reason {
	case "stream_json_invalid", "stream_usage_ambiguous", "stream_usage_invalid", "stream_usage_conflict",
		"stream_model_version_invalid", "stream_candidate_unsupported", "stream_candidate_unexpected",
		"stream_finish_rejected", "stream_part_invalid", "stream_part_unsupported", "stream_thought_invalid",
		"stream_function_ambiguous", "stream_content_missing":
		return true
	}
	return false
}

func behavior76ResponseMetadata(reason string, usage *transport.Usage, version string) bool {
	if usage != nil && (usage.Validate() != nil || usage.TotalTokens < usage.InputTokens+usage.OutputTokens) {
		return false
	}
	if version != "" && !behavior76ResponseVersion.MatchString(version) {
		return false
	}
	switch reason {
	case "stream_usage_ambiguous", "stream_usage_invalid", "stream_usage_conflict":
		return usage == nil
	case "stream_model_version_invalid":
		return version == ""
	}
	return true
}

// Only the adapter's complete safe projection can qualify. Partial text is
// retained evidence, never a final answer or an accepted tool proposal.
func behavior76ExactResponseEvents(events []map[string]any) bool {
	if len(events) < 2 || len(events[0]) != 1 || events[0]["type"] != transport.EventTypeRunStarted {
		return false
	}
	sequence, size := 0, 0
	for _, e := range events[1 : len(events)-1] {
		if len(e) != 2 || e["type"] != transport.EventTypeAssistantDelta {
			return false
		}
		d, ok := e["text_delta"].(*transport.AssistantDelta)
		if !ok || d == nil || d.DeltaType != "text" || d.Content == "" || !utf8.ValidString(d.Content) || d.SequenceNo < 1 || d.SequenceNo > 4 {
			return false
		}
		if sequence != 0 && sequence != d.SequenceNo {
			return false
		}
		sequence = d.SequenceNo
		if len(d.Content) > 16*1024*1024-size {
			return false
		}
		size += len(d.Content)
	}
	e := events[len(events)-1]
	if (len(e) != 4 && len(e) != 5) || e["type"] != transport.EventTypeTransportError || e["error_code"] != "provider_protocol_error" || e["http_status"] != 0 {
		return false
	}
	reason, ok := e["safe_reason"].(string)
	if !ok || !behavior76ResponseReason(reason) {
		return false
	}
	if len(e) == 5 {
		u, ok := e["usage"].(*transport.Usage)
		if !ok || u == nil || u.Validate() != nil {
			return false
		}
	}
	return true
}

func behavior76ExactStoredDelta(raw json.RawMessage) (*transport.AssistantDelta, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 3 {
		return nil, false
	}
	var d transport.AssistantDelta
	for key, value := range fields {
		if string(value) == "null" {
			return nil, false
		}
		switch key {
		case "DeltaType":
			if json.Unmarshal(value, &d.DeltaType) != nil {
				return nil, false
			}
		case "Content":
			if json.Unmarshal(value, &d.Content) != nil {
				return nil, false
			}
		case "SequenceNo":
			if json.Unmarshal(value, &d.SequenceNo) != nil {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return &d, true
}

func behavior76StoredMetadataMatches(u *transport.Usage, in, out, total, thinking *int64) bool {
	if u == nil {
		return in == nil && out == nil && total == nil && thinking == nil
	}
	if in == nil || out == nil || total == nil || *in != int64(u.InputTokens) || *out != int64(u.OutputTokens) || *total != int64(u.TotalTokens) {
		return false
	}
	if u.ThinkingTokens == nil {
		return thinking == nil
	}
	return thinking != nil && *thinking == int64(*u.ThinkingTokens)
}
