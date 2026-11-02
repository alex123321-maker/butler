package research

import (
	"encoding/json"
	"github.com/butler/butler/internal/memory/transcript"
	"testing"
	"time"
)

func TestResearchVerifiedLoginSupportsObservedAccountWithoutRefetch(t *testing.T) {
	for _, mode := range []string{"verified", "no_challenge", "no_proof", "failed", "unfinished", "other_run", "generic_snapshot"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Now()
			proof := map[string]any{"protocol": "browser_login_v1", "submitted_once": true, "authenticated_read": true, "anonymous_challenge": true}
			out := map[string]any{"ok": true, "browser_session_id": "b", "network_state": "frozen", "observation_version": 2, "final_url": "https://issuer.example/account", "text": "Account reference B-432.", "authentication": proof}
			call := transcript.ToolCall{RunID: "r", ToolName: "browser.login", Status: "completed", FinishedAt: &now, ArgsJSON: `{}`}
			switch mode {
			case "no_challenge":
				proof["anonymous_challenge"] = false
			case "no_proof":
				delete(out, "authentication")
			case "failed":
				call.Status = "effect_unknown"
			case "unfinished":
				call.FinishedAt = nil
			case "other_run":
				call.RunID = "old"
			case "generic_snapshot":
				call.ToolName = "browser.snapshot"
			}
			raw, _ := json.Marshal(out)
			call.ResultJSON = string(raw)
			pages, forced := Pages([]transcript.ToolCall{call}, "r")
			if forced || (len(pages) == 1) != (mode == "verified") {
				t.Fatal("login provenance changed")
			}
			if mode == "verified" && !Supports(Evidence{URL: "https://issuer.example/account", Quote: "Account reference B-432.", Role: "primary"}, pages, now) {
				t.Fatal("verified login required redundant read")
			}
			_ = IsReadAttempt(call) // unfinished login must not dereference a missing time.
		})
	}
}
