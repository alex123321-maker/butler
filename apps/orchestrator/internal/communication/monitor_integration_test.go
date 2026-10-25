package communication_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/butler/butler/internal/communicationcontrol"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/research"
	"github.com/butler/butler/internal/schedulecontrol"
)

func monitorHash(v string) string { s := sha256.Sum256([]byte(v)); return hex.EncodeToString(s[:]) }
func monitorFixture(t *testing.T) *fixture {
	f := setup(t, fixtureOptions{Task: true, Quiet: true})
	sp := schedulecontrol.Spec{Title: "Check changing availability", Mode: "agent_task", Monitor: true, Instruction: "Check current primary source", CompletionCriteria: []string{"Observation recorded"}, Timing: schedulecontrol.Timing{Kind: "interval", TimeZone: "UTC", IntervalSeconds: 3600, MissedPolicy: "coalesce", GraceSeconds: 60}, ExpiresAt: time.Now().Add(time.Hour), MaxOccurrences: 3}
	raw, _ := json.Marshal(sp)
	_, err := f.pool.Exec(f.ctx, `INSERT INTO scheduled_jobs(schedule_id,owner_id,session_key,source_run_id,source_constraints,source_goal,execution_policy,approved,spec_json,state,expires_at,maximum_occurrences) VALUES('monitor',$1,$2,$3,'[]','Monitor','normal',TRUE,$4,'active',clock_timestamp()+interval '1 hour',3)`, f.p.OwnerID, f.p.SessionKey, f.p.RunID, raw)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.pool.Exec(f.ctx, `INSERT INTO scheduled_occurrences(occurrence_id,schedule_id,schedule_revision,slot_key,scheduled_at,state,reason,task_id,input_id) VALUES('now','monitor',1,'now',clock_timestamp(),'admitted','test',$1,$2)`, f.p.TaskID, f.p.InputID)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestCommunicationCheckedMonitorObservationAndDedupeIntegration(t *testing.T) {
	for _, tc := range []struct {
		name, status            string
		prior, evidence, notify bool
	}{
		{"waiting", "pending", false, true, false},
		{"unavailable", "unknown", false, false, true},
		{"same_unavailable_after_restart", "unknown", true, false, false},
		{"actual_confirmation", "confirmed", false, true, true},
		{"same_confirmation_after_restart", "confirmed", true, true, false},
		{"postponement", "postponed", false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := monitorFixture(t)
			a := communicationcontrol.Args{SchemaVersion: 1, Purpose: "result", Text: "Availability is confirmed. [Issuer](https://issuer.example/status)", Observation: &communicationcontrol.Observation{Status: tc.status, EventKey: "release:8.3"}}
			if tc.status == "pending" {
				a.Text = "Still awaiting the announced event. [Issuer](https://issuer.example/status)"
			}
			if tc.status == "unknown" {
				a.Text = "The current issuer page could not be checked; availability is unverified."
				a.Purpose = "blocked"
			}
			if tc.evidence {
				a.Evidence = []research.Evidence{{URL: "https://issuer.example/status", Quote: "The requested version is now available.", Role: "primary"}}
				now := time.Now()
				_, err := transcript.NewStore(f.pool).AppendToolCall(f.ctx, transcript.ToolCall{ToolCallID: "actual-read", RunID: f.p.RunID, ToolName: "web.fetch", Status: "completed", StartedAt: now, FinishedAt: &now, ResultJSON: `{"status_code":200,"final_url":"https://issuer.example/status","content_text":"The requested version is now available."}`})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				now := time.Now()
				if _, err := transcript.NewStore(f.pool).AppendToolCall(f.ctx, transcript.ToolCall{ToolCallID: "actual-attempt", RunID: f.p.RunID, ToolName: "web.fetch", Status: "failed", StartedAt: now, FinishedAt: &now, ResultJSON: `{}`}); err != nil {
					t.Fatal(err)
				}
			}
			if tc.prior {
				stateHash := monitorHash("monitor:release:8.3:" + tc.status + ":null")
				if tc.status == "unknown" {
					stateHash = monitorHash("monitor:unknown")
				}
				_, err := f.pool.Exec(f.ctx, `INSERT INTO scheduled_occurrences(occurrence_id,schedule_id,schedule_revision,slot_key,scheduled_at,state,reason) VALUES('previous','monitor',1,'previous',clock_timestamp()-interval '1 hour','admitted','test')`)
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.pool.Exec(f.ctx, `INSERT INTO schedule_observations(occurrence_id,schedule_id,schedule_revision,input_id,owner_id,run_id,effect_id,tool_call_id,payload_hash,event_key_hash,state,state_hash,notified,receipt_json) VALUES('previous','monitor',1,$1,$2,$3,'old-effect','old-call','old-payload','old-event',$4,$5,TRUE,'{}')`, f.p.InputID, f.p.OwnerID, f.p.RunID, tc.status, stateHash)
				if err != nil {
					t.Fatal(err)
				}
			}
			raw, _ := json.Marshal(a)
			r, err := f.publish(t, "observation", "communication.message", string(raw))
			if err != nil {
				t.Fatal(err)
			}
			if tc.notify != (r.Status == "accepted") || tc.notify != (r.PublicationRef != "") {
				t.Fatal(r)
			}
			pubs := f.count(t, "communication_publications")
			if tc.notify && pubs != 1 || !tc.notify && pubs != 0 {
				t.Fatal("wrong publication count", pubs)
			}
			restarted := *f.s
			f.s = &restarted
			_, err = f.publish(t, "observation", "communication.message", string(raw))
			if err != nil || f.count(t, "communication_publications") != pubs {
				t.Fatal("restart duplicated observation", err)
			}
			f.revoked.Store(true)
			if _, err = f.publish(t, "observation", "communication.message", string(raw)); err == nil {
				t.Fatal("revoked source allowed observation replay")
			}
		})
	}
}

func TestCommunicationCheckedMonitorRequiresEvidenceAndObservationIntegration(t *testing.T) {
	f := monitorFixture(t)
	if _, err := f.publish(t, "missing", "communication.message", message); err == nil {
		t.Fatal("timer silently claimed success without an observation")
	}
	a := communicationcontrol.Args{SchemaVersion: 1, Purpose: "result", Text: "It must be released because the planned time passed.", Observation: &communicationcontrol.Observation{Status: "confirmed", EventKey: "version"}}
	raw, _ := json.Marshal(a)
	if _, err := f.publish(t, "unsupported", "communication.message", string(raw)); err == nil {
		t.Fatal("clock passing became evidence")
	}
	if decision, err := f.s.BeforeFinal(f.ctx, f.p.RunID, `{"answer_state":"final","content":""}`); err != nil || decision != "correct" {
		t.Fatal("quiet completion skipped observation", decision, err)
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM schedule_observations`).Scan(&count); err != nil || count != 0 {
		t.Fatal("denied observations persisted", count, err)
	}
}
