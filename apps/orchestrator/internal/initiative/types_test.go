package initiative

import (
	"testing"
	"time"
)

func testSource() Source {
	return Source{ID: "project", OwnerID: "owner", GoalID: "goal", MandateID: "mandate", SessionKey: "web:owner", ProjectRef: "repo:fixture", Purpose: "maintenance", TaskGoal: "Подготовить проверенное исправление", ExecutionPolicy: "prepare_only", CompletionCriteria: []string{"проверены критерии проекта"}, TimeZone: "UTC", ActiveStart: 8, ActiveEnd: 20, FailureThreshold: 3, ConfidenceThreshold: 80, CooldownSeconds: 3600, Enabled: true, Revision: 1}
}
func testEvent(at time.Time) Event {
	return Event{ID: "error:one", SourceID: "project", Kind: "project_error", ConditionDigest: hash("same normalized build failure"), ResourceRevision: hash("commit1"), EvidenceDigest: hash("distinct failure1"), Confidence: 95, Importance: 70, ObservedAt: at}
}

func TestObservationFiltersBeforeAnyModelOrTaskAdmission(t *testing.T) {
	at := instant(t, "2026-10-10T12:00:00Z")
	s, e := testSource(), testEvent(at)
	if status, reason, _ := Decide(s, e, 2, at); status != "observing" || reason != "repeat_threshold_not_met" {
		t.Fatal("one repeated error admitted work")
	}
	if status, _, _ := Decide(s, e, 3, at); status != "ready" {
		t.Fatal("third independent failure not considered")
	}
	e.Confidence = 79
	if status, _, _ := Decide(s, e, 3, at); status != "proposed" {
		t.Fatal("low confidence admitted model")
	}
	e.Confidence = 95
	e.ProducedByButler = true
	if status, reason, _ := Decide(s, e, 3, at); status != "suppressed" || reason != "own_change" {
		t.Fatal("own fix starts another loop")
	}
	e.ProducedByButler = false
	e.Kind = "periodic"
	if status, reason, _ := Decide(s, e, 3, at); status != "observing" || reason != "empty_check" {
		t.Fatal("empty check disturbs owner")
	}
	e.Kind = "project_error"
	night := instant(t, "2026-10-10T21:00:00Z")
	if status, reason, next := Decide(s, e, 3, night); status != "deferred" || reason != "outside_active_hours" || !next.Equal(instant(t, "2026-10-11T08:00:00Z")) {
		t.Fatal("active hours ignored")
	}
}

func TestSemanticIdentityRetainsOwnerAuthorityAndResourceRevision(t *testing.T) {
	at := instant(t, "2026-10-10T12:00:00Z")
	s, e := testSource(), testEvent(at)
	baseline := e.fingerprint(s)
	e.ID = "redelivered"
	e.EvidenceDigest = hash("new failure")
	e.ObservedAt = at.Add(time.Minute)
	if e.fingerprint(s) != baseline {
		t.Fatal("delivery metadata prevents semantic coalescing")
	}
	for _, field := range []string{"owner", "goal", "mandate", "purpose", "project", "resource"} {
		copySource, copyEvent := s, e
		switch field {
		case "owner":
			copySource.OwnerID = "another"
		case "goal":
			copySource.GoalID = "another"
		case "mandate":
			copySource.MandateID = "another"
		case "purpose":
			copySource.Purpose = "another"
		case "project":
			copySource.ProjectRef = "another"
		case "resource":
			copyEvent.ResourceRevision = hash("another")
		}
		if copyEvent.fingerprint(copySource) == baseline {
			t.Fatalf("%s widened semantic identity", field)
		}
	}
}

func TestBrokerObservationResourceIsExactCredentialFreeData(t *testing.T) {
	s := testSource()
	s.ObserverKind, s.ProjectRevision = "broker_project", hash("revision")
	s.CheckManifestHash, s.CheckToolName = hash("manifest"), "extension.project.inspect"
	for _, resource := range []string{"https://example.invalid/project", "http://external.test/project"} {
		s.CheckResource = resource
		if s.Validate() != nil {
			t.Fatal("recorded approved HTTP(S) resource was rejected")
		}
	}
	for _, resource := range []string{"file:///project", "https://owner:secret@example.invalid/project", "https://example.invalid/project?token=secret", "https://example.invalid/project#secret", "https://example.invalid/pro%6aect", "https://example.invalid"} {
		s.CheckResource = resource
		if s.Validate() == nil {
			t.Fatal("credential-bearing or ambiguous observation resource accepted")
		}
	}
}
