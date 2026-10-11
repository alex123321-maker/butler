package goal_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/internal/actionauth"
)

func TestExactConverterPermissionStoredAndRevocationPinnedIntegration(t *testing.T) {
	f := setup(t)
	p := actionauth.ConverterPermission{ConverterID: "inventory", Class: "delimited_json_v1", InputSchemaHash: strings.Repeat("a", 64), OutputSchemaHash: strings.Repeat("b", 64), RuntimePin: strings.Repeat("c", 64), InputBytes: 8192, OutputBytes: 32768, Rows: 128, FieldBytes: 1024, Millis: 1000, Trials: 2}
	m := f.m
	m.Tools = append(append([]string{}, m.Tools...), "converter.inventory.convert")
	m.ResourceScopes = append(append([]actionauth.ResourceScope{}, m.ResourceScopes...), actionauth.ResourceScope{Origin: "tool:converter.inventory.convert"})
	m.Effects = append(append([]actionauth.Effect{}, m.Effects...), actionauth.EffectPrepare)
	m.Converters = []actionauth.ConverterPermission{p}
	if _, e := f.manager.PutMandate(f.ctx, goal.MandateRequest{Mandate: m, ExpectedRevision: m.Revision}); !errors.Is(e, goal.ErrUnauthorized) {
		t.Fatal("model-authored permission admitted", e)
	}
	r, e := f.manager.PutMandate(f.actor("bounded-converter-permission"), goal.MandateRequest{Mandate: m, ExpectedRevision: m.Revision})
	if e != nil {
		t.Fatal(e)
	}
	f.m, e = actionauth.NewPostgresStore(f.pool).GetMandate(f.ctx, r.SubjectID)
	if e != nil || len(f.m.Converters) != 1 || f.m.Converters[0] != p {
		t.Fatal("typed permission not preserved", e)
	}
	task := f.create(t, "converter-task")
	a, e := actionauth.GetTaskAssignment(f.ctx, f.pool, task.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	check := func(expected actionauth.ConverterPermission) bool {
		tx, e := f.pool.Begin(f.ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(f.ctx)
		var now time.Time
		if e = tx.QueryRow(f.ctx, `SELECT clock_timestamp()`).Scan(&now); e != nil {
			t.Fatal(e)
		}
		current, _, e := actionauth.CheckAssignmentTx(f.ctx, tx, a, now)
		return e == nil && actionauth.AllowsConverterAttachment(current, expected, now)
	}
	if !check(p) {
		t.Fatal("current exact converter permission denied")
	}
	changed := p
	changed.Class = "shell"
	if check(changed) {
		t.Fatal("new process authority admitted")
	}
	changed = p
	changed.InputBytes++
	if check(changed) {
		t.Fatal("larger resource limit admitted")
	}
	changed = p
	changed.RuntimePin = strings.Repeat("d", 64)
	if check(changed) {
		t.Fatal("another executor admitted")
	}
	revoke := f.m
	revoke.Status = "revoked"
	if _, e = f.manager.PutMandate(f.actor("revoke-converter-permission"), goal.MandateRequest{Mandate: revoke, ExpectedRevision: revoke.Revision}); e != nil {
		t.Fatal(e)
	}
	if check(p) {
		t.Fatal("saved permission survived current owner revocation")
	}
}
