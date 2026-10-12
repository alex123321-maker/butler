package acceptance

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// Every history, signature, response and native label here is synthetic. This
// fixture exercises offline admission; it cannot establish native quality.
func cloudV5WindowFixture(t *testing.T, baseline int) (CloudV5Corpus, GoldenApproval, []byte, cloudV5WindowEnvelope, CloudV5WindowExpectation) {
	t.Helper()
	c, approval, key, pin, quality := cloudV5Fixture(t)
	pin.EvidenceKind = "native_cloud_observation"
	pin.WindowID = CloudV5WindowID(baseline)
	pin.Runtime = "go1.27.2/race; sha256:" + strings.Repeat("a", 64)
	single, multi := cloudV5ComparisonFixture(t, c, pin)
	h := cloudHistorical{Status: "mixed_completed_history", SHA: hashWindowBytes([]byte(fmt.Sprintf("synthetic-history-%d", baseline))), Attempts: baseline, Known: baseline - 26, Unknown: 26, Usage: make([]cloudHistoricalUsage, baseline)}
	counts := []int{3, 2, 4, 4, 5, 2, 1, 0, 1, 5, 10, 0, 2, 2, 4, 183, 20, 18, 2, 11, baseline - 279}
	for i, count := range counts {
		h.Artifacts = append(h.Artifacts, cloudHistoricalArtifact{SHA: hashWindowBytes([]byte(fmt.Sprintf("synthetic-history-artifact-%d", i))), Attempts: count})
	}
	for i := range h.Usage {
		h.Usage[i] = cloudHistoricalUsage{Run: "historical-" + cloudDigest(i)[:32], Reservation: fmt.Sprintf("old-%d", i+1)}
		if i < h.Known {
			h.Usage[i].Input, h.Usage[i].Output = intCloud(0), intCloud(0)
		}
	}
	h.Usage[0].Input, h.Usage[0].Output = intCloud(170000), intCloud(6000)
	start := quality[0].StartUTC.Add(-time.Second)
	e := cloudV5WindowEnvelope{Status: "passed", Model: CloudRequestedModel, Release: pin.CodeCommit, History: h, Total: baseline + 252, New: 252, Maximum: baseline + 252, MaximumNew: 252, Resources: FixedCloudV5Resources(), Pin: pin, Attempts: quality, Single: single, Multi: multi, StoredQuality: json.RawMessage(`{"PRIVATE_STORED_METRIC":"ignored"}`), StoredComparison: json.RawMessage(`{}`), Start: start, Expires: start.Add(75 * time.Minute)}
	ordinal := baseline
	decorate := func(a *CloudAttempt, phase string) {
		ordinal++
		a.AttemptID = fmt.Sprintf("native-%03d", ordinal)
		a.ContextID = "shadow-" + cloudDigest([]string{pin.WindowID, phase, a.CaseID, a.Role})[:32]
		hash, err := CloudV5OutputDigest(a.Output)
		if err != nil {
			t.Fatal(err)
		}
		e.Ledger = append(e.Ledger, cloudV5ChargedAttempt{Ordinal: ordinal, Context: a.ContextID, Kind: "start", InputSHA: a.InputSHA256, Created: a.StartUTC.Add(time.Millisecond), Phase: phase, CaseID: a.CaseID, Role: a.Role, Outcome: a.Outcome, InputTokens: a.InputTokens, OutputTokens: a.OutputTokens, TotalTokens: a.TotalTokens, OutputSHA: &hash, ParentContextID: a.ParentContextID})
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
		parent := "shadow-" + cloudDigest([]string{pin.WindowID, "comparison", m.CaseID, CloudRoleSynthesis})[:32]
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
	historyRaw, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	want := CloudV5WindowExpectation{CodeCommit: pin.CodeCommit, SourceSHA256: pin.SourceSHA256, HistorySHA256: h.SHA, HardwareSHA256: pin.HardwareSHA256, HistoricalBaseline: historyRaw, HistoricalKnownTotal: 300123, HistoricalUnknownTotal: 46, CompatibilitySHA256: h.Artifacts[20].SHA}
	return c, approval, key, e, want
}

func cloudV5WindowBytes(t *testing.T, e cloudV5WindowEnvelope) []byte {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCloudV5WindowAuditDerivesActualBaselineAndRetainsTrustedUnknownHistory(t *testing.T) {
	for _, baseline := range []int{280, 283} {
		t.Run(fmt.Sprint(baseline), func(t *testing.T) {
			c, a, key, e, want := cloudV5WindowFixture(t, baseline)
			raw := cloudV5WindowBytes(t, e)
			r, err := AuditCloudV5Window(c, a, key, raw, want)
			if err != nil || r.Status != "CLOUD_V5_WINDOW_AUDIT_PASS" || !r.CorrelationValid || r.HistoricalAttempts != baseline || r.CarryForwardAttempts != baseline+252 || r.MaximumTotal != baseline+252 || r.HistoricalKnown != baseline-26 || r.HistoricalUnknown != 26 || r.HistoricalTotal != 300123 || r.HistoricalNullTotal != 46 || r.HistoricalInput != 170000 || r.HistoricalOutput != 6000 || r.CompatibilitySHA256 != want.CompatibilitySHA256 || r.Cost.Attempts != 252 || r.Cost.KnownTotalTokens != 5040 || r.Execution {
				t.Fatal("synthetic dynamic audit mismatch", err, r.Status, r.Reasons)
			}
			if r.OriginalReportSHA256 != hashWindowBytes(raw) || len(r.Attempts) != 252 || r.Attempts[0].Ordinal != baseline+1 || r.Attempts[251].Ordinal != baseline+252 {
				t.Fatal("actual baseline or spent ordinal discarded")
			}
			encoded, _ := json.Marshal(r)
			for _, forbidden := range []string{`"output":`, `"case_id":`, `"documents":`, `"quality_attempts":`, "PRIVATE_STORED_METRIC", `"signature":`, `"golden":`} {
				if strings.Contains(string(encoded), forbidden) {
					t.Fatal("private content in safe projection", forbidden)
				}
			}
		})
	}
}

func TestCloudV5WindowAuditPreservesFailedChargedPrefixAndNullableSpend(t *testing.T) {
	c, a, key, e, want := cloudV5WindowFixture(t, 283)
	e.Status, e.New, e.Total = "failed", 2, 285
	e.Attempts, e.Ledger = e.Attempts[:2], e.Ledger[:2]
	e.Attempts[0].InputTokens, e.Attempts[0].OutputTokens, e.Attempts[0].TotalTokens = intCloud(0), intCloud(0), intCloud(0)
	e.Ledger[0].InputTokens, e.Ledger[0].OutputTokens, e.Ledger[0].TotalTokens = intCloud(0), intCloud(0), intCloud(0)
	e.Attempts[1].Outcome, e.Attempts[1].Output = "transport_error", json.RawMessage("null")
	e.Attempts[1].ProviderModelVersion, e.Attempts[1].InputTokens, e.Attempts[1].OutputTokens, e.Attempts[1].TotalTokens = nil, nil, nil, nil
	e.Ledger[1].Outcome, e.Ledger[1].OutputSHA, e.Ledger[1].InputTokens, e.Ledger[1].OutputTokens, e.Ledger[1].TotalTokens = "transport_error", nil, nil, nil, nil
	e.Single.Tasks, e.Multi.Tasks = []CloudComparisonTask{}, []CloudComparisonTask{}
	r, err := AuditCloudV5Window(c, a, key, cloudV5WindowBytes(t, e), want)
	if err != nil || r.Status != "NO_GO" || !r.CorrelationValid || r.ComparisonValid || r.Cost.Attempts != 2 || r.Cost.UnknownTotal != 1 || r.Cost.KnownTotalTokens != 0 || r.Cost.FullTokenCostKnown || r.CarryForwardAttempts != 285 || r.UnknownVersions != 1 || len(r.Attempts) != 2 || r.HistoricalUnknown != 26 || r.HistoricalNullTotal != 46 {
		t.Fatal("failed prefix spend or NULL history discarded", err, r)
	}
}

func TestCloudV5WindowAuditDeniesTrustedHistoryAndChargedIdentityMutations(t *testing.T) {
	c, a, key, original, want := cloudV5WindowFixture(t, 283)
	for _, name := range []string{"old_usage", "old_null_filled", "old_order", "old_omission", "history_baseline", "history_artifact", "compatibility_pin", "compatibility_last_artifact", "unknown_total", "ordinal", "charged_order", "case", "phase", "usage", "input", "output", "parent", "single_parent", "omitted_attempt", "omitted_charge", "alias", "missing_parent_field"} {
		t.Run(name, func(t *testing.T) {
			var e cloudV5WindowEnvelope
			if json.Unmarshal(cloudV5WindowBytes(t, original), &e) != nil {
				t.Fatal("fixture clone")
			}
			expected := want
			switch name {
			case "old_usage":
				e.History.Usage[0].Input = intCloud(1)
			case "old_null_filled":
				e.History.Usage[282].Input, e.History.Usage[282].Output = intCloud(0), intCloud(0)
			case "old_order":
				e.History.Usage[0], e.History.Usage[1] = e.History.Usage[1], e.History.Usage[0]
			case "old_omission":
				e.History.Usage = e.History.Usage[:282]
			case "history_baseline":
				e.History.Attempts = 280
			case "history_artifact":
				e.History.Artifacts[19].SHA = strings.Repeat("f", 64)
			case "compatibility_pin":
				expected.CompatibilitySHA256 = strings.Repeat("f", 64)
			case "compatibility_last_artifact":
				e.History.Artifacts[20].SHA = strings.Repeat("f", 64)
				expected.HistoricalBaseline, _ = json.Marshal(e.History)
			case "unknown_total":
				expected.HistoricalUnknownTotal = 0
			case "ordinal":
				e.Ledger[1].Ordinal = e.Ledger[0].Ordinal
			case "charged_order":
				e.Ledger[0], e.Ledger[1] = e.Ledger[1], e.Ledger[0]
			case "case":
				e.Ledger[0].CaseID = e.Ledger[1].CaseID
			case "phase":
				e.Ledger[0].Phase = "comparison"
			case "usage":
				e.Ledger[0].TotalTokens = intCloud(21)
			case "input":
				e.Ledger[0].InputSHA = strings.Repeat("f", 64)
			case "output":
				e.Ledger[0].OutputSHA = stringCloud(strings.Repeat("f", 64))
			case "parent":
				e.Ledger[237].ParentContextID = stringCloud("shadow-" + strings.Repeat("f", 32))
			case "single_parent":
				e.Ledger[0].ParentContextID = e.Ledger[237].ParentContextID
			case "omitted_attempt":
				e.Attempts = e.Attempts[1:]
			case "omitted_charge":
				e.Ledger = e.Ledger[1:]
			}
			raw := cloudV5WindowBytes(t, e)
			if name == "alias" {
				raw = []byte(strings.Replace(string(raw), `"phase":`, `"Phase":`, 1))
			} else if name == "missing_parent_field" {
				raw = []byte(strings.Replace(string(raw), `,"parent_context_id":null`, "", 1))
			}
			if r, err := AuditCloudV5Window(c, a, key, raw, expected); err == nil || r.Status != "NO_GO" {
				t.Fatal("history/charged mutation accepted", name)
			}
		})
	}
}

func cloudV5PGFixture(e cloudV5WindowEnvelope) CloudV5PGWitness {
	w := CloudV5PGWitness{Version: CloudV5PGWitnessVersion, Window: CloudV5PGWindow{Baseline: e.History.Attempts, Attempts: e.Total, BaselineHash: e.History.SHA, PolicyHash: e.Pin.PolicySHA256, Expires: e.Expires}, Attempts: []CloudV5PGAttempt{}}
	for _, a := range e.Ledger {
		w.Attempts = append(w.Attempts, CloudV5PGAttempt{a.Ordinal, a.Phase, a.CaseID, a.Role, a.Context, a.InputSHA, a.Created, a.Outcome, a.InputTokens, a.OutputTokens, a.TotalTokens, a.OutputSHA, a.ParentContextID})
	}
	return w
}

func TestCloudV5PGCorrelationPreservesActualBaselineFailedPrefixAndLiteralZero(t *testing.T) {
	_, _, _, e, _ := cloudV5WindowFixture(t, 280)
	e.Status, e.New, e.Total = "failed", 2, 282
	e.Attempts, e.Ledger = e.Attempts[:2], e.Ledger[:2]
	e.Single.Tasks, e.Multi.Tasks = []CloudComparisonTask{}, []CloudComparisonTask{}
	e.Ledger[0].InputTokens, e.Ledger[0].OutputTokens, e.Ledger[0].TotalTokens = intCloud(0), intCloud(0), intCloud(0)
	e.Ledger[1].Outcome, e.Ledger[1].OutputSHA = "unavailable", nil
	e.Ledger[1].InputTokens, e.Ledger[1].OutputTokens, e.Ledger[1].TotalTokens = nil, nil, nil
	w := cloudV5PGFixture(e)
	witness, _ := json.Marshal(w)
	r, err := CorrelateCloudV5PGWitness(cloudV5WindowBytes(t, e), witness)
	if err != nil || !r.Matched || r.TotalAttempts != 282 || r.NewAttempts != 2 || r.Rows != 2 || r.QualityPassed || r.InferenceProven || r.Execution {
		t.Fatal("dynamic partial equality failed", err, r)
	}
	w.Attempts[1].TotalTokens = intCloud(0)
	witness, _ = json.Marshal(w)
	r, err = CorrelateCloudV5PGWitness(cloudV5WindowBytes(t, e), witness)
	if err != nil || r.Matched || r.TotalAttempts != 282 {
		t.Fatal("NULL journal spend replaced by zero", err, r)
	}
}

func TestCloudV5PGCorrelationDeniesBaselineOrdinalLineageAndJournalMutations(t *testing.T) {
	_, _, _, e, _ := cloudV5WindowFixture(t, 283)
	for _, name := range []string{"baseline_unset", "baseline_old", "baseline_other", "ordinal", "counter", "expiry", "history", "policy", "role", "phase", "case", "context", "parent", "single_parent", "input", "output", "outcome", "usage", "missing", "reorder", "alias", "duplicate", "missing_baseline", "missing_parent_field"} {
		t.Run(name, func(t *testing.T) {
			w := cloudV5PGFixture(e)
			switch name {
			case "baseline_unset":
				w.Window.Baseline = 0
			case "baseline_old":
				w.Window.Baseline = 279
			case "baseline_other":
				w.Window.Baseline = 280
			case "ordinal":
				w.Attempts[0].Ordinal = 281
			case "counter":
				w.Window.Attempts--
			case "expiry":
				w.Window.Expires = w.Window.Expires.Add(1)
			case "history":
				w.Window.BaselineHash = strings.Repeat("f", 64)
			case "policy":
				w.Window.PolicyHash = strings.Repeat("f", 64)
			case "role":
				w.Attempts[236].Role = CloudRoleChildB
			case "phase":
				w.Attempts[0].Phase = "comparison"
			case "case":
				w.Attempts[0].CaseID = "foreign"
			case "context":
				w.Attempts[0].Context = "shadow-" + strings.Repeat("f", 32)
			case "parent":
				w.Attempts[237].ParentContextID = stringCloud("shadow-" + strings.Repeat("f", 32))
			case "single_parent":
				w.Attempts[0].ParentContextID = w.Attempts[237].ParentContextID
			case "input":
				w.Attempts[0].InputHash = strings.Repeat("f", 64)
			case "output":
				w.Attempts[0].OutputHash = stringCloud(strings.Repeat("f", 64))
			case "outcome":
				w.Attempts[0].Outcome, w.Attempts[0].OutputHash = "transport_error", nil
			case "usage":
				w.Attempts[0].TotalTokens = nil
			case "missing":
				w.Attempts = w.Attempts[:len(w.Attempts)-1]
			case "reorder":
				w.Attempts[0], w.Attempts[1] = w.Attempts[1], w.Attempts[0]
			}
			b, _ := json.Marshal(w)
			if name == "alias" {
				b = []byte(strings.Replace(string(b), `"phase":`, `"Phase":`, 1))
			} else if name == "duplicate" {
				b = []byte(strings.Replace(string(b), `"ordinal":284`, `"ordinal":284,"\u006frdinal":285`, 1))
			} else if name == "missing_baseline" {
				b = []byte(strings.Replace(string(b), `"native_baseline":283,`, "", 1))
			} else if name == "missing_parent_field" {
				b = []byte(strings.Replace(string(b), `,"parent_context_id":null`, "", 1))
			}
			r, err := CorrelateCloudV5PGWitness(cloudV5WindowBytes(t, e), b)
			if err == nil && r.Matched || r.QualityPassed || r.InferenceProven || r.Execution {
				t.Fatal("journal mutation accepted", name)
			}
		})
	}
}
