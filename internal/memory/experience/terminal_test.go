package experience

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
)

func terminalObservationFixture() Observation {
	o := observationFixture()
	o.ResultID, o.AgentReport = "", ""
	o.TerminalAttempt = &TerminalAttempt{InputID: "stopped-input", InputStatus: "blocked", Attempts: 1, Checkpoint: "tool_result_recorded", InputHash: strings.Repeat("a", 64), InputRevision: 1, Reason: "effect_unknown", RunState: "tool_running"}
	return o
}

func TestTerminalObservationCannotInventFinalOrVerification(t *testing.T) {
	o := terminalObservationFixture()
	if o.Validate() != nil {
		t.Fatal("actual stopped UNKNOWN observation rejected")
	}
	for _, field := range []string{"result", "report", "verified", "live", "cancelled", "reason", "revision"} {
		t.Run(field, func(t *testing.T) {
			v := terminalObservationFixture()
			switch field {
			case "result":
				v.ResultID = "fabricated-final"
			case "report":
				v.AgentReport = "fabricated assistant final"
			case "verified":
				v.OwnerVerdict, v.OwnerResultID, v.VerifiedCriteria = "verified", "owner-result", []string{"success"}
			case "live":
				v.TerminalAttempt.InputStatus = "running"
			case "cancelled":
				v.TerminalAttempt.RunState = "cancelled"
			case "reason":
				v.TerminalAttempt.Reason = "arbitrary private error text"
			case "revision":
				v.TerminalAttempt.InputRevision = 0
			}
			if v.Validate() == nil {
				t.Fatal("stopped observation gained unsupported evidence")
			}
		})
	}
	note, _ := json.Marshal(notesFixture())
	raw, _ := json.Marshal(o)
	body, err := (Handler{}).Validate([]catalog.Source{{Body: string(raw)}}, string(note))
	if err != nil || !strings.Contains(body, `"verification":"task_unverified"`) || !strings.Contains(body, `"effect":"effect_unknown"`) || !strings.Contains(body, `"result_id":""`) || !strings.Contains(body, `"agent_report":""`) {
		t.Fatal("note changed actual terminal/effect evidence", err)
	}
}
