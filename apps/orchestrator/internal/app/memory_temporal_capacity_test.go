package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	promptmgmt "github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestTemporalProspectiveV3CapacityArithmetic(t *testing.T) {
	carry := budget.Units{ModelTurns: 86, Tokens: 91766784, ActiveMillis: 15540000, ProviderInputBytes: 721420288, ExternalCalls: 2, EgressBytes: 8388608}
	p := temporalNativePolicy(carry)
	wantTask := budget.Units{ModelTurns: 12, Tokens: 12976128, ActiveMillis: 4560000, ProviderInputBytes: 100663296, ExternalCalls: 8, EgressBytes: 33554432}
	wantDay := budget.Units{ModelTurns: 230, Tokens: 247480320, ActiveMillis: 70260000, ProviderInputBytes: 1929379840, ExternalCalls: 98, EgressBytes: 411041792}
	if p.Validate() != nil || p.Task != wantTask || p.Day != wantDay || temporalMaxNative != 144 || temporalMaxToolCalls != 96 {
		t.Fatal("prospective finite capacity or retained spend differs", p.Task, p.Day)
	}
	raw, _ := json.Marshal(p)
	decoded, err := budget.ParsePolicy(string(raw))
	if err != nil || decoded.Task != wantTask || decoded.Day != wantDay {
		t.Fatal("Core/Broker shared policy roundtrip differs")
	}
}

// Capacity permits independently verifying both selected original sources after
// a typed facts read, followed by the final answer. It never changes the model
// prompt, semantic rubric, production tool declarations or authority rules.
func TestTemporalCollectorFactsThenBothOriginalsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx := context.Background()
	proof := t.TempDir()
	manifest := filepath.Join(proof, "deletion-anchor")
	m := catalog.NewFileManifest(manifest)
	if m.Initialize() != nil {
		t.Fatal("anchor")
	}
	cat := catalog.New(pool, m)
	if _, err := pool.Exec(ctx, temporalNativeSchemaSQL()); err != nil {
		t.Fatal(err)
	}
	cases := temporalPrepareCases(t, ctx, pool, cat, temporalLoadCorpus(t))
	if _, err := pool.Exec(ctx, `UPDATE temporal_native_window SET sealed=TRUE,input_hash='controlled' WHERE singleton`); err != nil {
		t.Fatal(err)
	}
	policy := temporalNativePolicy(budget.Units{})
	broker := temporalCollectorBroker(t, pool, cat, manifest, proof, policy)
	b := budget.NewController(pool, policy)
	// Same authority resolver as the actual ordinary collector.
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	pm := promptmgmt.NewManager(config.NewPostgresSettingsStore(pool))
	on := true
	if _, err := pm.Update(ctx, promptmgmt.UpdateRequest{BasePrompt: temporalNativeInstruction, Enabled: &on, UpdatedBy: "controlled-capacity-v3"}); err != nil {
		t.Fatal(err)
	}
	p := &temporalNativeProvider{ModelProvider: &temporalControlledProvider{pool: pool, openOriginals: true}, pool: pool, proof: proof}
	ob := temporalRunOriginal(t, ctx, pool, cat, p, b, broker, pm, cases[0])
	var completed, opened int
	if err := pool.QueryRow(ctx, `SELECT count(*)FILTER(WHERE status='completed'),count(*)FILTER(WHERE status='completed' AND tool_name='memory.open') FROM tool_calls WHERE run_id=$1`, ob.RunID).Scan(&completed, &opened); err != nil {
		t.Fatal(err)
	}
	if ob.Status != "completed" || ob.NativeEntries != 4 || ob.KnownUsage != 4 || ob.FactsReads != 1 || completed != 3 || opened != 2 {
		t.Fatal("legitimate facts/open/open/final strategy stopped", ob, completed, opened)
	}
}
