package goal_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
)

func TestGeneratedConverterMigrationRollbackGuardsIntegration(t *testing.T) {
	f := setup(t)
	load := func(name string) string {
		t.Helper()
		b, e := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", name))
		if e != nil {
			t.Fatal(e)
		}
		return string(b)
	}
	if _, e := f.pool.Exec(f.ctx, load("052_generated_converters.down.sql")); e != nil {
		t.Fatal("empty converter rollback", e)
	}
	if _, e := f.pool.Exec(f.ctx, load("052_generated_converters.up.sql")); e != nil {
		t.Fatal("empty converter reapply", e)
	}
	var count int
	if f.pool.QueryRow(f.ctx, `SELECT count(*) FROM converter_catalog`).Scan(&count) != nil || count != 0 {
		t.Fatal("converter schema did not reapply empty")
	}
	flow := setupConverter(t)
	flow.candidate(t, "1.0.0")
	if _, e := flow.f.pool.Exec(flow.f.ctx, load("052_generated_converters.down.sql")); e == nil {
		t.Fatal("rollback discarded retained need/version/source/effect audit")
	}
	if flow.f.pool.QueryRow(flow.f.ctx, `SELECT count(*) FROM converter_versions`).Scan(&count) != nil || count != 1 {
		t.Fatal("rejected rollback lost durable converter history")
	}
}

func TestGeneratedConverterMissingCoreLifecycleDeniesBeforeReleaseIntegration(t *testing.T) {
	f := setupConverter(t)
	f.env.lifecycleDisabled = true
	c, e := f.env.GetActionContext(f.f.ctx, f.env.run)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.Catalog(f.f.ctx, c, "", 64); e == nil {
		t.Fatal("misconfigured core exposed converter capabilities")
	}
	raw, _ := json.Marshal(map[string]any{"source_ref": f.needRef})
	call := &toolbrokerv1.ToolCall{RunId: f.env.run, ToolCallId: "missing-core-guard", ToolName: "converters.discover", ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
	if _, e := f.auth.Prepare(f.f.ctx, call, generatedtools.Contracts()[0]); e == nil {
		t.Fatal("misconfigured core prepared an authorized source release")
	}
	if f.runtime.developCalls.Load() != 0 || f.runtime.convertCalls.Load() != 0 {
		t.Fatal("missing core guards released runtime DATA")
	}
}

func TestGeneratedConverterPinnedProviderRecoveryResultGuardIntegration(t *testing.T) {
	f := setupConverter(t)
	r := f.candidate(t, "1.0.0")
	f.tested(t, r)
	f.trial(t, r)
	ctx, call, contract, grant := f.action(t, "converters.activate", map[string]any{"digest": r.Digest, "generation": r.Generation})
	active, e := f.store.Activate(ctx, call, contract, r.Digest, r.Generation)
	if e != nil {
		t.Fatal(e)
	}
	f.finish(t, grant, active)
	ctx, call, contract, grant = f.action(t, "converter.inventory.convert", map[string]any{"input_ref": f.inputRef})
	if _, e = f.store.Apply(ctx, call, contract, r.Digest, f.inputRef, false); e != nil {
		t.Fatal(e)
	}
	f.finish(t, grant, map[string]any{"stored_output": true})
	check := func() error {
		tx, e := f.f.pool.Begin(f.f.ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(f.f.ctx)
		return generatedtools.CheckPinnedLifecycleTx(f.f.ctx, tx, f.env.run)
	}
	if e = check(); e != nil {
		t.Fatal("current exact generated use denied", e)
	}
	if _, e = f.store.Revoke(f.f.actor("revoke-before-provider-recovery-final"), active.Digest, active.Generation); e != nil {
		t.Fatal(e)
	}
	// This same transaction guard composes after current source checks at model
	// release/recovery and atomic final-result publication. A saved known result
	// cannot keep a revoked catalog version current or trigger redispatch.
	if e = check(); e == nil {
		t.Fatal("saved converter observation kept revoked provider/result authority")
	}
	var status string
	if f.f.pool.QueryRow(f.f.ctx, `SELECT status FROM tasks WHERE task_id=(SELECT task_id FROM runs WHERE run_id=$1)`, f.env.run).Scan(&status) != nil || status != "active" {
		t.Fatal("revocation changed semantic task truth")
	}
}

func TestGeneratedConverterPrepareOnlyCannotActivateIntegration(t *testing.T) {
	f := setupConverter(t)
	r := f.candidate(t, "1.0.0")
	f.tested(t, r)
	f.trial(t, r)
	if _, e := f.f.pool.Exec(f.f.ctx, `UPDATE tasks SET execution_policy='prepare_only' WHERE task_id=(SELECT task_id FROM runs WHERE run_id=$1)`, f.env.run); e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(map[string]any{"digest": r.Digest, "generation": r.Generation})
	call := &toolbrokerv1.ToolCall{RunId: f.env.run, ToolCallId: "prepare-only-activation", ToolName: "converters.activate", ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
	var contract *toolbrokerv1.ToolContract
	for _, c := range generatedtools.Contracts() {
		if c.ToolName == call.ToolName {
			contract = c
		}
	}
	if _, e := f.auth.Prepare(f.f.ctx, call, contract); e == nil {
		t.Fatal("broad converter mandate widened a prepare-only task to activation")
	}
	var active bool
	if f.f.pool.QueryRow(f.f.ctx, `SELECT active_digest IS NOT NULL FROM converter_catalog WHERE owner_id=$1 AND converter_id=$2`, f.f.owner, f.need.ConverterID).Scan(&active) != nil || active {
		t.Fatal("denied activation changed the working catalog")
	}
}
