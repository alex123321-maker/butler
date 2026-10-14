package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

func (c *memoryEvalCollector) reuseCapture(ctx context.Context, id string, refs map[string]catalog.SourceRef) error {
	start := time.Now()
	name := strings.TrimPrefix(id, "memory:")
	raw, err := acceptance.ReadPrivateFile(filepath.Join(c.priorDirectory, "bindings-"+name+".json"), 16<<20)
	var original map[string]catalog.SourceRef
	if err != nil || json.Unmarshal(raw, &original) != nil || acceptance.MemoryDigest(original) != acceptance.MemoryDigest(refs) {
		return errors.New("rebased source authority differs from original capture")
	}
	var capture acceptance.MemoryCapture
	found := false
	for _, cap := range c.prior.Frozen.Captures {
		if cap.CaseID == id {
			capture, found = cap, true
		}
	}
	if !found || capture.ConfigSHA256 != c.plan.MemoryConfigSHA256 {
		return errors.New("original capture identity missing")
	}
	before := c.retainedBytes
	for _, prefix := range []string{"bundle-", "search-"} {
		file := prefix + name + ".json"
		raw, err := acceptance.ReadPrivateFile(filepath.Join(c.priorDirectory, file), 16<<20)
		if err != nil {
			return err
		}
		if err = c.writeOriginal(file, raw); err != nil {
			return err
		}
	}
	c.captures = append(c.captures, capture)
	zero, ms, bytes := int64(0), time.Since(start).Milliseconds(), c.retainedBytes-before
	return c.recordExpense(ctx, acceptance.MemoryExpense{ID: id + "/replay-read/" + c.plan.WindowID, CaseID: id, Arm: "retrieval", Stage: "read", Status: "measured", Outcome: "success", EvidenceSHA256: capture.EvidenceSHA256, Calls: 1, InputTokens: &zero, OutputTokens: &zero, TotalTokens: &zero, Milliseconds: &ms, Microunits: &zero, StorageBytes: &bytes})
}

type memoryFailureEventProvider struct {
	nativeScriptedObservationProvider
}

func (*memoryFailureEventProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	for n := 0; n < 2; n++ {
		if err := transport.CheckInferenceAuthority(ctx); err != nil {
			return nil, err
		}
	}
	return nativeScriptedStream(transport.NewEvent(r.Context.RunID, transport.EventTypeRunStarted, antigravity.Name), transport.NewTransportErrorEvent(r.Context.RunID, antigravity.Name, &transport.Error{Type: transport.ErrorTypeProviderUnavailable, Message: "PRIVATE_NOT_EXPORTABLE"})), nil
}

func TestMemoryEvalCollectorPendingCrossRootCorrelationIntegration(t *testing.T) {
	old := memoryCollectorFixture(t)
	ctx := context.Background()
	for _, cse := range old.suite.Cases {
		if err := old.capture(ctx, cse); err != nil {
			t.Fatal(err)
		}
	}
	var err error
	old.frozen, err = acceptance.FreezeMemory(old.suite, old.plan, old.captures)
	if err != nil {
		t.Fatal(err)
	}
	if err = old.writeEvidence("frozen-before-native.json", old.frozen); err != nil {
		t.Fatal(err)
	}
	old.factory = func(json.RawMessage) (transport.ModelProvider, error) { return &memoryFailureEventProvider{}, nil }
	i := old.frozen.Inputs[0]
	o, err := old.sample(ctx, old.suite.Cases[0], i)
	if err == nil || o.Status != "failed" {
		t.Fatal("failed prefix missing", err)
	}
	old.measurements.Observations = append(old.measurements.Observations, o)
	old.measurements.FrozenSHA256 = old.frozen.SHA256
	if err = old.fillUnused(ctx, i); err != nil {
		t.Fatal(err)
	}
	if err = old.writeEvidence("measurements.json", old.measurements); err != nil {
		t.Fatal(err)
	}
	if _, err = old.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET closed=TRUE`); err != nil {
		t.Fatal(err)
	}
	if err = old.exportLedger(ctx); err != nil {
		t.Fatal(err)
	}
	var pins acceptance.MemoryPriorPins
	pins.SourceSHA, pins.CodeCommit, pins.BinarySHA = old.plan.SourceSHA256, old.plan.CodeCommit, strings.Repeat("4", 64)
	var expiry time.Time
	if err = old.pool.QueryRow(ctx, `SELECT calls,reserved_units,expires_at FROM fixture_memory_eval_window`).Scan(&pins.Calls, &pins.ReservedUnits, &expiry); err != nil {
		t.Fatal(err)
	}
	pins.ExpiresUTC = expiry.UTC().Format(time.RFC3339Nano)
	for _, item := range []struct {
		name string
		out  *string
	}{{"ledger.json", &pins.LedgerSHA}, {"frozen-before-native.json", &pins.FrozenFileSHA}, {"measurements.json", &pins.MeasurementsSHA}} {
		*item.out, err = acceptance.FileHash(filepath.Join(old.private, item.name))
		if err != nil {
			t.Fatal(err)
		}
	}
	prior, err := acceptance.LoadMemoryPrior(old.suite, old.private, pins)
	if err != nil {
		t.Fatal("prefix correlation", err)
	}
	// Retain a second paid failure in a separate root. Its full-context slot
	// must disappear only from admission, never from scoring or expenses.
	mid := memoryCollectorFixtureWithPrior(t, &prior, &pins, old.private)
	mid.backend = &memoryFixtureBackend{fail: true}
	for _, cse := range mid.suite.Cases {
		if err = mid.capture(ctx, cse); err != nil {
			t.Fatal(err)
		}
	}
	mid.frozen, err = acceptance.MemoryPendingSchedule(prior, mid.plan)
	if err != nil {
		t.Fatal(err)
	}
	if err = mid.writeEvidence("frozen-before-native.json", mid.frozen); err != nil {
		t.Fatal(err)
	}
	mid.factory = func(json.RawMessage) (transport.ModelProvider, error) { return &memoryFailureEventProvider{}, nil }
	midObs, sampleErr := mid.sample(ctx, mid.suite.Cases[0], mid.frozen.Inputs[0])
	if sampleErr == nil || midObs.Status != "failed" {
		t.Fatal("second paid failure missing")
	}
	mid.measurements.Observations = append(mid.measurements.Observations, midObs)
	mid.measurements.FrozenSHA256 = mid.frozen.SHA256
	if err = mid.fillUnused(ctx, mid.frozen.Inputs[0]); err != nil {
		t.Fatal(err)
	}
	midCombined, err := acceptance.CombineMemoryMeasurements(prior, mid.measurements)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name  string
		value any
	}{{"measurements.json", mid.measurements}, {"comparison-frozen.json", prior.Frozen}, {"comparison-measurements.json", midCombined}} {
		if err = mid.writeEvidence(item.name, item.value); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = mid.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET closed=TRUE`); err != nil {
		t.Fatal(err)
	}
	if err = mid.exportLedger(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = acceptance.AuditMemoryContinuationPrefix(mid.suite, prior, mid.frozen, mid.measurements, mid.private, strings.Repeat("5", 64), pins); err != nil {
		t.Fatal("second prefix", err)
	}
	parentPins := pins
	pins = acceptance.MemoryPriorPins{Parent: &parentPins, SourceSHA: mid.plan.SourceSHA256, CodeCommit: mid.plan.CodeCommit, BinarySHA: strings.Repeat("5", 64), ExpiresUTC: parentPins.ExpiresUTC}
	if err = mid.pool.QueryRow(ctx, `SELECT calls,reserved_units FROM fixture_memory_eval_window`).Scan(&pins.Calls, &pins.ReservedUnits); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name string
		out  *string
	}{{"ledger.json", &pins.LedgerSHA}, {"frozen-before-native.json", &pins.FrozenFileSHA}, {"measurements.json", &pins.MeasurementsSHA}} {
		*item.out, err = acceptance.FileHash(filepath.Join(mid.private, item.name))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = os.Mkdir(filepath.Join(mid.private, "prior"), 0700); err != nil {
		t.Fatal(err)
	}
	entriesOld, err := os.ReadDir(old.private)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entriesOld {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".json") && !strings.HasSuffix(entry.Name(), ".txt")) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(old.private, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(mid.private, "prior", entry.Name()), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	prior, err = acceptance.LoadMemoryPrior(mid.suite, mid.private, pins)
	if err != nil || len(prior.PaidInputs) != 2 || len(prior.Measurements.Observations) != 2 || prior.Audit.NativeCalls != 2 || prior.Audit.AuxiliaryCalls != 53 {
		t.Fatal("two-prefix lineage", err)
	}
	for _, kind := range []string{"third_parent", "refunded_counter", "reset_deadline"} {
		bad := pins
		switch kind {
		case "third_parent":
			parent, grandparent := *pins.Parent, *pins.Parent
			parent.Parent, bad.Parent = &grandparent, &parent
		case "refunded_counter":
			bad.Calls, bad.ReservedUnits = parentPins.Calls, parentPins.ReservedUnits
		case "reset_deadline":
			bad.ExpiresUTC = prior.Expires.Add(time.Hour).Format(time.RFC3339Nano)
		}
		if _, err = acceptance.LoadMemoryPrior(mid.suite, mid.private, bad); err == nil {
			t.Fatal("forged two-prefix lineage accepted", kind)
		}
	}
	// The loader recomputes the composite from original parent expenses. An
	// altered comparison cannot replace a paid failure or drop an old cost.
	for _, kind := range []string{"dropped_expense", "replaced_failure"} {
		forged := t.TempDir()
		for _, rel := range []string{"", "prior"} {
			if rel != "" {
				if err = os.Mkdir(filepath.Join(forged, rel), 0700); err != nil {
					t.Fatal(err)
				}
			}
			entries, err := os.ReadDir(filepath.Join(mid.private, rel))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				raw, err := os.ReadFile(filepath.Join(mid.private, rel, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				if rel == "" && entry.Name() == "comparison-measurements.json" {
					var changed acceptance.MemoryMeasurements
					if err = json.Unmarshal(raw, &changed); err != nil {
						t.Fatal(err)
					}
					if kind == "dropped_expense" {
						changed.Expenses = changed.Expenses[1:]
					} else {
						changed.Observations[0].Status = "completed"
					}
					raw, err = json.Marshal(changed)
					if err != nil {
						t.Fatal(err)
					}
				}
				if err = os.WriteFile(filepath.Join(forged, rel, entry.Name()), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
		if _, err = acceptance.LoadMemoryPrior(mid.suite, forged, pins); err == nil || !strings.Contains(err.Error(), "retained composite") {
			t.Fatal("retained composite forgery accepted", kind, err)
		}
	}
	current := memoryCollectorFixtureWithPrior(t, &prior, &pins, mid.private)
	current.backend = &memoryFixtureBackend{fail: true} // Any accidental Infer would fail.
	for _, cse := range current.suite.Cases {
		if err = current.capture(ctx, cse); err != nil {
			t.Fatal("rebase", err)
		}
	}
	if err = current.verifyRebasedChunks(ctx); err != nil {
		t.Fatal(err)
	}
	current.frozen, err = acceptance.MemoryPendingSchedule(prior, current.plan)
	if err != nil {
		t.Fatal(err)
	}
	if err = current.writeEvidence("frozen-before-native.json", current.frozen); err != nil {
		t.Fatal(err)
	}
	for _, input := range current.frozen.Inputs {
		var cse acceptance.MemoryCase
		for _, candidate := range current.suite.Cases {
			if acceptance.MemoryOpaqueCaseID(candidate) == input.CaseID {
				cse = candidate
			}
		}
		raw, _ := json.Marshal(acceptance.MemoryAnswer{Abstain: true, Citations: []acceptance.CommandWitness{}, TaskRevision: input.Task.Revision, NextAction: cse.Expected.NextAction})
		current.factory = func(json.RawMessage) (transport.ModelProvider, error) {
			return &memoryFixtureProvider{raw: string(raw)}, nil
		}
		observation, err := current.sample(ctx, cse, input)
		if err != nil {
			t.Fatal(err)
		}
		if input.Arm == "retrieval" && input.SkipReason == "" && observation.PipelineMilliseconds != nil {
			t.Fatal("reused full pipeline time invented")
		}
		current.measurements.Observations = append(current.measurements.Observations, observation)
		if err = current.fillUnused(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	current.measurements.FrozenSHA256 = current.frozen.SHA256
	if err = current.writeEvidence("measurements.json", current.measurements); err != nil {
		t.Fatal(err)
	}
	// Probe each immutable field while open, so closed-window rejection cannot
	// mask a missing carry or deadline guard.
	if _, err = current.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET prior_reserved_units=0`); err == nil {
		t.Fatal("carried spend refunded")
	}
	if _, err = current.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET expires_at=expires_at+interval '1 hour'`); err == nil {
		t.Fatal("absolute clock reset")
	}
	if _, err = current.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET closed=TRUE`); err != nil {
		t.Fatal(err)
	}
	if err = current.exportLedger(ctx); err != nil {
		t.Fatal(err)
	}
	audit, err := acceptance.AuditMemoryContinuation(current.suite, prior, current.frozen, current.measurements, current.private, strings.Repeat("5", 64), pins)
	if err != nil || audit.NativeCalls != 64 || audit.AuxiliaryCalls != 53 || audit.QuestionResults != 62 {
		t.Fatal("pending correlation", audit, err)
	}
	combined, err := acceptance.CombineMemoryMeasurements(prior, current.measurements)
	if err != nil || len(combined.Observations) != 72 || combined.Observations[0].Status != "failed" {
		t.Fatal("failed denominator lost", err)
	}
	score, err := acceptance.ScoreMemory(current.suite, prior.Frozen, combined, acceptance.GoldenApproval{}, nil)
	if err != nil || len(score.Cases) != 72 || score.Arms["no_ltm"].Cases != 24 || score.Arms["no_ltm"].Cost.FailedSamples != 1 || score.Arms["full_context"].Cost.FailedSamples != 1 {
		t.Fatal("composite denominator lost", err)
	}
	for _, reason := range score.Reasons {
		if reason == "contradictory_unused_stage" {
			t.Fatal("carried expense contradicted by new unused marker")
		}
	}
	retried := current.measurements
	retried.Observations = append(append([]acceptance.MemoryObservation(nil), retried.Observations...), o)
	if _, err = acceptance.CombineMemoryMeasurements(prior, retried); err == nil {
		t.Fatal("spent entry resampled")
	}
	retried.Observations[len(retried.Observations)-1] = midObs
	if _, err = acceptance.CombineMemoryMeasurements(prior, retried); err == nil {
		t.Fatal("second spent entry resampled")
	}
	changed := current.frozen
	changed.Inputs = append(append([]acceptance.MemoryInput(nil), changed.Inputs...), prior.PaidInput)
	changed.SHA256 = acceptance.MemoryFrozenDigest(changed)
	if _, err = acceptance.AuditMemoryContinuation(current.suite, prior, changed, current.measurements, current.private, strings.Repeat("5", 64), pins); err == nil {
		t.Fatal("paid schedule entry accepted")
	}
	// Exported totals must also retain every conservative reservation, even if
	// an import keeps the correct immutable prior offset field.
	ledgerBytes, err := acceptance.ReadPrivateFile(filepath.Join(current.private, "ledger.json"), 32<<20)
	if err != nil {
		t.Fatal(err)
	}
	var ledger map[string]json.RawMessage
	_ = json.Unmarshal(ledgerBytes, &ledger)
	var window []map[string]any
	_ = json.Unmarshal(ledger["fixture_memory_eval_window"], &window)
	window[0]["reserved_units"] = pins.ReservedUnits
	ledger["fixture_memory_eval_window"], _ = json.Marshal(window)
	forgedDir := t.TempDir()
	entries, err := os.ReadDir(current.private)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(current.private, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if entry.Name() == "ledger.json" {
			raw, err = json.Marshal(ledger)
			if err != nil {
				t.Fatal(err)
			}
		}
		if err = os.WriteFile(filepath.Join(forgedDir, entry.Name()), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = acceptance.AuditMemoryContinuation(current.suite, prior, current.frozen, current.measurements, forgedDir, strings.Repeat("5", 64), pins); err == nil || !strings.Contains(err.Error(), "accumulated conservative charge") {
		t.Fatal("current conservative reservations dropped", err)
	}
}

// Rebased chunk IDs/source links must also match. Original search hits are
// validated against this new catalog in the final offline provenance audit.
func (c *memoryEvalCollector) verifyRebasedChunks(ctx context.Context) error {
	if c.prior == nil {
		return nil
	}
	raw, err := acceptance.ReadPrivateFile(filepath.Join(c.priorDirectory, "ledger.json"), 32<<20)
	var old map[string]json.RawMessage
	if err != nil || json.Unmarshal(raw, &old) != nil {
		return errors.New("original source ledger absent")
	}
	for _, table := range []string{"memory_object_sources", "memory_chunks"} {
		// Compare canonical identity/content, not new creation timestamps.
		var current []byte
		if err = c.pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]'::jsonb) FROM `+table+` t`).Scan(&current); err != nil {
			return errors.New("rebased source ledger absent")
		}
		var a, b []map[string]any
		if json.Unmarshal(current, &a) != nil || json.Unmarshal(old[table], &b) != nil || len(a) != len(b) {
			return errors.New("rebased chunk count differs")
		}
		key := "id"
		if table == "memory_object_sources" {
			key = "object_id"
		}
		index := map[string]map[string]any{}
		for _, row := range b {
			delete(row, "created_at")
			delete(row, "updated_at")
			index[acceptance.MemoryDigest(row[key])] = row
		}
		for _, row := range a {
			delete(row, "created_at")
			delete(row, "updated_at")
			if acceptance.MemoryDigest(row) != acceptance.MemoryDigest(index[acceptance.MemoryDigest(row[key])]) {
				return errors.New("rebased chunk binding or original body differs")
			}
		}
	}
	return nil
}
