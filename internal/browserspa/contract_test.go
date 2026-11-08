package browserspa

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfileArgsAndDestinationCanonicalization(t *testing.T) {
	for _, raw := range []string{
		`{"url":"https://rp.example/catalog/","filter":{"selector":"input[name=filter]","value":"books"}}`,
		`{"url":"https://rp.example/","ready_selector":"main","settle_ms":3000}`,
	} {
		if _, err := Parse(raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{
		`{"url":"http://rp.example/"}`, `{"url":"https://127.0.0.1/"}`, `{"url":"https://169.254.169.254/"}`,
		`{"url":"https://u:p@rp.example/"}`, `{"url":"https://rp.example/","credential_ref":"x"}`,
		`{"url":"https://rp.example/","filter":{"selector":"input","value":{"type":"credential_ref"}}}`,
		`{"url":"https://rp.example/","settle_ms":3001}`, `{"url":"https://rp.example/","filter":{"selector":"","value":"x"}}`,
		`{"url":"https://rp.example/%2e%2e/private"}`, `{"url":"https://rp.example/a/../private"}`,
	} {
		if _, err := Parse(raw); err == nil {
			t.Fatal("invalid profile arguments accepted")
		}
	}
	for _, target := range []string{"https://foreign.example/", "https://rp.example/%2Fprivate", "https://rp.example/a%5cb", "https://rp.example/a/../b", "https://rp.example/#secret", "file:///etc/passwd"} {
		if NetworkURL("https://rp.example/", target) {
			t.Fatal("destination canonicalization bypass")
		}
	}
}

func TestReadProofDoesNotExportAContextOrClaimCommit(t *testing.T) {
	a := Args{URL: "https://rp.example/catalog/"}
	base := map[string]any{"ok": true, "final_url": a.URL, "spa_profile": Profile, "network_state": "frozen", "spa_network": NetworkStats{Requests: 8, Queries: 2, CompletedQueries: 2, WebSockets: 1, Messages: 1, MeteredBytes: 1024}}
	raw, _ := json.Marshal(base)
	if _, err := ReadResultURL(string(raw), a); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"context", "nonce", "foreign", "wrong_profile", "thawed", "pending", "excess", "error"} {
		t.Run(mode, func(t *testing.T) {
			copy := map[string]any{}
			for k, v := range base {
				copy[k] = v
			}
			switch mode {
			case "context":
				copy["browser_session_id"] = strings.Repeat("a", 32)
			case "nonce":
				copy["spa_nonce"] = "private"
			case "foreign":
				copy["final_url"] = "https://foreign.example/"
			case "wrong_profile":
				copy["spa_profile"] = "unreviewed"
			case "thawed":
				copy["network_state"] = "read_only"
			case "pending":
				copy["spa_network"] = NetworkStats{Requests: 2, Queries: 1, PendingQueries: 1}
			case "excess":
				copy["spa_network"] = NetworkStats{Requests: 129}
			case "error":
				copy["error"] = map[string]string{"code": "failed"}
			}
			raw, _ := json.Marshal(copy)
			if _, err := ReadResultURL(string(raw), a); err == nil {
				t.Fatal("invalid proof accepted")
			}
		})
	}
}
