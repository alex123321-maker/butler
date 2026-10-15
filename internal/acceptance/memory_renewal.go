package acceptance

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

// MemoryRenewal is a new, prospective operator mandate for exactly the five
// never-admitted inputs. Historical deadlines, admissions and expenses stay sealed.
type MemoryRenewal struct {
	AuthorizedUTC  string `json:"authorized_utc"`
	ExpiresUTC     string `json:"expires_utc"`
	PriorSHA       string `json:"prior_sha256"`
	InputsSHA      string `json:"inputs_sha256"`
	WindowID       string `json:"window_id"`
	SourceSHA      string `json:"source_sha256"`
	BinarySHA      string `json:"binary_sha256"`
	MaxNativeCalls int64  `json:"maximum_new_native_calls"`
	MaxUnits       int64  `json:"maximum_new_conservative_units"`
}

func ValidateMemoryRenewal(p MemoryPrior, pins MemoryRuntimePins, f MemoryFrozen, now time.Time) (time.Time, error) {
	r := pins.Renewal
	if f.Plan.SourceSHA256 != pins.Source || f.Plan.CodeCommit != pins.Release || f.Plan.HardwareSHA256 != pins.Hardware {
		return time.Time{}, errors.New("renewed plan/runtime identity differs")
	}
	deny := errors.New("new five-input mandate differs or is expired")
	if r == nil || pins.Prior == nil || r.PriorSHA != MemoryDigest(*pins.Prior) || r.WindowID != pins.WindowID || r.WindowID != f.Plan.WindowID || r.SourceSHA != pins.Source || r.BinarySHA != pins.Binary || !validSHA(r.SourceSHA) || !validSHA(r.BinarySHA) || r.InputsSHA != MemoryDigest(f.Inputs) || r.MaxNativeCalls != 5 || pins.Prior.Calls != p.Audit.NativeCalls+p.Audit.AuxiliaryCalls || pins.Prior.ReservedUnits <= 0 || pins.Prior.ReservedUnits+r.MaxUnits > 4000000 || len(p.PaidInputs) != 59 || len(p.Measurements.Observations) != 67 || len(f.Inputs) != 5 {
		return time.Time{}, deny
	}
	want, err := MemoryPendingSchedule(p, f.Plan)
	if err != nil || MemoryDigest(want) != MemoryDigest(f) {
		return time.Time{}, deny
	}
	var units int64
	for _, i := range f.Inputs {
		if i.SkipReason != "" {
			return time.Time{}, deny
		}
		units += MemoryNativeReservedUnits(i)
	}
	issued, e1 := time.Parse(time.RFC3339Nano, r.AuthorizedUTC)
	expires, e2 := time.Parse(time.RFC3339Nano, r.ExpiresUTC)
	if e1 != nil || e2 != nil || units != r.MaxUnits || !expires.After(issued) || expires.Sub(issued) > 30*time.Minute || !issued.After(p.Expires) || (!now.IsZero() && (now.Before(issued) || !now.Before(expires))) {
		return time.Time{}, deny
	}
	return expires, nil
}

// Complete attempt matrix may contain clean retained provider failures. Missing
// observations, resampling and guard/retention failures remain hard failures.
func AuditMemoryRenewedContinuation(s MemorySuite, p MemoryPrior, f MemoryFrozen, m MemoryMeasurements, directory, binary string, pins MemoryRuntimePins) (MemoryRunnerAudit, error) {
	expires, err := ValidateMemoryRenewal(p, pins, f, time.Time{})
	if err != nil {
		return MemoryRunnerAudit{}, err
	}
	if len(m.Observations) != len(f.Inputs) {
		return MemoryRunnerAudit{}, errors.New("renewed attempt matrix incomplete")
	}
	completed := 0
	for n, o := range m.Observations {
		i := f.Inputs[n]
		if o.CaseID != i.CaseID || o.Arm != i.Arm || o.InputSHA256 != MemoryInputDigest(i) || (o.Status != "completed" && o.Status != "failed") {
			return MemoryRunnerAudit{}, errors.New("renewed attempt order/input differs")
		}
		if o.Status == "completed" {
			completed++
		} else {
			raw, err := ReadPrivateFile(filepath.Join(directory, "events-"+strings.TrimPrefix(i.CaseID, "memory:")+"-"+i.Arm+".json"), 1<<20)
			var events []struct {
				Type string `json:"type"`
				Code string `json:"error_code"`
			}
			if err != nil || json.Unmarshal(raw, &events) != nil || len(events) != 2 || events[0].Type != "run_started" || events[1].Type != "transport_error" || !MemoryAvailabilityCode(events[1].Code) {
				return MemoryRunnerAudit{}, errors.New("renewed failure is not a clean availability observation")
			}
		}
	}
	a, err := auditMemoryRunner(s, f, m, directory, binary, true, pins.Prior.Calls, pins.Prior.ReservedUnits)
	if err != nil {
		return a, err
	}
	if a.NativeCalls != 5 || a.AuxiliaryCalls != 0 || a.QuestionResults != completed {
		return a, errors.New("renewed admissions or results differ")
	}
	window := p // Only the new window is checked against its own mandate.
	window.Expires = expires
	if err = memoryContinuationWindow(window, directory, pins.Prior.ReservedUnits+pins.Renewal.MaxUnits); err != nil {
		return a, err
	}
	if err = auditMemoryRenewedAuth(pins, f, m, directory, expires); err != nil {
		return a, err
	}
	a.NativeCalls += p.Audit.NativeCalls
	a.AuxiliaryCalls += p.Audit.AuxiliaryCalls
	a.QuestionResults += p.Audit.QuestionResults
	a.UnknownBroaderTaskResults += p.Audit.UnknownBroaderTaskResults
	a.Status = "PASS_CORRELATED_COMPLETE_ATTEMPT_MATRIX_WITH_RETAINED_FAILURES"
	a.Limits = append(a.Limits, "New owner-authorized five-input window has its own deadline; all old expiries, paid failures and expenses remain unchanged.", "Clean provider failures remain eligible in the original denominator; unavailable usage and invoice remain unknown.", "Reused retrieval pipeline latency and broader user task effects remain unknown.")
	return a, nil
}

func MemoryAvailabilityCode(code string) bool {
	switch code {
	case "provider_unavailable", "provider_timeout", "rate_limited", "transport_connection_error":
		return true
	}
	return false
}
