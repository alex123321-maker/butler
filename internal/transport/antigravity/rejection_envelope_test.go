package antigravity

import (
	"strings"
	"testing"
)

func TestBadRequestEnvelopeShapesStayPrivate(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{"[{\"error\":{\"status\":\"INVALID_ARGUMENT\",\"message\":\"PRIVATE invalid model PRIVATE_PROJECT\"}}]", "upstream_model_rejected"},
		{"data: [{\"error\":{\"status\":\"INVALID_ARGUMENT\",\"message\":\"PRIVATE thinkingBudget\"}}]\n\n", "upstream_thinking_rejected"},
		{"<html>PRIVATE</html>", "upstream_bad_request_non_json"},
		{"{\"error\":{\"status\":\"INVALID_ARGUMENT\",\"message\":\"PRIVATE maxOutputTokens\"}}", "upstream_output_limit_rejected"},
		{"{\"error\":{\"status\":\"INVALID_ARGUMENT\",\"message\":\"PRIVATE systemInstruction\"}}", "upstream_system_instruction_rejected"},
		{"[{\"error\":{\"status\":\"INVALID_ARGUMENT\"}},{\"error\":{\"status\":\"INVALID_ARGUMENT\"}}]", ""},
		{"{\"error\":{\"status\":\"INVALID_ARGUMENT\",\"status\":\"PRIVATE\"}}", ""},
	} {
		got := invalidArgumentReason(strings.NewReader(tc.body))
		if got != tc.want || strings.Contains(got, "PRIVATE") {
			t.Fatal("unsafe rejection projection", got)
		}
	}
}
