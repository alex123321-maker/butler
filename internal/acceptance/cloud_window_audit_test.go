package acceptance

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// These collector envelopes are deliberately synthetic. There are no native
// calls, database mutations or real signing keys in this unit fixture.
func cloudWindowFixture(t *testing.T) (Corpus, GoldenApproval, []byte, cloudWindowEnvelope, CloudWindowExpectation) {
	t.Helper()
	c, a, key, p := cloudFixture(t)
	p.EvidenceKind = "native_cloud_observation" // Untrusted fixture claim, not native proof.
	p.CodeCommit = CloudAuditedCode
	p.WindowID = CloudAuditedWindow
	p.Runtime = "go1.27.2/race; sha256:" + strings.Repeat("a", 64)
	resources := fixedCloudWindowResources()
	p.ResourcesSHA256 = cloudDigest(resources)
	h := cloudHistorical{Status: "mixed_completed_history", Attempts: 45, Known: 20, Unknown: 25, Usage: make([]cloudHistoricalUsage, 45)}
	counts := []int{3, 2, 4, 4, 5, 2, 1, 0, 1, 5, 10, 0, 2, 2, 4}
	for i, n := range counts {
		h.Artifacts = append(h.Artifacts, cloudHistoricalArtifact{SHA: hashWindowBytes([]byte(fmt.Sprintf("synthetic-historic-%d", i))), Attempts: n})
	}
	h.SHA = historyArtifactDigest(h.Artifacts)
	for i := 0; i < 20; i++ {
		h.Usage[i].Input = intCloud(0)
		h.Usage[i].Output = intCloud(0)
	}
	h.Usage[0].Input = intCloud(74698)
	h.Usage[0].Output = intCloud(2210)
	q := cloudQualityFixture(t, c)
	single, multi := cloudComparisonFixture(t, c, p)
	start := q[0].StartUTC.Add(-time.Second)
	e := cloudWindowEnvelope{Status: "passed", Model: CloudRequestedModel, Release: CloudAuditedCode, History: h, Total: 297, New: 252, Maximum: 297, MaximumNew: 252, Resources: resources, Pin: p, Attempts: q, Single: single, Multi: multi, StoredQuality: json.RawMessage(`{"PRIVATE_STORED_METRIC":"ignored"}`), StoredComparison: json.RawMessage(`{}`), Start: start}
	ordinal := 45
	decorate := func(attempt *CloudAttempt, phase string) {
		ordinal++
		attempt.AttemptID = fmt.Sprintf("native-%03d", ordinal)
		attempt.ContextID = "shadow-" + cloudDigest([]string{CloudAuditedWindow, phase, attempt.CaseID, attempt.Role})[:32]
		attempt.ProviderModelVersion = stringCloud("gemini-3.8-flash-n")
		e.Ledger = append(e.Ledger, cloudChargedAttempt{ordinal, attempt.ContextID, "start", attempt.InputSHA256, attempt.StartUTC.Add(time.Millisecond)})
	}
	for i := range e.Attempts {
		decorate(&e.Attempts[i], "quality")
	}
	base := start.Add(10 * time.Minute)
	for i := range e.Single.Tasks {
		at := base.Add(time.Duration(i) * time.Second)
		s := &e.Single.Tasks[i]
		s.StartUTC = at
		s.EndUTC = at.Add(10 * time.Millisecond)
		s.Milliseconds = 10
		s.Attempts[0].StartUTC = s.StartUTC
		s.Attempts[0].EndUTC = s.EndUTC
		decorate(&s.Attempts[0], "comparison")
		m := &e.Multi.Tasks[i]
		m.StartUTC = at.Add(20 * time.Millisecond)
		m.EndUTC = at.Add(70 * time.Millisecond)
		m.Milliseconds = 50
		parent := "shadow-" + cloudDigest([]string{CloudAuditedWindow, "comparison", m.CaseID, CloudRoleSynthesis})[:32]
		for j := range m.Attempts {
			x := &m.Attempts[j]
			x.StartUTC = m.StartUTC.Add(time.Duration(j) * 20 * time.Millisecond)
			x.EndUTC = x.StartUTC.Add(10 * time.Millisecond)
			if j < 2 {
				x.ParentContextID = &parent
			}
			decorate(x, "comparison")
		}
	}
	e.End = e.Multi.Tasks[3].EndUTC.Add(time.Second)
	return c, a, key, e, CloudWindowExpectation{SourceSHA256: p.SourceSHA256, HistorySHA256: h.SHA, HardwareSHA256: p.HardwareSHA256}
}
func windowFixtureBytes(t *testing.T, e cloudWindowEnvelope) []byte {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func cloneWindowFixture(t *testing.T, e cloudWindowEnvelope) cloudWindowEnvelope {
	t.Helper()
	var out cloudWindowEnvelope
	if json.Unmarshal(windowFixtureBytes(t, e), &out) != nil {
		t.Fatal("fixture clone")
	}
	return out
}

func TestCloudWindowAuditCorrelatesEveryAttemptAndExportsOnlySafeFacts(t *testing.T) {
	c, a, key, e, want := cloudWindowFixture(t)
	raw := windowFixtureBytes(t, e)
	r, err := AuditCloudWindow(c, a, key, raw, want)
	if err != nil || r.Status != "CLOUD_WINDOW_AUDIT_PASS" || !r.CorrelationValid || !r.QualityValid || !r.ComparisonValid || r.NewAttempts != 252 || len(r.Attempts) != 252 || r.Cost.Attempts != 252 || r.Cost.KnownTotalTokens != 5040 || r.CarryForwardAttempts != 297 || r.HistoricalUnknown != 25 || r.HistoricalInput != 74698 || r.HistoricalOutput != 2210 || r.Execution || r.OriginalReportSHA256 != hashWindowBytes(raw) {
		t.Fatal("independent synthetic audit mismatch", err, r.Status, r.Reasons)
	}
	encoded, _ := json.Marshal(r)
	for _, private := range []string{`"output":`, `"case_id":`, `"documents":`, `"quality_attempts":`, `PRIVATE_STORED_METRIC`, `"signature":`} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("unsafe projection", private)
		}
	}
	for i, x := range r.Attempts {
		if x.Ordinal != 46+i || x.ModelVersion == nil || *x.ModelVersion != "gemini-3.8-flash-n" {
			t.Fatal("ordinal/version lost")
		}
	}
}

func TestCloudWindowAuditRetainsFailedPartialSpendAndUnknowns(t *testing.T) {
	c, a, key, e, want := cloudWindowFixture(t)
	e.Status = "failed"
	e.Attempts = e.Attempts[:2]
	e.Ledger = e.Ledger[:2]
	e.New = 2
	e.Total = 47
	e.Single.Tasks = []CloudComparisonTask{}
	e.Multi.Tasks = []CloudComparisonTask{}
	e.Attempts[1].Outcome = "transport_error"
	e.Attempts[1].Output = json.RawMessage("null")
	e.Attempts[1].InputTokens = nil
	e.Attempts[1].OutputTokens = nil
	e.Attempts[1].TotalTokens = nil
	r, err := AuditCloudWindow(c, a, key, windowFixtureBytes(t, e), want)
	if err != nil || r.Status != "NO_GO" || !r.CorrelationValid || r.CarryForwardAttempts != 47 || len(r.Attempts) != 2 || r.Cost.Attempts != 2 || r.Cost.KnownTotalTokens != 20 || r.Cost.UnknownInput != 1 || r.Cost.UnknownOutput != 1 || r.Cost.UnknownTotal != 1 || r.Cost.FullTokenCostKnown || r.Attempts[1].Outcome != "transport_error" || r.Attempts[1].InputTokens != nil {
		t.Fatal("failed spend hidden", err, r.Status, r.Reasons)
	}
	perfect := cloudWindowFixtureStatusFailed(t)
	r, err = AuditCloudWindow(c, a, key, windowFixtureBytes(t, perfect), want)
	if err != nil || r.Status != "NO_GO" || !r.Quality.ThresholdsPassed || !strings.Contains(strings.Join(r.Reasons, ","), "collector_failed") {
		t.Fatal("failed collector promoted", err, r.Status)
	}
}
func cloudWindowFixtureStatusFailed(t *testing.T) cloudWindowEnvelope {
	_, _, _, e, _ := cloudWindowFixture(t)
	e.Status = "failed"
	return e
}

func TestCloudWindowAuditDeniesOmissionCounterPinAndMetadataTampering(t *testing.T) {
	c, a, key, e, want := cloudWindowFixture(t)
	for _, mode := range []string{"counter", "ledger_omit", "ordinal_gap", "context", "input", "omit_child", "old_null", "old_hash", "source", "hardware", "code", "scope", "role", "unsafe_runtime", "unsafe_version", "overlap"} {
		t.Run(mode, func(t *testing.T) {
			bad := cloneWindowFixture(t, e)
			switch mode {
			case "counter":
				bad.Total--
			case "ledger_omit":
				bad.Ledger = bad.Ledger[1:]
			case "ordinal_gap":
				bad.Ledger[0].Ordinal = 298
			case "context":
				bad.Ledger[1].Context = bad.Ledger[0].Context
			case "input":
				bad.Ledger[0].InputSHA = strings.Repeat("f", 64)
			case "omit_child":
				bad.Multi.Tasks[0].Attempts = bad.Multi.Tasks[0].Attempts[:2]
			case "old_null":
				bad.History.Usage[44].Input = intCloud(0)
			case "old_hash":
				bad.History.Artifacts[0].SHA = strings.Repeat("f", 64)
			case "source":
				bad.Pin.SourceSHA256 = strings.Repeat("f", 64)
			case "hardware":
				bad.Pin.HardwareSHA256 = strings.Repeat("f", 64)
			case "code":
				bad.Pin.CodeCommit = strings.Repeat("f", 40)
			case "scope":
				bad.Pin.WindowID = "new-window"
			case "role":
				bad.Attempts[0].Role = "PRIVATE_ROLE"
			case "unsafe_runtime":
				bad.Pin.Runtime = "PRIVATE_RUNTIME"
			case "unsafe_version":
				bad.Attempts[0].ProviderModelVersion = stringCloud("PRIVATE_VERSION")
			case "overlap":
				bad.Attempts[1].StartUTC = bad.Attempts[0].StartUTC
				bad.Attempts[1].EndUTC = bad.Attempts[0].EndUTC
				bad.Ledger[1].Created = bad.Attempts[1].StartUTC.Add(time.Millisecond)
			}
			if _, err := AuditCloudWindow(c, a, key, windowFixtureBytes(t, bad), want); err == nil {
				t.Fatal("tampered window accepted", mode)
			}
		})
	}
	raw := windowFixtureBytes(t, e)
	for _, bad := range [][]byte{append(raw, []byte(` {}`)...), []byte(strings.Replace(string(raw), `"native_attempts":297`, `"Native_attempts":297`, 1)), []byte(strings.Replace(string(raw), `"attempt":46`, `"attempt":46,"attempt":47`, 1)), make([]byte, CloudWindowReportLimit+1)} {
		if _, err := AuditCloudWindow(c, a, key, bad, want); err == nil {
			t.Fatal("invalid envelope accepted")
		}
	}
}

func TestCloudWindowAuditKnownOtherFamilyNoGoAndMissingVersionUnavailable(t *testing.T) {
	c, a, key, e, want := cloudWindowFixture(t)
	setVersion := func(value *string) {
		for i := range e.Attempts {
			e.Attempts[i].ProviderModelVersion = value
		}
		for _, run := range []*CloudComparisonRun{&e.Single, &e.Multi} {
			for i := range run.Tasks {
				for j := range run.Tasks[i].Attempts {
					run.Tasks[i].Attempts[j].ProviderModelVersion = value
				}
			}
		}
	}
	setVersion(stringCloud("gemini-2.5-flash"))
	r, err := AuditCloudWindow(c, a, key, windowFixtureBytes(t, e), want)
	if err != nil || r.Status != "NO_GO" || r.ModelFamily != "other_reported_family" {
		t.Fatal("foreign known family promoted", err, r.Status)
	}
	setVersion(nil)
	r, err = AuditCloudWindow(c, a, key, windowFixtureBytes(t, e), want)
	if err != nil || r.Status != "CLOUD_WINDOW_AUDIT_PASS" || r.ModelFamily != "unavailable" || r.UnknownVersions != 252 || r.Pin.ProviderModelVersion != nil {
		t.Fatal("unknown version invented", err, r.Status, r.Reasons)
	}
}
