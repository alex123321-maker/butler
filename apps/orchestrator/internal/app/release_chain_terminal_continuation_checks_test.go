package app

import (
	"reflect"
	"testing"
)

func TestReleaseChainV18SkipsVerifiedOwnerStop(t *testing.T) {
	fixed := []string{"explicit_absence", "lost_ack", "restart_sent"}
	entry := releaseChainHighEntry{ScenarioNames: fixed, SkipVerifiedOwnerStop: true, PreserveInvalidOriginalEvidence: true, ContinueAfterCaseFailure: true}
	got, err := releaseChainHighDeclaredScenarios(entry)
	if err != nil || !reflect.DeepEqual(got, fixed) {
		t.Fatal("new pending cases differ")
	}
	for _, bad := range [][]string{{"owner_stop", "explicit_absence", "lost_ack", "restart_sent"}, {"lost_ack", "explicit_absence", "restart_sent"}, {"explicit_absence", "lost_ack"}} {
		entry.ScenarioNames = bad
		if _, err = releaseChainHighDeclaredScenarios(entry); err == nil {
			t.Fatal("verified Stop repeated or mandatory restart skipped")
		}
	}
	entry.ScenarioNames = fixed
	entry.SkipVerifiedOwnerStop = false
	if _, err = releaseChainHighDeclaredScenarios(entry); err == nil {
		t.Fatal("old four-case scope silently skipped Stop")
	}
}

func TestReleaseChainV18TerminalContinuationIntegration(t *testing.T) {
	c := newContinuationFixture(t, 0)
	ctx, _, inputID := c.claimParent(t)
	pool := c.f.f.pool
	taskID := c.f.f.parent.TaskID
	// The real failed input had this stale run state and no live inference.
	if _, err := pool.Exec(ctx, `UPDATE runs SET status='model_running',current_state='model_running' WHERE run_id='continuation-parent-worker'`); err != nil {
		t.Fatal(err)
	}
	if failed, err := releaseChainTerminalContinuation(ctx, pool, taskID); err != nil || failed {
		t.Fatal("live continuation treated as terminal", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE accepted_inputs SET status='blocked',error_code='execution_stopped' WHERE input_id=$1`, inputID); err != nil {
		t.Fatal(err)
	}
	if failed, err := releaseChainTerminalContinuation(ctx, pool, taskID); err != nil || !failed {
		t.Fatal("terminal input waited for completed receipt", err)
	}
	if failed, err := releaseChainTerminalContinuation(ctx, pool, "foreign-task"); err != nil || failed {
		t.Fatal("other task terminal leaked into current wait", err)
	}
	var state string
	if err := pool.QueryRow(ctx, `SELECT current_state FROM runs WHERE run_id='continuation-parent-worker'`).Scan(&state); err != nil || state != "model_running" {
		t.Fatal("terminal input read mutated/depended on run state", err)
	}
}
