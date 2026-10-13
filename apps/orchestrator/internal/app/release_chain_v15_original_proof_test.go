package app

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
)

func releaseChainV15TaskScopedProof(f *developerFixture) bool {
	return releaseChainV12RepeatedReads(f) && f.modelConfig.TaskScopedOriginalProof
}

// A promoted task legitimately spans admission, candidate-return and activated
// catalog runs. Preserve the actual conversion final as owner-verification run;
// correlate earlier project receipts to their own completed current-task runs.
func releaseChainV15OriginalProof(ctx context.Context, f *developerFixture, current task.Record) (artifact, patchRun, patchCall, checkRun, checkCall string, err error) {
	deny := func() (string, string, string, string, string, error) {
		return "", "", "", "", "", errNativeFixtureDenied
	}
	if !releaseChainV15TaskScopedProof(f) || current.TaskID != f.f.parent.TaskID || current.OwnerID != f.f.parent.OwnerID || current.SessionKey != f.f.parent.SessionKey || current.Status != "active" {
		return deny()
	}
	_, receipt, e := releaseChainV12CaptureOriginalResult(ctx, f, "normal_sent")
	if e != nil || receipt == nil || receipt.ArtifactID == nil || receipt.PatchCall == nil || len(receipt.CheckInventory) == 0 || len(receipt.InspectInventory) == 0 {
		return deny()
	}
	// Capture already requires one exact immutable prepared patch and agreement
	// of every actual check/inspection. Resolve the sole prepare's run explicitly.
	rows, e := f.f.pool.Query(ctx, `SELECT c.run_id FROM tool_calls c JOIN runs r ON r.run_id=c.run_id WHERE r.task_id=$1 AND c.tool_name='extension.project.prepare-patch' AND c.tool_call_id=$2`, current.TaskID, *receipt.PatchCall)
	if e != nil {
		return deny()
	}
	count := 0
	for rows.Next() {
		if rows.Scan(&patchRun) != nil {
			rows.Close()
			return deny()
		}
		count++
	}
	e = rows.Err()
	rows.Close()
	if e != nil || count != 1 {
		return deny()
	}
	verify := func(run, call, tool string) bool {
		var binding []byte
		e := f.f.pool.QueryRow(ctx, `SELECT i.binding FROM tool_calls c JOIN runs r ON r.run_id=c.run_id JOIN tasks t ON t.task_id=r.task_id JOIN action_intents i ON i.run_id=c.run_id AND i.tool_call_id=c.tool_call_id AND i.binding->>'tool_name'=c.tool_name JOIN action_effects e ON e.action_id=i.effect_id JOIN action_authorizations a ON a.action_id=i.action_id JOIN action_effect_dispatches d ON d.authorization_id=a.authorization_id AND d.execution_action_id=i.action_id AND d.action_id=i.effect_id WHERE c.run_id=$1 AND c.tool_call_id=$2 AND c.tool_name=$3 AND c.status='completed' AND r.task_id=$4 AND r.session_key=$5 AND r.task_revision=$6 AND r.current_state='completed' AND t.owner_id=$7 AND t.status='active' AND t.revision=$6 AND t.constraints_revision=$8 AND e.state='effect_confirmed' AND e.dispatch_outcome='dispatch_completed' AND a.state='dispatch_completed' AND d.outcome='dispatch_completed' AND a.finished_at<=a.expires_at AND d.finished_at>=d.dispatched_at`, run, call, tool, current.TaskID, current.SessionKey, current.Revision, current.OwnerID, current.ConstraintsRevision).Scan(&binding)
		var b actionauth.ActionBinding
		return e == nil && json.Unmarshal(binding, &b) == nil && b.RunID == run && b.ToolCallID == call && b.ToolName == tool && b.TaskID == current.TaskID && b.OwnerID == current.OwnerID && b.TaskRevision == current.Revision && b.ConstraintsRevision == current.ConstraintsRevision
	}
	if !verify(patchRun, *receipt.PatchCall, "extension.project.prepare-patch") {
		return deny()
	}
	for _, r := range receipt.CheckInventory {
		if !verify(r.RunID, r.ToolCall, "extension.project.check") {
			return deny()
		}
	}
	for _, r := range receipt.InspectInventory {
		if !verify(r.RunID, r.ToolCall, "extension.project.inspect") {
			return deny()
		}
	}
	chosen := receipt.CheckInventory[0]
	return *receipt.ArtifactID, patchRun, *receipt.PatchCall, chosen.RunID, chosen.ToolCall, nil
}
