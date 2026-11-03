package browserspa

// Diagnostic is a closed content-free category. Never echo runtime exceptions,
// URLs, frame bodies, headers, certificate material or query values as errors.
func Diagnostic(reason string) string {
	switch reason {
	case "profile_denied", "query_failed", "query_denied", "query_unsettled", "method_denied", "destination_denied", "authority_denied", "client_message_denied", "server_message_denied", "websocket_denied", "websocket_protocol_denied", "content_encoding_denied", "traffic_limit", "request_limit", "connection_limit", "websocket_limit", "message_limit", "query_limit", "response_denied", "response_lost", "websocket_lost", "runtime_lost", "proof_invalid", "relay_unavailable", "filter_unsupported", "finish_unknown":
		return "spa_" + reason
	default:
		return "spa_query_failed"
	}
}
func ValidDiagnostic(code string) bool {
	if len(code) < 5 || code[:4] != "spa_" {
		return false
	}
	return Diagnostic(code[4:]) == code
}
