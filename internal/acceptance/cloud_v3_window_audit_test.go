package acceptance

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Entirely synthetic; the native evidence label is an untrusted fixture claim.
func cloudV3WindowFixture(t *testing.T) (CloudV3Corpus, GoldenApproval, []byte, cloudV3WindowEnvelope, CloudV3WindowExpectation) {
	t.Helper()
	c, approval, key, pin, q := cloudV3Fixture(t)
	pin.EvidenceKind = "native_cloud_observation"
	pin.WindowID = CloudV3AuditedWindow
	pin.Runtime = "go1.27.2/race; sha256:" + strings.Repeat("a", 64)
	single, multi := cloudV3ComparisonFixture(t, c, pin)
	h := cloudHistorical{Status: "mixed_completed_history", Attempts: 248, Known: 222, Unknown: 26, Usage: make([]cloudHistoricalUsage, 248)}
	counts := []int{3, 2, 4, 4, 5, 2, 1, 0, 1, 5, 10, 0, 2, 2, 4}
	for i, n := range counts {
		h.Artifacts = append(h.Artifacts, cloudHistoricalArtifact{SHA: hashWindowBytes([]byte(fmt.Sprintf("synthetic-historic-%d", i))), Attempts: n})
	}
	old := historyArtifactDigest(h.Artifacts)
	prior228 := hashWindowBytes([]byte(old + "\n" + CloudV2PreviousPrivateSHA256 + "\n" + CloudV2PreviousAuditSHA256 + "\n"))
	h.Artifacts = append(h.Artifacts, cloudHistoricalArtifact{SHA: CloudV2PreviousAuditSHA256, Release: CloudAuditedCode, Attempts: 183})
	h.SHA = hashWindowBytes([]byte(prior228 + "\n" + CloudV3PreviousPrivateSHA256 + "\n" + CloudV3PreviousAuditSHA256 + "\n" + CloudV3PreviousPGSHA256 + "\n" + CloudV3PreviousCorrelationSHA256 + "\n"))
	h.Artifacts = append(h.Artifacts, cloudHistoricalArtifact{SHA: CloudV3PreviousAuditSHA256, Release: CloudV3PreviousCode, Attempts: 20})
	for i := 0; i < 20; i++ {
		h.Usage[i].Input, h.Usage[i].Output = intCloud(0), intCloud(0)
	}
	h.Usage[0].Input, h.Usage[0].Output = intCloud(74698), intCloud(2210)
	for i := 45; i < 228; i++ {
		h.Usage[i] = cloudHistoricalUsage{Run: "shadow-" + cloudDigest(i)[:32], Reservation: fmt.Sprintf("shadow-v1-native-%d", i+1), Input: intCloud(0), Output: intCloud(0)}
	}
	h.Usage[45].Input, h.Usage[45].Output = intCloud(62707), intCloud(3083)
	for i := 228; i < 248; i++ {
		h.Usage[i] = cloudHistoricalUsage{Run: "shadow-" + cloudDigest(i)[:32], Reservation: fmt.Sprintf("shadow-v2-native-%d", i+1)}
		if i < 247 {
			h.Usage[i].Input, h.Usage[i].Output = intCloud(0), intCloud(0)
		}
	}
	h.Usage[228].Input, h.Usage[228].Output = intCloud(7141), intCloud(244)
	start := q[0].StartUTC.Add(-time.Second)
	e := cloudV3WindowEnvelope{Status: "passed", Model: CloudRequestedModel, Release: pin.CodeCommit, History: h, Total: 500, New: 252, Maximum: 500, MaximumNew: 252, Resources: FixedCloudV3Resources(), Pin: pin, Attempts: q, Single: single, Multi: multi, StoredQuality: json.RawMessage(`{"PRIVATE_STORED_METRIC":"ignored"}`), StoredComparison: json.RawMessage(`{}`), Start: start, Expires: start.Add(75 * time.Minute)}
	ordinal := 248
	decorate := func(a *CloudAttempt, phase string) {
		ordinal++
		a.AttemptID = fmt.Sprintf("native-%03d", ordinal)
		a.ContextID = "shadow-" + cloudDigest([]string{CloudV3AuditedWindow, phase, a.CaseID, a.Role})[:32]
		hash, err := CloudV3OutputDigest(a.Output)
		if err != nil {
			t.Fatal(err)
		}
		e.Ledger = append(e.Ledger, cloudV3ChargedAttempt{Ordinal: ordinal, Context: a.ContextID, Kind: "start", InputSHA: a.InputSHA256, Created: a.StartUTC.Add(time.Millisecond), Phase: phase, CaseID: a.CaseID, Role: a.Role, Outcome: a.Outcome, InputTokens: a.InputTokens, OutputTokens: a.OutputTokens, TotalTokens: a.TotalTokens, OutputSHA: &hash, ParentContextID: a.ParentContextID})
	}
	for i := range e.Attempts {
		decorate(&e.Attempts[i], "quality")
	}
	base := start.Add(10 * time.Minute)
	for i := range e.Single.Tasks {
		at := base.Add(time.Duration(i) * time.Second)
		s := &e.Single.Tasks[i]
		s.StartUTC, s.EndUTC, s.Milliseconds = at, at.Add(10*time.Millisecond), 10
		s.Attempts[0].StartUTC, s.Attempts[0].EndUTC = s.StartUTC, s.EndUTC
		decorate(&s.Attempts[0], "comparison")
		m := &e.Multi.Tasks[i]
		m.StartUTC, m.EndUTC, m.Milliseconds = at.Add(20*time.Millisecond), at.Add(70*time.Millisecond), 50
		parent := "shadow-" + cloudDigest([]string{CloudV3AuditedWindow, "comparison", m.CaseID, CloudRoleSynthesis})[:32]
		for j := range m.Attempts {
			x := &m.Attempts[j]
			x.StartUTC, x.EndUTC = m.StartUTC.Add(time.Duration(j)*20*time.Millisecond), m.StartUTC.Add(time.Duration(j)*20*time.Millisecond+10*time.Millisecond)
			if j < 2 {
				x.ParentContextID = &parent
			}
			decorate(x, "comparison")
		}
	}
	e.End = e.Multi.Tasks[3].EndUTC.Add(time.Second)
	return c, approval, key, e, CloudV3WindowExpectation{CodeCommit: pin.CodeCommit, SourceSHA256: pin.SourceSHA256, HistorySHA256: h.SHA, HardwareSHA256: pin.HardwareSHA256}
}

func cloudV3WindowBytes(t *testing.T, e cloudV3WindowEnvelope) []byte {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCloudV3WindowAuditCorrelatesNewLedgerAndRetainsImmutableFailedHistory(t *testing.T) {
	c, a, key, e, want := cloudV3WindowFixture(t)
	raw := cloudV3WindowBytes(t, e)
	r, err := AuditCloudV3Window(c, a, key, raw, want)
	if err != nil || r.Status != "CLOUD_V3_WINDOW_AUDIT_PASS" || !r.CorrelationValid || r.NewAttempts != 252 || r.CarryForwardAttempts != 500 || r.HistoricalKnown != 222 || r.HistoricalUnknown != 26 || r.HistoricalTotal != 249727 || r.HistoricalNullTotal != 46 || r.Cost.Attempts != 252 || r.Cost.KnownTotalTokens != 5040 || r.Execution {
		t.Fatal("synthetic audit mismatch", err, r.Status, r.Reasons)
	}
	encoded, _ := json.Marshal(r)
	for _, forbidden := range []string{`"output":`, `"case_id":`, `"documents":`, `"quality_attempts":`, "PRIVATE_STORED_METRIC", `"signature":`, `"golden":`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("private material in safe audit")
		}
	}
	if r.OriginalReportSHA256 != hashWindowBytes(raw) || len(r.Attempts) != 252 || r.Attempts[0].Ordinal != 249 || r.Attempts[251].Ordinal != 500 {
		t.Fatal("spent ordinal or raw artifact identity lost")
	}
}

func TestCloudV3WindowAuditPreservesFailedPartialCostsBeforeComparisonError(t *testing.T) {
	c, a, key, e, want := cloudV3WindowFixture(t)
	e.Status, e.New, e.Total = "failed", 2, 250
	e.Attempts, e.Ledger = e.Attempts[:2], e.Ledger[:2]
	e.Attempts[1].Outcome, e.Attempts[1].Output = "transport_error", json.RawMessage("null")
	e.Attempts[1].ProviderModelVersion, e.Attempts[1].TotalTokens = nil, nil
	e.Ledger[1].Outcome, e.Ledger[1].OutputSHA, e.Ledger[1].TotalTokens = "transport_error", nil, nil
	e.Single.Tasks, e.Multi.Tasks = []CloudComparisonTask{}, []CloudComparisonTask{}
	r, err := AuditCloudV3Window(c, a, key, cloudV3WindowBytes(t, e), want)
	if err != nil || r.Status != "NO_GO" || !r.CorrelationValid || r.ComparisonValid || r.Cost.Attempts != 2 || r.Cost.UnknownTotal != 1 || r.Cost.FullTokenCostKnown || r.CarryForwardAttempts != 250 || r.UnknownVersions != 1 || len(r.Attempts) != 2 {
		t.Fatal("partial failed spend discarded or rescued", err, r)
	}
}

func TestCloudV3WindowAuditDeniesLedgerMutationOutputSubstitutionAndFalseCollectorPass(t *testing.T) {
	c, a, key, original, want := cloudV3WindowFixture(t)
	for _, name := range []string{"ordinal", "reordered_case", "phase", "usage", "output_hash", "expiry", "code", "history", "timeout_unknown_filled", "parent", "single_parent", "alias", "missing_parent_field"} {
		t.Run(name, func(t *testing.T) {
			var e cloudV3WindowEnvelope
			if json.Unmarshal(cloudV3WindowBytes(t, original), &e) != nil {
				t.Fatal("clone")
			}
			switch name {
			case "ordinal":
				e.Ledger[1].Ordinal = e.Ledger[0].Ordinal
			case "reordered_case":
				e.Ledger[0].Ordinal, e.Ledger[1].Ordinal = e.Ledger[1].Ordinal, e.Ledger[0].Ordinal
			case "phase":
				e.Ledger[0].Phase = "comparison"
			case "usage":
				e.Ledger[0].TotalTokens = intCloud(21)
			case "output_hash":
				e.Ledger[0].OutputSHA = stringCloud(strings.Repeat("f", 64))
			case "expiry":
				e.Expires = e.Start.Add(76 * time.Minute)
			case "code":
				e.Release = strings.Repeat("f", 40)
			case "history":
				e.History.Artifacts[15].SHA = strings.Repeat("f", 64)
			case "timeout_unknown_filled":
				e.History.Usage[247].Input, e.History.Usage[247].Output = intCloud(0), intCloud(0)
			case "parent":
				e.Ledger[237].ParentContextID = stringCloud("shadow-" + strings.Repeat("f", 32))
			case "single_parent":
				e.Ledger[0].ParentContextID = e.Ledger[237].ParentContextID
			}
			raw := cloudV3WindowBytes(t, e)
			if name == "alias" {
				raw = []byte(strings.Replace(string(raw), `"phase":`, `"Phase":`, 1))
			} else if name == "missing_parent_field" {
				raw = []byte(strings.Replace(string(raw), `,"parent_context_id":null`, "", 1))
			}
			if r, err := AuditCloudV3Window(c, a, key, raw, want); err == nil || r.Status != "NO_GO" {
				t.Fatal("untrusted journal accepted", name)
			}
		})
	}
	original.Status = "failed"
	r, err := AuditCloudV3Window(c, a, key, cloudV3WindowBytes(t, original), want)
	if err != nil || r.Status != "NO_GO" || r.Cost.Attempts != 252 || !r.QualityValid || !r.ComparisonValid {
		t.Fatal("collector failure replaced by scorer pass", err, r.Status)
	}
}
