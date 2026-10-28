package antigravity

// Match only transport-owned literals. Never classify provider text, JSON keys,
// reader errors or other untrusted content by substring. A reason identifies a
// local rejection branch, not whether the adapter or upstream caused the fault.
func streamDiagnosticReason(message string) string {
	switch message {
	case "invalid SSE response JSON":
		return "stream_json_invalid"
	case "ambiguous Cloud Code response envelope", "invalid Antigravity response envelope", "invalid Cloud Code response envelope":
		return "stream_envelope_invalid"
	case "ambiguous usage metadata":
		return "stream_usage_ambiguous"
	case "invalid usage metadata":
		return "stream_usage_invalid"
	case "conflicting complete usage metadata":
		return "stream_usage_conflict"
	case "invalid or conflicting observed model version":
		return "stream_model_version_invalid"
	case "multiple candidates are unsupported":
		return "stream_candidate_unsupported"
	case "unexpected candidate after completion":
		return "stream_candidate_unexpected"
	case "model response was blocked or failed":
		return "stream_finish_rejected"
	case "provider attempted automatic function execution":
		return "stream_automatic_function_execution"
	case "invalid model content part":
		return "stream_part_invalid"
	case "unsupported model content part":
		return "stream_part_unsupported"
	case "invalid thought marker", "invalid thought signature":
		return "stream_thought_invalid"
	case "ambiguous function content":
		return "stream_function_ambiguous"
	case "model content is missing text or function":
		return "stream_content_missing"
	case "stream ended before a terminal candidate", "Antigravity stream ended without a complete terminal response":
		return "stream_terminal_missing"
	case "SSE frame exceeds limit":
		return "stream_frame_limit"
	case "truncated function proposal":
		return "stream_function_truncated"
	case "model returned no usable text or function":
		return "stream_empty_answer"
	case "Antigravity returned a non-SSE response":
		return "http_non_sse_response"
	default:
		return ""
	}
}
