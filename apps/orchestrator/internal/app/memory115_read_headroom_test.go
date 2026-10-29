package app

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/memorytest"
)

const guided115ExpandedFormat = "butler.experience.laba-guided-full-window.v3"
const guided115Closed768SHA = "d7699d80d7883729454fff7b9e921c82a4e6c6f8a5c6fc6a54adb42a3d07dbda"
const guided115ExpandedDecisionSHA = "f5a4b920e5bf7ed3f87893348700944e62957d468a828b72291c87c9063d94ac"
const guided115ExpandedDSN = "postgres://postgres@127.0.0.1:55451/memory115_laba_guided_full_v5_test?sslmode=disable"
const guided115ToolCalls = 32
const guided115ModelEntries = 24
const guided115Maximum = 53*guided115ModelEntries + 4

func guided115Expanded(h experienceNativeLineage) bool {
	return h.GuidedFull != nil && (h.GuidedFull.Historical.Hash == guided115Closed768SHA || h.GuidedFull.Historical.Hash == guided115Closed1049SHA)
}

// Only the new, separately authorized recipe selects this profile. Legacy
// policies and recursively checked old quotes remain byte-for-byte unchanged.
func experienceNativeReadHeadroom(p budget.Policy) (budget.Policy, error) {
	oldDay := p.Day
	delta := int64(guided115ToolCalls - 11)
	modelDelta := int64(guided115ModelEntries - 12)
	p.Task.ModelTurns += modelDelta
	p.Task.Tokens += modelDelta * 1081344
	p.Task.ProviderInputBytes += modelDelta * p.ModelInputBytes
	p.Task.ActiveMillis += modelDelta * p.ModelMillis
	p.Task.ExternalCalls += delta
	p.Task.EgressBytes += delta * p.ToolEgressBytes
	p.Task.ActiveMillis += delta * p.ToolMillis
	p.Mandate, p.Conversation = p.Task, p.Task
	p.Day = consolidationAddUnits(p.Day, budget.Units{ModelTurns: 53 * modelDelta, Tokens: 53 * modelDelta * 1081344, ProviderInputBytes: 53 * modelDelta * p.ModelInputBytes, ExternalCalls: 53 * delta, EgressBytes: 53 * delta * p.ToolEgressBytes, ActiveMillis: 53*delta*p.ToolMillis + 53*modelDelta*p.ModelMillis})
	if !consolidationV2Within(oldDay, p.Day) || p.Validate() != nil {
		return budget.Policy{}, errors.New("finite prospective read headroom overflows")
	}
	return p, nil
}

func experienceNativePolicyForHistory(source bool, h experienceNativeLineage) (budget.Policy, error) {
	p, err := experienceNativeCarryPolicy(source, h.Carry)
	if err != nil || !guided115Expanded(h) {
		return p, err
	}
	return experienceNativeReadHeadroom(p)
}

func TestGuided115ReadHeadroomIsProspectiveAndKeepsFullCarry(t *testing.T) {
	h := experienceNativeLineage{Carry: budget.Units{ModelTurns: 769, Tokens: 9000000, ExternalCalls: 400, ActiveMillis: 1000000, EgressBytes: 1000}}
	legacy, err := experienceNativePolicyForHistory(false, h)
	if err != nil || legacy.Task.ExternalCalls != 11 {
		t.Fatal("legacy policy changed", err)
	}
	h.GuidedFull = &guided115Binding{Historical: experienceNativeLineage{Hash: guided115Closed768SHA}}
	for _, source := range []bool{false, true} {
		p, err := experienceNativePolicyForHistory(source, h)
		models := int64(guided115ModelEntries)
		if source {
			models++
		}
		if err != nil || p.Task.ExternalCalls != 32 || p.Mandate != p.Task || p.Conversation != p.Task || p.Task.ModelTurns != models || p.Day.ExternalCalls != 53*32+h.Carry.ExternalCalls || p.Day.ModelTurns != guided115Maximum+h.Carry.ModelTurns || p.Day.Tokens != legacy.Day.Tokens+53*12*1081344 || !consolidationV2Within(h.Carry, p.Day) {
			t.Fatal("expanded profile lost carry, changed inference limit or exceeded partition", source, err)
		}
	}
}

func experienceNativeScheduleForHistory(c experienceNativeCorpus, h experienceNativeLineage) []experienceNativeSlot {
	slots := experienceNativeSchedule(c)
	if guided115Expanded(h) {
		for i := range slots {
			if slots[i].Kind != "note" {
				slots[i].Maximum = guided115ModelEntries
			}
		}
	}
	return slots
}

func TestGuided115ExpandedJournalBoundsIntegration(t *testing.T) {
	h := experienceNativeLineage{GuidedFull: &guided115Binding{Historical: experienceNativeLineage{Hash: guided115Closed768SHA}}}
	slots := experienceNativeScheduleForHistory(experienceWindowFixture(), h)
	p, err := newExperienceNativeProvider(t.TempDir(), slots)
	if err != nil || p.maximum != 1276 || len(slots) != 57 || slots[4].Maximum != 1 || slots[56].Maximum != 24 {
		t.Fatal("expanded full denominator changed", err)
	}
	pool := memorytest.Database(t)
	ctx := context.Background()
	if err = p.installGlobal(ctx, pool, experienceDigest(t.Name())); err != nil {
		t.Fatal(err)
	}
	// Synthetic journal rows only, no provider/credential/network request. A
	// per-slot rejection must roll back the independent total as well.
	entry := func(i int, slot int) *experienceNativeEntry {
		return &experienceNativeEntry{Sequence: i, Slot: slots[slot].ID, RunID: "controlled", Kind: "start", Operation: fmt.Sprint(i), Request: []byte(`{}`)}
	}
	for i := 1; i <= 24; i++ {
		if err = p.reserveGlobal(ctx, entry(i, 0)); err != nil {
			t.Fatal(err)
		}
	}
	if err = p.reserveGlobal(ctx, entry(25, 0)); err == nil {
		t.Fatal("25th entry escaped slot limit")
	}
	var used, maximum int
	if err = pool.QueryRow(ctx, `SELECT used,maximum FROM experience_native_windows WHERE window_id=$1`, p.window).Scan(&used, &maximum); err != nil || used != 24 || maximum != 1276 {
		t.Fatal("slot rejection lost global accounting", err, used, maximum)
	}
	if _, err = pool.Exec(ctx, `UPDATE experience_native_windows SET used=maximum WHERE window_id=$1`, p.window); err != nil {
		t.Fatal(err)
	}
	if err = p.reserveGlobal(ctx, entry(1277, 4)); err == nil {
		t.Fatal("global ceiling escaped")
	}
}
