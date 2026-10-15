package acceptance

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/butler/butler/internal/providerauth"
)

type memoryAuditAuthProbe struct {
	Passed       bool                                 `json:"passed"`
	Milliseconds int64                                `json:"milliseconds"`
	Events       []providerauth.AntigravityDiagnostic `json:"events"`
	Expense      MemoryExpense                        `json:"expense"`
}

func auditMemoryRenewedAuth(pins MemoryRuntimePins, f MemoryFrozen, m MemoryMeasurements, directory string, expires time.Time) error {
	deny := errors.New("new auth original/ledger/expense correlation differs")
	load := func(name string, out any) ([]byte, error) {
		raw, err := ReadPrivateFile(filepath.Join(directory, name), 32<<20)
		if err != nil {
			return nil, err
		}
		return raw, json.Unmarshal(raw, out)
	}
	var receipt struct {
		Started    time.Time              `json:"started_utc"`
		Finished   time.Time              `json:"finished_utc"`
		ModelCalls int                    `json:"model_calls"`
		Probes     []memoryAuditAuthProbe `json:"probes"`
	}
	issued, _ := time.Parse(time.RFC3339Nano, pins.Renewal.AuthorizedUTC)
	if _, err := load("auth-preflight.json", &receipt); err != nil || receipt.ModelCalls != 0 || len(receipt.Probes) < 1 || len(receipt.Probes) > 3 || !receipt.Probes[len(receipt.Probes)-1].Passed || receipt.Started.Before(issued) || receipt.Finished.Before(receipt.Started) || receipt.Finished.After(expires) || receipt.Finished.Sub(receipt.Started) > 75*time.Second {
		return deny
	}
	var authLedger struct {
		Admissions []struct {
			ID      string    `json:"id"`
			Window  string    `json:"window_id"`
			Mask    string    `json:"input_mask"`
			Started time.Time `json:"started_at"`
		} `json:"admissions"`
		Observations []struct {
			ID     string               `json:"id"`
			Record memoryAuditAuthProbe `json:"record"`
		} `json:"observations"`
	}
	if _, err := load("auth-ledger.json", &authLedger); err != nil || len(authLedger.Admissions) != len(receipt.Probes) || len(authLedger.Observations) != len(receipt.Probes) {
		return deny
	}
	for n, probe := range receipt.Probes {
		id := pins.WindowID + "/auth-probe/" + strconv.Itoa(n+1)
		a, o := authLedger.Admissions[n], authLedger.Observations[n]
		var original memoryAuditAuthProbe
		if _, err := load("auth-probe-"+strconv.Itoa(n+1)+".json", &original); err != nil || MemoryDigest(original) != MemoryDigest(probe) || o.ID != id || MemoryDigest(o.Record) != MemoryDigest(probe) || a.ID != id || a.Window != pins.WindowID || a.Mask != pins.Renewal.InputsSHA || a.Started.Before(receipt.Started) || a.Started.After(receipt.Finished) {
			return deny
		}
		e := probe.Expense
		outcome := "failed"
		if probe.Passed {
			outcome = "success"
		}
		if e.ID != id || e.Stage != "background" || e.Calls != 1 || e.Outcome != outcome || e.EvidenceSHA256 != MemoryDigest(probe.Events) || e.Microunits != nil || e.Milliseconds == nil || *e.Milliseconds != probe.Milliseconds {
			return deny
		}
		matched := 0
		for _, candidate := range m.Expenses {
			if candidate.ID == id {
				if MemoryDigest(candidate) != MemoryDigest(e) {
					return deny
				}
				matched++
			}
		}
		if matched != 1 {
			return deny
		}
		confirmed := false
		for _, d := range probe.Events {
			if d.Stage == "paid_discovery" && d.Outcome == "completed" && d.HTTPStatus == 200 {
				confirmed = true
			}
		}
		if probe.Passed && !confirmed {
			return deny
		}
		if !providerauth.ValidAntigravityDiagnostics(probe.Events) || providerauth.UnsafeAntigravityRefresh(probe.Events) {
			return deny
		}
	}
	var ledger map[string]json.RawMessage
	if _, err := load("ledger.json", &ledger); err != nil {
		return deny
	}
	var traces []struct {
		Run   string `json:"run_id"`
		Input string `json:"input_hash"`
		SHA   string `json:"trace_hash"`
	}
	if json.Unmarshal(ledger["fixture_memory_eval_auth"], &traces) != nil || len(traces) != 5 {
		return deny
	}
	used := map[string]bool{}
	for _, i := range f.Inputs {
		var trace struct {
			Run    string                               `json:"run_id"`
			Input  string                               `json:"input_sha256"`
			Events []providerauth.AntigravityDiagnostic `json:"diagnostics"`
		}
		raw, err := load("auth-"+strings.TrimPrefix(i.CaseID, "memory:")+"-"+i.Arm+".json", &trace)
		run := "memory-run-" + MemoryDigest([]string{f.Plan.WindowID, i.CaseID, i.Arm})[:32]
		if err != nil || trace.Run != run || trace.Input != MemoryInputDigest(i) || (!providerauth.ValidAntigravityDiagnostics(trace.Events) || providerauth.UnsafeAntigravityRefresh(trace.Events)) {
			return deny
		}
		matched := 0
		for _, r := range traces {
			if r.Run == run {
				if used[run] || r.Input != trace.Input || r.SHA != hashWindowBytes(raw) {
					return deny
				}
				used[run] = true
				matched++
			}
		}
		if matched != 1 {
			return deny
		}
	}
	return nil
}
