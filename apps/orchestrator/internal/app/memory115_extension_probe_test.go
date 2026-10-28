package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestExperienceNativeExpiredProbeKeepsGenerationAndForegroundLimitsIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, done := context.WithTimeout(t.Context(), time.Minute)
	defer done()
	dir := t.TempDir()
	anchor := filepath.Join(dir, "anchor")
	manifest := catalog.NewFileManifest(anchor)
	if manifest.Initialize() != nil {
		t.Fatal("new private anchor required")
	}
	addresses := newExperienceNativeAddresses(t)
	makeRuntime := func(name string, policy budget.Policy, version string) *experienceNativeRuntime {
		t.Helper()
		stage := filepath.Join(dir, name)
		if err := os.Mkdir(stage, 0700); err != nil {
			t.Fatal(err)
		}
		ready, initial := "true", "ready=false\n"
		if version == "v2" {
			ready, initial = "verified", "ready=pending\n"
		}
		recipient, err := newExperienceRecipient(stage, experienceRecipientConfig{Version: version, Ready: ready, Files: map[string]string{"alpha": "Probe fixture\n" + initial, "beta": "Other fixture\n" + initial}})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = recipient.close() })
		return newExperienceNativeRuntime(t, pool, catalog.New(pool, manifest), anchor, stage, recipient, addresses, policy, "Current probe only", experienceNativeArms[0], nil, time.Now().Add(time.Hour))
	}
	source := makeRuntime("source", experienceNativeSourcePolicy(), "v1")
	generation := source.generation
	source.close()
	if _, err := pool.Exec(ctx, `UPDATE extension_installations SET healthy_until=clock_timestamp()-interval '1 second' WHERE extension_id='project'`); err != nil {
		t.Fatal(err)
	}
	day := time.Now().UTC().Truncate(24 * time.Hour)
	controlPolicy := experienceNativeBudgetPolicy()
	_, err := budget.NewLedger(pool).Ensure(ctx, budget.ScopeSpec{Kind: "conversation", OwnerID: experienceNativeOwner, SubjectID: "extension-admin:" + day.Format("2006-01-02"), Limits: controlPolicy.Conversation, Deadline: day.Add(24 * time.Hour)})
	if !errors.Is(err, budget.ErrConflict) {
		t.Fatal("old13/12 administration mismatch was not reproduced", err)
	}
	control := makeRuntime("control", controlPolicy, "v1")
	defer control.close()
	if generation != 3 || control.generation != generation || !reflect.DeepEqual(control.budgets.Policy, controlPolicy) || control.budgets.Policy.Task.ModelTurns != 12 || control.budgets.Policy.Mandate.ModelTurns != 12 || control.budgets.Policy.Conversation.ModelTurns != 12 {
		t.Fatal("readiness refresh changed generation or foreground policy")
	}
	var probes int
	var calls int64
	var healthy bool
	if pool.QueryRow(ctx, `SELECT count(*) FROM extension_probes WHERE state='ready'`).Scan(&probes) != nil || probes != 2 || pool.QueryRow(ctx, `SELECT (used_json->>'external_calls')::bigint FROM execution_budget_scopes WHERE kind='conversation' AND subject_id=$1`, "extension-admin:"+day.Format("2006-01-02")).Scan(&calls) != nil || calls != 2 || pool.QueryRow(ctx, `SELECT status='enabled' AND health='healthy' AND healthy_until>clock_timestamp() AND healthy_until<clock_timestamp()+interval '16 minutes' FROM extension_installations WHERE extension_id='project'`).Scan(&healthy) != nil || !healthy {
		t.Fatal("both actual probes, unchanged finite TTL and retained administration charges required")
	}
	control.close()
	drift := makeRuntime("drift", controlPolicy, "v2")
	defer drift.close()
	if drift.pack.Digest != source.pack.Digest || drift.environment == source.environment || drift.generation != generation {
		t.Fatal("recipient drift rewrote the immutable ABI package or lost environment separation")
	}
	if pool.QueryRow(ctx, `SELECT count(*) FROM extension_probes WHERE state='ready'`).Scan(&probes) != nil || probes != 3 {
		t.Fatal("actual changed-recipient readiness probe missing")
	}
	t.Log("OLD_ADMIN_POLICY_CONFLICT_REPRODUCED; actual fresh probe preserves generation3, foreground12/11, finite15m health and both charged probes; model/auth0")
}
