package antigravity

import (
	"strings"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestAuthRejectionOnlyProjectsExactSafeReasons(t *testing.T) {
	for _, test := range []struct{ body, want string }{
		{`{"error":{"status":"PERMISSION_DENIED","message":"PRIVATE_TOKEN","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"ACCESS_TOKEN_EXPIRED","metadata":{"account":"PRIVATE_ACCOUNT"}}]}}`, "upstream_access_token_expired"},
		{`{"error":{"status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"ACCESS_TOKEN_SCOPE_INSUFFICIENT"}]}}`, "upstream_scope_insufficient"},
		{`{"error":{"status":"PERMISSION_DENIED","message":"ACCESS_TOKEN_EXPIRED PRIVATE_TOKEN"}}`, "upstream_permission_denied"},
		{`{"error":{"status":"PRIVATE_REASON","details":[{"@type":"foreign","reason":"ACCESS_TOKEN_EXPIRED"}]}}`, ""},
		{`{"error":{"status":"PERMISSION_DENIED","status":"UNAUTHENTICATED"}}`, ""},
	} {
		err := statusError(403, strings.NewReader(test.body))
		if transport.DiagnosticReason(err) != test.want || transport.DiagnosticHTTPStatus(err) != 403 || strings.Contains(err.Error(), "PRIVATE") {
			t.Fatal("unsafe auth diagnostic", transport.DiagnosticReason(err))
		}
	}
}
