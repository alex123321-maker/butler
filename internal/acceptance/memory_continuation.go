package acceptance

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"time"
)

// Operator-owned pins describe the retained first failed native entry and its
// already measured retrieval. They are not model input or authority to retry.
type MemoryPriorPins struct {
	LedgerSHA       string `json:"ledger_sha256"`
	FrozenFileSHA   string `json:"frozen_file_sha256"`
	MeasurementsSHA string `json:"measurements_sha256"`
	SourceSHA       string `json:"source_sha256"`
	BinarySHA       string `json:"binary_sha256"`
	CodeCommit      string `json:"code_commit"`
	Calls           int64  `json:"calls"`
	ReservedUnits   int64  `json:"reserved_units"`
	ExpiresUTC      string `json:"expires_utc"`
}

type MemoryPrior struct {
	Frozen       MemoryFrozen
	Measurements MemoryMeasurements
	Audit        MemoryRunnerAudit
	Expires      time.Time
	PaidInput    MemoryInput
}

func MemoryFrozenDigest(f MemoryFrozen) string {
	return MemoryDigest(struct {
		Plan     MemoryPlan
		Inputs   []MemoryInput
		Captures []MemoryCapture
	}{f.Plan, f.Inputs, f.Captures})
}

// The wire framing reservation is deliberately stricter than policy tokenizer
// estimates. Both native admission and the offline audit use this same bound.
func MemoryNativeReservedUnits(i MemoryInput) int64 {
	user, _ := MemoryModelProjection(i)
	serialized, _ := json.Marshal(struct {
		System, User string
		Schema       json.RawMessage
	}{i.Instructions, string(user), MemoryNativeSchema(i)})
	return min(int64(32768), int64(len(serialized))+8192) + 32768
}

func LoadMemoryPrior(s MemorySuite, directory string, pins MemoryPriorPins) (MemoryPrior, error) {
	var p MemoryPrior
	read := func(name, hash string, out any) error {
		raw, err := ReadPrivateFile(filepath.Join(directory, name), 32<<20)
		if err != nil || !validSHA(hash) || hashWindowBytes(raw) != hash {
			return errors.New("prior original bytes differ from operator pin")
		}
		return json.Unmarshal(raw, out)
	}
	var ledger map[string]json.RawMessage
	if err := read("ledger.json", pins.LedgerSHA, &ledger); err != nil {
		return p, err
	}
	if err := read("frozen-before-native.json", pins.FrozenFileSHA, &p.Frozen); err != nil {
		return p, err
	}
	if err := read("measurements.json", pins.MeasurementsSHA, &p.Measurements); err != nil {
		return p, err
	}
	canonical, err := FreezeMemory(s, p.Frozen.Plan, p.Frozen.Captures)
	if err != nil || canonical.SHA256 != p.Frozen.SHA256 || MemoryDigest(canonical) != MemoryDigest(p.Frozen) || p.Frozen.Plan.SourceSHA256 != pins.SourceSHA || p.Frozen.Plan.CodeCommit != pins.CodeCommit || len(p.Measurements.Observations) != 1 || len(p.Frozen.Captures) != 22 {
		return p, errors.New("prior full comparison freeze differs")
	}
	var windows []struct {
		Calls   int64     `json:"calls"`
		Units   int64     `json:"reserved_units"`
		Expires time.Time `json:"expires_at"`
	}
	p.Expires, err = time.Parse(time.RFC3339Nano, pins.ExpiresUTC)
	if err != nil || json.Unmarshal(ledger["fixture_memory_eval_window"], &windows) != nil || len(windows) != 1 || windows[0].Calls != pins.Calls || windows[0].Units != pins.ReservedUnits || !windows[0].Expires.Equal(p.Expires) {
		return p, errors.New("prior charge or absolute expiry differs")
	}
	eligible := 0
	for _, i := range p.Frozen.Inputs {
		if i.SkipReason == "" {
			if eligible == 0 {
				p.PaidInput = i
			}
			eligible++
		}
	}
	o := p.Measurements.Observations[0]
	if eligible != 64 || o.CaseID != p.PaidInput.CaseID || o.Arm != p.PaidInput.Arm || o.InputSHA256 != MemoryInputDigest(p.PaidInput) || o.Status != "failed" {
		return p, errors.New("only the retained first failed entry may be carried")
	}
	p.Audit, err = AuditMemoryRunnerPrefix(s, p.Frozen, p.Measurements, directory, pins.BinarySHA)
	if err != nil || p.Audit.NativeCalls != 1 || p.Audit.AuxiliaryCalls != 53 || pins.Calls != 54 || pins.ReservedUnits <= 0 {
		return p, errors.New("prior charged prefix cannot be correlated")
	}
	return p, nil
}

// The execution schedule omits a spent entry. The scored original72-input
// matrix remains intact; the failed entry is never relabelled as a planned skip.
func MemoryPendingSchedule(p MemoryPrior, plan MemoryPlan) (MemoryFrozen, error) {
	f := MemoryFrozen{Plan: plan, Captures: append([]MemoryCapture(nil), p.Frozen.Captures...)}
	old := p.Frozen.Plan
	if plan.WindowID == old.WindowID || plan.Model != old.Model || plan.ModelVersion != old.ModelVersion || plan.Runtime != old.Runtime || plan.MemoryConfigSHA256 != old.MemoryConfigSHA256 || plan.PricingBasis != old.PricingBasis || plan.SuiteSHA256 != old.SuiteSHA256 || plan.CriteriaSHA256 != old.CriteriaSHA256 || plan.Split != old.Split {
		return f, errors.New("pending schedule cannot change the experiment")
	}
	for _, i := range p.Frozen.Inputs {
		if i.CaseID == p.PaidInput.CaseID && i.Arm == p.PaidInput.Arm {
			continue
		}
		f.Inputs = append(f.Inputs, i)
	}
	f.SHA256 = MemoryFrozenDigest(f)
	return f, nil
}

func CombineMemoryMeasurements(p MemoryPrior, current MemoryMeasurements) (MemoryMeasurements, error) {
	m := MemoryMeasurements{FrozenSHA256: p.Frozen.SHA256, Observations: append([]MemoryObservation(nil), p.Measurements.Observations...), Expenses: append([]MemoryExpense(nil), p.Measurements.Expenses...)}
	ids := map[string]bool{}
	for _, e := range m.Expenses {
		ids[e.ID] = true
	}
	for _, e := range current.Expenses {
		if ids[e.ID] {
			return m, errors.New("carried expense cannot be replaced or duplicated")
		}
		ids[e.ID] = true
		m.Expenses = append(m.Expenses, e)
	}
	for _, o := range current.Observations {
		if o.CaseID == p.PaidInput.CaseID && o.Arm == p.PaidInput.Arm {
			return m, errors.New("spent observation was retried")
		}
		m.Observations = append(m.Observations, o)
	}
	return m, nil
}

func AuditMemoryContinuation(s MemorySuite, p MemoryPrior, currentFrozen MemoryFrozen, current MemoryMeasurements, directory, binarySHA string, pins MemoryPriorPins) (MemoryRunnerAudit, error) {
	expected, err := MemoryPendingSchedule(p, currentFrozen.Plan)
	if err != nil || MemoryDigest(expected) != MemoryDigest(currentFrozen) {
		return MemoryRunnerAudit{}, errors.New("pending input matrix changed or spent slot resampled")
	}
	a, err := auditMemoryRunner(s, currentFrozen, current, directory, binarySHA, false, pins.Calls, pins.ReservedUnits)
	if err != nil {
		return a, err
	}
	if a.NativeCalls != 63 || a.AuxiliaryCalls != 0 || a.QuestionResults != 63 {
		return a, errors.New("pending matrix incomplete or auxiliary inference repeated")
	}
	var ledger map[string]json.RawMessage
	raw, err := ReadPrivateFile(filepath.Join(directory, "ledger.json"), 32<<20)
	if err != nil || json.Unmarshal(raw, &ledger) != nil {
		return a, errors.New("pending ledger absent")
	}
	var windows []struct {
		Expires time.Time `json:"expires_at"`
		Units   int64     `json:"reserved_units"`
	}
	expectedUnits := pins.ReservedUnits
	for _, i := range currentFrozen.Inputs {
		if i.SkipReason == "" {
			expectedUnits += MemoryNativeReservedUnits(i)
		}
	}
	if json.Unmarshal(ledger["fixture_memory_eval_window"], &windows) != nil || len(windows) != 1 || !windows[0].Expires.Equal(p.Expires) || windows[0].Units != expectedUnits {
		return a, errors.New("original absolute deadline or accumulated conservative charge differs")
	}
	a.Status = "PASS_CORRELATED_ATTEMPT_MATRIX_WITH_RETAINED_FAILURE"
	a.NativeCalls += p.Audit.NativeCalls
	a.AuxiliaryCalls += p.Audit.AuxiliaryCalls
	a.UnknownBroaderTaskResults += p.Audit.UnknownBroaderTaskResults
	a.Limits = append(a.Limits, "The original first eligible native entry failed with unknown usage/cause; it remains in the scored denominator.", "22 retrieval captures and53 auxiliary calls are original observations, reused without inference; current catalog authority was rebased separately.", "Full original retrieval pipeline latency was not retained and remains unknown for reused captures.")
	return a, nil
}
