package communication_test

import (
	"encoding/json"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/internal/communicationcontrol"
)

func TestCommunicationAllResultFormatsPreserveObligationAndUnverifiedCompletionIntegration(t *testing.T) {
	for _, kind := range []string{"message", "photo", "report"} {
		t.Run(kind, func(t *testing.T) {
			f, imageRef := mediaFixture(t)
			a := communicationcontrol.Args{SchemaVersion: 1, Purpose: "progress", Text: "Checking the requested fixture"}
			if kind == "photo" {
				a.Images = []communicationcontrol.Image{{ArtifactRef: imageRef, Alt: "Fixture"}}
			}
			if kind == "report" {
				a.Text = ""
				a.Title, a.Summary = "Fixture report", "Recorded observations"
				a.Blocks = []communicationcontrol.Block{{Type: "paragraph", Text: "Fixture output"}}
			}
			publish := func(effect string) {
				t.Helper()
				raw, _ := json.Marshal(a)
				if _, e := f.publish(t, effect, "communication."+kind, string(raw)); e != nil {
					t.Fatal(e)
				}
			}
			publish("progress")
			var state string
			if e := f.pool.QueryRow(f.ctx, `SELECT obligation_state FROM communication_admissions WHERE input_id=$1`, f.p.InputID).Scan(&state); e != nil || state != "open" {
				t.Fatal("progress fulfilled obligation", state, e)
			}
			a.Purpose = "result"
			publish("result")
			if decision, e := f.s.BeforeFinal(f.ctx, f.p.RunID, "internal final"); e != nil || decision != "ready" {
				t.Fatal(decision, e)
			}
			before := f.count(t, "delivery_outbox")
			if _, e := f.pool.Exec(f.ctx, `UPDATE runs SET current_state='finalizing',status='finalizing' WHERE run_id=$1`, f.p.RunID); e != nil {
				t.Fatal(e)
			}
			c := completion.New(f.pool)
			c.Communication = f.s
			c.DataGuard = f.s.SourceGuard
			request := completion.Request{RunID: f.p.RunID, SessionKey: f.p.SessionKey, TerminalObserved: true}
			for n := 0; n < 2; n++ {
				result, e := c.Save(f.ctx, request)
				if e != nil || result.Verdict != "unverified" {
					t.Fatal("publication verified task or final failed", result, e)
				}
			}
			if f.count(t, "delivery_outbox") != before || f.count(t, "communication_publications") != 2 {
				t.Fatal("frozen final created duplicate delivery")
			}
		})
	}
}
