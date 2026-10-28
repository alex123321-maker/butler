package runtime

import (
	"testing"

	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
)

func TestFrozenObservationRequiresExactReadContract(t *testing.T) {
	runner := &stubRunner{result: Result{OK: true, NetworkState: "frozen"}}
	server := NewServer(runner, nil)
	obs := openTest(t, server)
	for _, change := range []string{"revision", "write", "credentials", "extra_args", "foreign_run"} {
		t.Run(change, func(t *testing.T) {
			req := browserCall("browser.freeze_snapshot", "run-1", map[string]any{"browser_session_id": obs.SessionID})
			switch change {
			case "revision":
				req.Contract.ContractRevision = "0"
			case "write":
				req.Contract.EffectContract.RetrySafety = "unsafe"
			case "credentials":
				req.ResolvedCredentials = []*runtimev1.ResolvedCredential{{Alias: "not-allowed"}}
			case "extra_args":
				req.ToolCall.ArgsJson = `{"browser_session_id":"` + obs.SessionID + `","url":"https://example.com/other"}`
			case "foreign_run":
				req.Context.RunId = "foreign"
				req.ToolCall.RunId = "foreign"
			}
			if response := executeTest(t, server, req); response.GetStatus() == "completed" {
				t.Fatal("invalid frozen observation reached the runner")
			}
		})
	}
	if len(runner.requests) != 1 {
		t.Fatal("invalid read executed")
	}
	resp := executeTest(t, server, browserCall("browser.freeze_snapshot", "run-1", map[string]any{"browser_session_id": obs.SessionID}))
	if resp.GetStatus() != "completed" || len(runner.requests) != 2 {
		t.Fatal("valid existing-context read refused")
	}
}
