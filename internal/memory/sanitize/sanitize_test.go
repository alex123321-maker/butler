package sanitize

import (
	"strings"
	"testing"
)

func TestTextRedactsSensitiveValues(t *testing.T) {
	t.Parallel()
	input := "Authorization: Bearer sk-secret-token password=supersecret cookie value is abc123 postgres://user:pass@localhost/db"
	output := Text(input)
	for _, secret := range []string{"sk-secret-token", "supersecret", "abc123", "user:pass"} {
		if strings.Contains(output, secret) {
			t.Fatalf("expected %q to be redacted in %q", secret, output)
		}
	}
	for _, marker := range []string{"[REDACTED_TOKEN]", "[REDACTED_PASSWORD]", "[REDACTED_COOKIE]", "[REDACTED_DSN]"} {
		if !strings.Contains(output, marker) {
			t.Fatalf("expected marker %q in %q", marker, output)
		}
	}
}

func TestJSONRedactsSensitiveKeys(t *testing.T) {
	t.Parallel()
	input := `{"access_token":"abc","nested":{"password":"secret","cookie_bundle":"cookie=1"},"safe":"ok"}`
	output := JSON(input)
	for _, secret := range []string{"abc", "secret", "cookie=1"} {
		if strings.Contains(output, secret) {
			t.Fatalf("expected secret %q to be redacted in %q", secret, output)
		}
	}
	if !strings.Contains(output, `"safe":"ok"`) {
		t.Fatalf("expected safe field to remain in %q", output)
	}
	if !strings.Contains(output, `[REDACTED_TOKEN]`) || !strings.Contains(output, `[REDACTED_PASSWORD]`) || !strings.Contains(output, `[REDACTED_COOKIE]`) {
		t.Fatalf("expected redaction markers in %q", output)
	}
}

func TestTranscriptFieldHelpersSanitizeMessagesAndToolCalls(t *testing.T) {
	t.Parallel()
	joined := TranscriptMessageContent("my password is supersecret") +
		TranscriptMetadataJSON(`{"token":"abc"}`) +
		TranscriptToolArgsJSON(`{"authorization":"Bearer abc"}`) +
		TranscriptToolResultJSON(`{"cookie":"abc123"}`) +
		TranscriptToolErrorJSON(`{"dsn":"postgres://user:pass@host/db"}`)
	for _, secret := range []string{"supersecret", "abc123", "user:pass@host", "Bearer abc"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("expected secret %q to be redacted in %q", secret, joined)
		}
	}
	if !strings.Contains(joined, "[REDACTED") {
		t.Fatalf("expected redaction markers in %q", joined)
	}
}
