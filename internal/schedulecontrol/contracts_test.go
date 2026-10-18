package schedulecontrol

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func validSpec() Spec {
	return Spec{Title: "Check project", Mode: "agent_task", Instruction: "Read project state", CompletionCriteria: []string{"Saved evidence"}, Timing: Timing{Kind: "once", TimeZone: "Asia/Tomsk", AfterSeconds: 600, MissedPolicy: "coalesce", GraceSeconds: 60}, ExpiresAt: time.Now().Add(24 * time.Hour).UTC(), MaxOccurrences: 1}
}
func TestScheduleArgumentsRejectAmbiguousAuthorityAndTiming(t *testing.T) {
	raw, _ := json.Marshal(Args{Job: ptr(validSpec())})
	if _, err := DecodeArgs("schedule.create", string(raw)); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"job":null}`, string(raw) + ` {}`, strings.Replace(string(raw), `"title":`, `"title":"other","title":`, 1), strings.Replace(string(raw), `"paused":false`, `"paused":false,"owner_id":"other"`, 1), strings.Replace(string(raw), `"Asia/Tomsk"`, `"Local"`, 1), strings.Replace(string(raw), `"max_occurrences":1`, `"max_occurrences":101`, 1), strings.Replace(string(raw), `"after_seconds":600`, `"after_seconds":600,"at":"2026-10-19T00:00:00Z"`, 1), strings.Replace(string(raw), `"paused":false`, `"paused":null`, 1)} {
		if _, err := DecodeArgs("schedule.create", body); err == nil {
			t.Fatalf("accepted ambiguous body %s", body)
		}
	}
	if _, err := DecodeArgs("schedule.cancel", `{"schedule_id":"sch_00000000000000000000000000000000","expected_revision":1,"job":null}`); err == nil {
		t.Fatal("cross-operation field accepted")
	}
}
func ptr(s Spec) *Spec { return &s }

type fakeService struct{ called int }

func (s *fakeService) ExecuteDispatched(context.Context, Envelope) (Receipt, error) {
	s.called++
	return Receipt{Schedules: []View{}}, nil
}
func TestSchedulePrivateControlRejectsForgedEnvelopeAndRedirect(t *testing.T) {
	const token = "schedule-private-test-token-at-least-32"
	service := &fakeService{}
	h, _ := NewHandler(token, service)
	server := httptest.NewServer(h)
	defer server.Close()
	client, err := NewClient(server.URL+Path, token)
	if err != nil {
		t.Fatal(err)
	}
	e := Envelope{RunID: "run", ToolCallID: "call", EffectID: "effect", Tool: "schedule.list", ArgsJSON: "{}", ContractHash: strings.Repeat("0", 64)}
	if _, err = client.ExecuteDispatched(context.Background(), e); err != nil || service.called != 1 {
		t.Fatal("private route", err)
	}
	for _, suffix := range []string{"?x=1", "?", "#frag", "/extra"} {
		if _, err = NewClient(server.URL+Path+suffix, token); err == nil {
			t.Fatal("accepted changed endpoint", suffix)
		}
	}
	bad, _ := NewClient(server.URL+Path, strings.Repeat("x", 32))
	if _, err = bad.ExecuteDispatched(context.Background(), e); err == nil || service.called != 1 {
		t.Fatal("service authentication failed")
	}
}
