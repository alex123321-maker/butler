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
	return c.recordExpense(ctx, acceptance.MemoryExpense{ID: id + "/replay-read", CaseID: id, Arm: "retrieval", Stage: "read", Status: "measured", Outcome: "success", EvidenceSHA256: capture.EvidenceSHA256, Calls: 1, InputTokens: &zero, OutputTokens: &zero, TotalTokens: &zero, Milliseconds: &ms, Microunits: &zero, StorageBytes: &bytes})
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
	current := memoryCollectorFixtureWithPrior(t, &prior, &pins, old.private)
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
	if _, err = current.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET closed=TRUE`); err != nil {
		t.Fatal(err)
	}
	if err = current.exportLedger(ctx); err != nil {
		t.Fatal(err)
	}
	audit, err := acceptance.AuditMemoryContinuation(current.suite, prior, current.frozen, current.measurements, current.private, strings.Repeat("5", 64), pins)
	if err != nil || audit.NativeCalls != 64 || audit.AuxiliaryCalls != 53 || audit.QuestionResults != 63 {
		t.Fatal("pending correlation", audit, err)
	}
	combined, err := acceptance.CombineMemoryMeasurements(prior, current.measurements)
	if err != nil || len(combined.Observations) != 72 || combined.Observations[0].Status != "failed" {
		t.Fatal("failed denominator lost", err)
	}
	retried := current.measurements
	retried.Observations = append(append([]acceptance.MemoryObservation(nil), retried.Observations...), o)
	if _, err = acceptance.CombineMemoryMeasurements(prior, retried); err == nil {
		t.Fatal("spent entry resampled")
	}
	changed := current.frozen
	changed.Inputs = append(append([]acceptance.MemoryInput(nil), changed.Inputs...), prior.PaidInput)
	changed.SHA256 = acceptance.MemoryFrozenDigest(changed)
	if _, err = acceptance.AuditMemoryContinuation(current.suite, prior, changed, current.measurements, current.private, strings.Repeat("5", 64), pins); err == nil {
		t.Fatal("paid schedule entry accepted")
	}
	if _, err = current.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET prior_reserved_units=0`); err == nil {
		t.Fatal("carried spend refunded")
	}
	if _, err = current.pool.Exec(ctx, `UPDATE fixture_memory_eval_window SET expires_at=expires_at+interval '1 hour'`); err == nil {
		t.Fatal("absolute clock reset")
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
