package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport"
)

func memoryRenewedFiveFixture(t *testing.T, p acceptance.MemoryPrior, pins acceptance.MemoryPriorPins, directory string) {
	ctx := context.Background()
	seal := func(c *memoryEvalCollector, prior acceptance.MemoryPrior, parent acceptance.MemoryPriorPins, parentDir string) acceptance.MemoryPriorPins {
		c.measurements.FrozenSHA256 = c.frozen.SHA256
		combined, err := acceptance.CombineMemoryMeasurements(prior, c.measurements)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range []struct {
			name  string
			value any
		}{{"frozen-before-native.json", c.frozen}, {"measurements.json", c.measurements}, {"comparison-frozen.json", prior.Frozen}, {"comparison-measurements.json", combined}} {
			if err = c.writeEvidence(x.name, x.value); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = c.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET closed=TRUE`); err != nil {
			t.Fatal(err)
		}
		if err = c.exportLedger(ctx); err != nil {
			t.Fatal(err)
		}
		result := acceptance.MemoryPriorPins{Parent: &parent, SourceSHA: c.plan.SourceSHA256, CodeCommit: c.plan.CodeCommit, BinarySHA: strings.Repeat("5", 64), ExpiresUTC: parent.ExpiresUTC}
		if err = c.pool.QueryRow(ctx, `SELECT calls,reserved_units FROM fixture_memory_eval_window`).Scan(&result.Calls, &result.ReservedUnits); err != nil {
			t.Fatal(err)
		}
		for _, x := range []struct {
			name string
			out  *string
		}{{"ledger.json", &result.LedgerSHA}, {"frozen-before-native.json", &result.FrozenFileSHA}, {"measurements.json", &result.MeasurementsSHA}} {
			*x.out, err = acceptance.FileHash(filepath.Join(c.private, x.name))
			if err != nil {
				t.Fatal(err)
			}
		}
		copyMemoryFixtureTree(t, parentDir, filepath.Join(c.private, "prior"))
		return result
	}
	cseFor := func(c *memoryEvalCollector, i acceptance.MemoryInput) acceptance.MemoryCase {
		for _, cse := range c.suite.Cases {
			if acceptance.MemoryOpaqueCaseID(cse) == i.CaseID {
				return cse
			}
		}
		t.Fatal("case absent")
		return acceptance.MemoryCase{}
	}
	prepare := func(c *memoryEvalCollector, prior acceptance.MemoryPrior) {
		for _, cse := range c.suite.Cases {
			if err := c.capture(ctx, cse); err != nil {
				t.Fatal(err)
			}
		}
		var err error
		c.frozen, err = acceptance.MemoryPendingSchedule(prior, c.plan)
		if err != nil {
			t.Fatal(err)
		}
	}
	fifth := memoryCollectorFixtureWithPrior(t, &p, &pins, directory)
	prepare(fifth, p)
	fifth.factory = func(json.RawMessage) (transport.ModelProvider, error) {
		return &memoryFailureEventProvider{failureType: transport.ErrorTypeProviderUnavailable, preHTTP: true}, nil
	}
	for _, i := range fifth.frozen.Inputs {
		o, err := fifth.sample(ctx, cseFor(fifth, i), i)
		fifth.measurements.Observations = append(fifth.measurements.Observations, o)
		if e := fifth.fillUnused(ctx, i); e != nil {
			t.Fatal(e)
		}
		if i.SkipReason == "" {
			if err == nil {
				t.Fatal("expected clean retained failure")
			}
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	pins5 := seal(fifth, p, pins, directory)
	p5, err := acceptance.LoadMemoryPrior(fifth.suite, fifth.private, pins5)
	if err != nil || len(p5.PaidInputs) != 59 || len(p5.Measurements.Observations) != 67 {
		t.Fatal("five old prefixes", err)
	}
	issued := p5.Expires.Add(time.Second)
	r := acceptance.MemoryRenewal{AuthorizedUTC: issued.Format(time.RFC3339Nano), ExpiresUTC: issued.Add(30 * time.Minute).Format(time.RFC3339Nano), PriorSHA: acceptance.MemoryDigest(pins5), WindowID: "fixture-renewed-five-components", SourceSHA: strings.Repeat("2", 64), BinarySHA: strings.Repeat("5", 64), MaxNativeCalls: 5}
	renewed := memoryCollectorFixtureWithRenewal(t, &p5, &pins5, fifth.private, &r)
	prepare(renewed, p5)
	r.InputsSHA = acceptance.MemoryDigest(renewed.frozen.Inputs)
	for _, i := range renewed.frozen.Inputs {
		r.MaxUnits += acceptance.MemoryNativeReservedUnits(i)
	}
	runtime := acceptance.MemoryRuntimePins{Prior: &pins5, Renewal: &r, WindowID: r.WindowID, Source: r.SourceSHA, Binary: r.BinarySHA, Release: renewed.plan.CodeCommit, Hardware: renewed.plan.HardwareSHA256}
	if _, err = acceptance.ValidateMemoryRenewal(p5, runtime, renewed.frozen, issued); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"source", "commit", "hardware"} {
		changed := renewed.frozen
		switch field {
		case "source":
			changed.Plan.SourceSHA256 = strings.Repeat("0", 64)
		case "commit":
			changed.Plan.CodeCommit = strings.Repeat("0", 40)
		case "hardware":
			changed.Plan.HardwareSHA256 = strings.Repeat("0", 64)
		}
		changed.SHA256 = acceptance.MemoryFrozenDigest(changed)
		if _, err = acceptance.ValidateMemoryRenewal(p5, runtime, changed, issued); err == nil {
			t.Fatal("forged renewed plan accepted", field)
		}
	}
	for _, kind := range []string{"expiry", "before_issue", "sixth_call", "mask", "parent", "carry", "binary"} {
		bad, pr, rr := runtime, pins5, r
		bad.Prior = &pr
		bad.Renewal = &rr
		now := issued
		switch kind {
		case "expiry":
			now = issued.Add(30 * time.Minute)
		case "before_issue":
			now = issued.Add(-time.Nanosecond)
		case "sixth_call":
			rr.MaxNativeCalls = 6
		case "mask":
			rr.InputsSHA = strings.Repeat("0", 64)
		case "parent":
			rr.PriorSHA = strings.Repeat("0", 64)
		case "carry":
			pr.Calls--
		case "binary":
			rr.BinarySHA = strings.Repeat("0", 64)
		}
		if _, err = acceptance.ValidateMemoryRenewal(p5, bad, renewed.frozen, now); err == nil {
			t.Fatal("invalid renewed mandate accepted", kind)
		}
	}
	if err = renewed.writeEvidence("frozen-before-native.json", renewed.frozen); err != nil {
		t.Fatal(err)
	}

	ms := int64(1)
	probe := memoryAuthProbe{Passed: true, Milliseconds: 1, Events: []providerauth.AntigravityDiagnostic{{Stage: "paid_discovery", Outcome: "completed", HTTPStatus: 200}}}
	probe.Expense = acceptance.MemoryExpense{ID: renewed.plan.WindowID + "/auth-probe/1", CaseID: renewed.frozen.Inputs[0].CaseID, Arm: renewed.frozen.Inputs[0].Arm, Stage: "background", Status: "unknown", Outcome: "success", Calls: 1, EvidenceSHA256: acceptance.MemoryDigest(probe.Events), Milliseconds: &ms}
	receipt := memoryAuthReceipt{Started: issued, Finished: issued.Add(time.Millisecond), Probes: []memoryAuthProbe{probe}}
	for _, x := range []struct {
		name  string
		value any
	}{{"auth-preflight.json", receipt}, {"auth-probe-1.json", probe}, {"auth-ledger.json", map[string]any{"admissions": []map[string]any{{"id": probe.Expense.ID, "window_id": renewed.plan.WindowID, "input_mask": r.InputsSHA, "started_at": issued}}, "observations": []map[string]any{{"id": probe.Expense.ID, "record": probe}}}}} {
		if err = renewed.writeEvidence(x.name, x.value); err != nil {
			t.Fatal(err)
		}
	}
	if err = renewed.recordExpense(ctx, probe.Expense); err != nil {
		t.Fatal(err)
	}
	for n, i := range renewed.frozen.Inputs {
		renewed.factory = func(json.RawMessage) (transport.ModelProvider, error) {
			if n%2 == 0 {
				return &memoryFailureEventProvider{failureType: transport.ErrorTypeProviderUnavailable, preHTTP: true}, nil
			}
			return &memoryFixtureProvider{raw: `{}`}, nil
		}
		o, e := renewed.sample(ctx, cseFor(renewed, i), i)
		if n%2 == 0 && !errors.Is(e, errMemoryAvailability) || n%2 == 1 && e != nil {
			t.Fatal("continuation gate", e)
		}
		renewed.measurements.Observations = append(renewed.measurements.Observations, o)
		if e = renewed.fillUnused(ctx, i); e != nil {
			t.Fatal(e)
		}
	}
	if e := renewed.reserveCall(ctx, 1); e == nil {
		t.Fatal("sixth new admission allowed")
	}
	renewed.measurements.FrozenSHA256 = renewed.frozen.SHA256
	if err = renewed.writeEvidence("measurements.json", renewed.measurements); err != nil {
		t.Fatal(err)
	}
	if _, err = renewed.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET closed=TRUE`); err != nil {
		t.Fatal(err)
	}
	if err = renewed.exportLedger(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := acceptance.AuditMemoryRenewedContinuation(renewed.suite, p5, renewed.frozen, renewed.measurements, renewed.private, r.BinarySHA, runtime)
	if err != nil || a.NativeCalls != 64 || a.AuxiliaryCalls != 53 || a.QuestionResults != 56 {
		t.Fatal("complete five attempt audit", a, err)
	}
	missing := renewed.measurements
	missing.Observations = missing.Observations[:4]
	if _, err = acceptance.AuditMemoryRenewedContinuation(renewed.suite, p5, renewed.frozen, missing, renewed.private, r.BinarySHA, runtime); err == nil {
		t.Fatal("missing fifth observation accepted")
	}
	if !p5.Expires.Equal(p.Expires) {
		t.Fatal("old expiry rewritten")
	}
}
