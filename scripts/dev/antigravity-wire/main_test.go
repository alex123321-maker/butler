package main

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
)

func TestRejectAnyExternalInterfaceEvenWhenDown(t *testing.T) {
	loopback := net.Interface{Name: "lo", Flags: net.FlagLoopback | net.FlagUp}
	if !loopbackOnly([]net.Interface{loopback}) {
		t.Fatal("loopback denied")
	}
	for _, ifaces := range [][]net.Interface{nil, {loopback, {Name: "eth0"}}, {loopback, {Name: "eth0", Flags: net.FlagUp}}} {
		if loopbackOnly(ifaces) {
			t.Fatal("non-isolated network accepted")
		}
	}
}

func TestFixedProxyHostAllowlist(t *testing.T) {
	if !allowedHost("aicode.googleapis.com") || !allowedHost("daily-cloudcode-pa.googleapis.com") {
		t.Fatal("fixture host denied")
	}
	for _, host := range []string{"evil.invalid", "aicode.googleapis.com.evil.invalid", "user@aicode.googleapis.com", "127.0.0.1", "oauth2.googleapis.com/path"} {
		if allowedHost(host) {
			t.Fatal("arbitrary host accepted")
		}
	}
}

func TestRequestProjectionOmitsBodiesIdentityAndAuthorization(t *testing.T) {
	private := "do-not-release-fixture-content"
	input := map[string]any{
		"aicode":    map[string]any{"experience": "gemini-3.8-flash-tiered"},
		"sessionId": private, "project": private, "authorization": private,
		"systemInstruction": map[string]any{"parts": []any{map[string]any{"text": private}}},
		"contents":          []any{map[string]any{"role": "user", "parts": []any{map[string]any{"text": prompt + private, "thoughtSignature": private}}}},
		"generationConfig":  map[string]any{"maxOutputTokens": float64(8192), "responseMimeType": "application/json", "unknown": private},
		"request":           map[string]any{"sessionId": private, "contents": []any{map[string]any{"role": "model", "parts": []any{map[string]any{"text": private}}}}},
	}
	encoded, err := json.Marshal(summarize(input))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), private) || strings.Contains(string(encoded), "thoughtSignature") || strings.Contains(string(encoded), "authorization") {
		t.Fatal("private content released")
	}
	for _, expected := range []string{`"experience_expected":true`, `"maxOutputTokens":8192`, `"prompt_seen":true`, `"sessionId_present":true`} {
		if !strings.Contains(string(encoded), expected) {
			t.Fatalf("missing fixed comparison field %s", expected)
		}
	}
}
