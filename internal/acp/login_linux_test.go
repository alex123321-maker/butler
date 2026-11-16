//go:build linux

package acp

import (
	"strings"
	"testing"
)

func TestLoginDiagnosticsNeverReleaseRawCLIText(t *testing.T) {
	for _, message := range []string{"timeout private-canary", "invalid_grant private-canary", "proxyconnect private-canary", "unexpected EOF private-canary", "unrecognized private-canary"} {
		reason := loginFailureReason(classifyLoginFailure(message))
		if strings.Contains(reason, "private-canary") || reason == "" {
			t.Fatal("raw Google login message escaped")
		}
	}
}

func TestLoginCodeAcceptsOnlyCodeOrExactGoogleCallback(t *testing.T) {
	for _, raw := range []string{"fixture-code-12345\r\n", "https://antigravity.google/oauth-callback?code=fixture-code-12345"} {
		if code, ok := normalizeLoginCode(raw); !ok || code != "fixture-code-12345" {
			t.Fatal("valid one-use code rejected")
		}
	}
	for _, raw := range []string{"https://untrusted.test/?code=fixture-code-12345", "https://antigravity.google/not-callback?code=fixture-code-12345", "code\nsecond-code", "too-short", "https://antigravity.google/oauth-callback?code=fixture-code-12345&code=other"} {
		if _, ok := normalizeLoginCode(raw); ok {
			t.Fatal("ambiguous private code accepted")
		}
	}
}
