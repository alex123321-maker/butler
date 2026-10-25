package app

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/butler/butler/internal/generatedtools"
)

type converterLocalEffect struct{ task, tool string }

// Trial slots come from the immutable admitted permission, never model output.
// Require the entire role-specific workflow, exactly one original application,
// and no additional operation. Only the already permitted trial count varies.
func converterLocalInventoryTrials(effects []converterLocalEffect, parent, child, converter string, allowance int64) (int64, error) {
	deny := errors.New("converter effect inventory violates admitted workflow")
	if parent == "" || child == "" || parent == child || allowance < 1 || allowance > 8 {
		return 0, deny
	}
	want := map[converterLocalEffect]int{
		{parent, "converters.discover"}: 1, {parent, "task.delegate_converter"}: 1,
		{parent, "converters.collect"}: 1, {parent, "converters.test"}: 1,
		{parent, "converters.activate"}: 1, {parent, "converter." + converter + ".convert"}: 1,
		{child, "converters.discover"}: 1, {child, "converters.develop"}: 1, {child, "converters.test"}: 1,
	}
	counts := map[converterLocalEffect]int{}
	trial := converterLocalEffect{parent, "converters.trial"}
	for _, effect := range effects {
		if _, ok := want[effect]; !ok && effect != trial {
			return 0, deny
		}
		counts[effect]++
	}
	for effect, count := range want {
		if counts[effect] != count {
			return 0, deny
		}
	}
	n := int64(counts[trial])
	if n < 1 || n > allowance {
		return 0, deny
	}
	return n, nil
}

func verifyConverterLocalActionInventory(t *testing.T, f *developerFixture, child string) {
	t.Helper()
	parent, owner := f.f.parent.TaskID, f.f.parent.OwnerID
	var raw []byte
	var snapshot generatedtools.DeveloperSnapshot
	if err := f.f.pool.QueryRow(f.f.ctx, `SELECT accepted_json FROM converter_developer_snapshots WHERE task_id=$1 AND parent_task_id=$2 AND owner_id=$3`, child, parent, owner).Scan(&raw); err != nil || json.Unmarshal(raw, &snapshot) != nil || snapshot.ChildTaskID != child || snapshot.Binding.TaskID != parent || snapshot.NeedHash != generatedtools.Hash(f.need) || snapshot.Permission != generatedtools.ExpectedPermission(f.need, f.profile, snapshot.Permission.Trials) {
		t.Fatal("immutable admitted converter permission unavailable")
	}
	rows, err := f.f.pool.Query(f.f.ctx, `SELECT i.binding->>'task_id',i.binding->>'tool_name' FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id JOIN runs r ON r.run_id=i.run_id WHERE r.task_id IN($1,$2) AND e.state='effect_confirmed' AND i.binding->>'owner_id'=$3 AND i.binding->>'task_id'=r.task_id AND i.binding->>'run_id'=r.run_id ORDER BY i.created_at`, parent, child, owner)
	if err != nil {
		t.Fatal(err)
	}
	var effects []converterLocalEffect
	for rows.Next() {
		var effect converterLocalEffect
		if err = rows.Scan(&effect.task, &effect.tool); err != nil {
			break
		}
		effects = append(effects, effect)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	trials, checkErr := converterLocalInventoryTrials(effects, parent, child, f.need.ConverterID, snapshot.Permission.Trials)
	if err != nil || checkErr != nil {
		t.Fatal("actual converter role/action inventory invalid", err, checkErr)
	}
	input, _ := json.Marshal(f.document)
	var slots int64
	var allPassed bool
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*),COALESCE(bool_and(status='passed' AND input_ref_json=$3::jsonb),false) FROM converter_trials WHERE owner_id=$1 AND task_id=$2`, owner, parent, input).Scan(&slots, &allPassed); err != nil || slots != trials || !allPassed {
		t.Fatal("trial slots, original input or actual fixed result invalid", slots, trials, allPassed, err)
	}
	var confirmed int
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id WHERE i.run_id IN(SELECT run_id FROM runs WHERE task_id IN($1,$2)) AND e.state='effect_confirmed'`, parent, child).Scan(&confirmed); err != nil || confirmed != len(effects) {
		t.Fatal("foreign or unmatched confirmed effect omitted", err)
	}
	t.Logf("actual role/action inventory: confirmed=%d original_applications=1 trials=%d admitted_trial_limit=%d", confirmed, trials, snapshot.Permission.Trials)
}

func TestConverterLocalInventoryRequiresRolesAndAdmittedTrialLimit(t *testing.T) {
	base := []converterLocalEffect{{"p", "converters.discover"}, {"p", "task.delegate_converter"}, {"c", "converters.discover"}, {"c", "converters.develop"}, {"c", "converters.test"}, {"p", "converters.collect"}, {"p", "converters.test"}, {"p", "converters.trial"}, {"p", "converters.activate"}, {"p", "converter.inventory.convert"}}
	for _, tc := range []struct {
		name  string
		extra []converterLocalEffect
		limit int64
		ok    bool
	}{
		{"one_slot", nil, 1, true},
		{"second_admitted_trial", []converterLocalEffect{{"p", "converters.trial"}}, 2, true},
		{"second_not_admitted", []converterLocalEffect{{"p", "converters.trial"}}, 1, false},
		{"third_not_admitted", []converterLocalEffect{{"p", "converters.trial"}, {"p", "converters.trial"}}, 2, false},
		{"duplicate_original", []converterLocalEffect{{"p", "converter.inventory.convert"}}, 2, false},
		{"child_cannot_activate", []converterLocalEffect{{"c", "converters.activate"}}, 2, false},
		{"foreign_task", []converterLocalEffect{{"foreign", "converters.trial"}}, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			effects := append(append([]converterLocalEffect{}, base...), tc.extra...)
			_, err := converterLocalInventoryTrials(effects, "p", "c", "inventory", tc.limit)
			if (err == nil) != tc.ok {
				t.Fatal(err)
			}
		})
	}
	if _, err := converterLocalInventoryTrials(base[1:], "p", "c", "inventory", 2); err == nil {
		t.Fatal("development without original discovery passed")
	}
}
