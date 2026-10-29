package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
)

// Post-exit projection of original facts, not a replacement quality oracle.
// No proposal/result is repaired, accepted as memory, or sent to a model.
func TestLaba115ABCPostExitProjection(t *testing.T) {
	profile := os.Getenv("BUTLER_ISSUE115_LABA_ABC_REVIEW")
	if profile != "closed-abc-v3" && profile != "closed-abc-v4" && profile != "closed-abc-v5" && profile != "closed-abc-v6" && profile != "guided-full-v1" && profile != "guided-full-v3" {
		t.Skip("explicit closed ABC33 raw exports required")
	}
	read := func(name string, v any) bool {
		t.Helper()
		p := filepath.Join("/review", name)
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			return false
		}
		h, e := experienceNativeFileSHA(p)
		if os.IsNotExist(e) {
			return false
		}
		if e != nil {
			t.Fatal(name, e)
		}
		b, e := closedExperienceRead("/review", name, h, 64*1024*1024)
		if e != nil || json.Unmarshal(b, v) != nil {
			t.Fatal("invalid bounded original export", name)
		}
		return true
	}
	var window struct {
		Slots   []experienceNativeSlot
		Entries []experienceNativeEntry
	}
	if !read("actual/window-final.json", &window) || len(window.Slots) != 57 {
		t.Fatal("closed full57 window required")
	}
	var accounting map[string]json.RawMessage
	b, e := os.ReadFile("/review-output/cost-review.json")
	if e != nil || json.Unmarshal(b, &accounting) != nil {
		t.Fatal("original cost validators must pass first")
	}
	var accountingPlan string
	_ = json.Unmarshal(accounting["plan_sha256"], &accountingPlan)
	wantPlan := "2b17c33e79f9c2b72b71d5fc3363d40c85f67b4aae89c158c9b0a0fecf54eae3"
	if profile == "closed-abc-v4" {
		wantPlan = "18d4ec76d5cbbbf241d7151eb542e7a48d7125c7ea4452ac8c67e0d148d729a2"
	}
	if profile == "closed-abc-v5" {
		wantPlan = "e9a22fdb7f4125e84eef442520f41cae42ce291234343f2eeccd1b129a127497"
	}
	if profile == "closed-abc-v6" {
		wantPlan = "8ce5c9aeb47a5837f18a572d45b19e7714be2871122c32ffc8acf2652f65a511"
	}
	limit := 33
	if profile == "guided-full-v1" {
		wantPlan = "30acfb4f8ae5260a7e3675822247672d3dde178057d7292d54b0166200ab893c"
		limit = 57
	}
	if profile == "guided-full-v3" {
		wantPlan = "0d957c32b47ac93fcc6f21826af8d54c645cf15b5347aed72dc98d653dd87ac6"
		limit = 57
	}
	if accountingPlan != wantPlan {
		t.Fatal("different cohort accounting")
	}
	type cost struct {
		Entries      int   `json:"entries"`
		Unknown      int   `json:"unknown_usage"`
		MissingTotal int   `json:"missing_total"`
		KnownTokens  int64 `json:"known_full_tokens"`
		Input        int64 `json:"input_tokens"`
		Output       int64 `json:"output_tokens"`
		ModelMillis  int64 `json:"model_millis"`
	}
	bySlot := map[string]*cost{}
	groups := map[string]*cost{}
	add := func(c *cost, entry experienceNativeEntry) {
		c.Entries++
		if entry.Usage == nil {
			c.Unknown++
		} else {
			c.Input += int64(entry.Usage.InputTokens)
			c.Output += int64(entry.Usage.OutputTokens)
			if entry.Usage.TotalTokens == 0 {
				c.MissingTotal++
				c.KnownTokens += int64(entry.Usage.InputTokens + entry.Usage.OutputTokens)
			} else {
				c.KnownTokens += int64(entry.Usage.TotalTokens)
			}
		}
		if entry.Millis != nil {
			c.ModelMillis += *entry.Millis
		}
	}
	slots := map[string]experienceNativeSlot{}
	for _, s := range window.Slots {
		slots[s.ID] = s
	}
	for _, entry := range window.Entries {
		s, ok := slots[entry.Slot]
		if !ok {
			t.Fatal("entry lacks slot")
		}
		g := s.Kind
		if s.Kind == "control" {
			g = s.Arm
		}
		if groups[g] == nil {
			groups[g] = &cost{}
		}
		if bySlot[s.ID] == nil {
			bySlot[s.ID] = &cost{}
		}
		add(groups[g], entry)
		add(bySlot[s.ID], entry)
	}
	rows := []map[string]any{}
	complete := true
	for _, s := range window.Slots[9:limit] {
		row := map[string]any{"sequence": s.Sequence, "case": s.CaseID, "arm": s.Arm, "repetition": s.Repetition, "slot_status": s.Status, "blocker": s.Blocker, "cost": bySlot[s.ID]}
		if s.Started != nil && s.Finished != nil {
			row["elapsed_millis"] = s.Finished.Sub(*s.Started).Milliseconds()
		}
		var outcome struct {
			Task struct {
				ID          string `json:"task_id"`
				Revision    int64  `json:"revision"`
				Constraints int64  `json:"constraints_revision"`
			} `json:"task"`
			Result    *flow.ExecutionResult
			FlowError bool                     `json:"flow_error"`
			Recipient experienceRecipientState `json:"recipient_audit"`
		}
		if !read(fmt.Sprintf("actual/case-%03d-outcome.json", s.Sequence), &outcome) || outcome.Result == nil {
			complete = false
			row["owning_outcome_present"] = false
			rows = append(rows, row)
			continue
		}
		row["owning_outcome_present"] = true
		row["flow_error"] = outcome.FlowError
		row["run_state"] = outcome.Result.CurrentState.String()
		row["run_id"] = outcome.Result.RunID
		var db map[string]json.RawMessage
		if !read(fmt.Sprintf("actual/case-%03d-database.json", s.Sequence), &db) {
			t.Fatal("owning case database omitted")
		}
		var finals []struct {
			Run     string `json:"run_id"`
			Content string `json:"content"`
		}
		_ = json.Unmarshal(db["run_final_results"], &finals)
		final := ""
		for _, f := range finals {
			if f.Run == outcome.Result.RunID {
				final = f.Content
			}
		}
		row["final"] = final
		var calls []struct {
			Run      string          `json:"run_id"`
			Name     string          `json:"tool_name"`
			Status   string          `json:"status"`
			Args     json.RawMessage `json:"args"`
			Result   json.RawMessage `json:"result"`
			Error    json.RawMessage `json:"error"`
			Started  time.Time       `json:"started_at"`
			Finished *time.Time      `json:"finished_at"`
		}
		if json.Unmarshal(db["tool_calls"], &calls) != nil {
			t.Fatal("tool population missing")
		}
		tools := []map[string]any{}
		checks := map[string]bool{}
		prepare := 0
		currentCheckFailed := false
		memoryReads := []string{}
		loadedSkills := []string{}
		loadsFinished := []time.Time{}
		var firstTaskAction time.Time
		for _, call := range calls {
			if call.Run != outcome.Result.RunID {
				continue
			}
			if call.Name == "extension.project.prepare-patch" {
				prepare++
			}
			if strings.HasPrefix(call.Name, "extension.project.") && (firstTaskAction.IsZero() || call.Started.Before(firstTaskAction)) {
				firstTaskAction = call.Started
			}
			var result struct {
				Body       string `json:"body"`
				StatusCode int    `json:"status_code"`
				Items      []struct {
					Source struct {
						ID string `json:"source_id"`
					} `json:"source"`
				} `json:"items"`
				Digest string `json:"digest"`
			}
			_ = json.Unmarshal(call.Result, &result)
			if call.Status == "completed" && call.Name == "memory.open" {
				for _, item := range result.Items {
					memoryReads = append(memoryReads, item.Source.ID)
				}
			}
			if call.Status == "completed" && call.Name == "skills.load" && result.Digest != "" {
				loadedSkills = append(loadedSkills, result.Digest)
				if call.Finished != nil {
					loadsFinished = append(loadsFinished, *call.Finished)
				}
			}
			var check struct {
				ID      string `json:"artifact_id"`
				Passed  *bool  `json:"checks_passed"`
				Applied bool   `json:"applied"`
				Content string `json:"prepared_content"`
			}
			_ = json.Unmarshal([]byte(result.Body), &check)
			if call.Name == "extension.project.check" && call.Status == "completed" && check.Passed != nil {
				if check.ID != "" && *check.Passed && !check.Applied {
					checks[check.ID+"\x00"+check.Content] = true
				}
				currentCheckFailed = currentCheckFailed || !*check.Passed
			}
			tools = append(tools, map[string]any{"name": call.Name, "status": call.Status, "http_status": result.StatusCode, "error": call.Error})
		}
		artifacts := []map[string]any{}
		currentMatching := 0
		currentChecked := 0
		finalCorrelated := 0
		recipient := &experienceRecipient{state: outcome.Recipient}
		for _, entry := range outcome.Recipient.Entries {
			valid := recipient.validProposal(entry.Resource, entry.Proposal)
			checked := false
			if len(entry.Proposal.Changes) == 1 {
				checked = checks[entry.Artifact+"\x00"+entry.Proposal.Changes[0].After]
			}
			correlated := strings.Contains(final, entry.Artifact) && entry.Artifact != ""
			if valid {
				currentMatching++
			}
			if valid && checked {
				currentChecked++
			}
			if valid && checked && correlated {
				finalCorrelated++
			}
			artifacts = append(artifacts, map[string]any{"resource": entry.Resource, "artifact": entry.Artifact, "state": entry.State, "proposal_matches_current": valid, "matching_unapplied_check": checked, "final_mentions_artifact": correlated})
		}
		row["tools"] = tools
		row["prepare_calls"] = prepare
		row["recipient_dispatches"] = outcome.Recipient.Dispatches
		row["recipient_entries"] = len(outcome.Recipient.Entries)
		row["artifacts"] = artifacts
		row["current_matching_artifacts"] = currentMatching
		row["current_checked_artifacts"] = currentChecked
		row["current_checked_artifacts_in_final"] = finalCorrelated
		row["initial_check_failed_observed"] = currentCheckFailed
		row["memory_open_sources"] = memoryReads
		row["loaded_skill_digests"] = loadedSkills
		beforeAction := 0
		for _, finished := range loadsFinished {
			if !firstTaskAction.IsZero() && !finished.After(firstTaskAction) {
				beforeAction++
			}
		}
		row["loads_completed_before_first_task_action"] = beforeAction
		var usagePins []struct {
			Run                 string `json:"run_id"`
			Digest              string `json:"digest"`
			Task                string `json:"task_id"`
			TaskRevision        int64  `json:"task_revision"`
			ConstraintsRevision int64  `json:"constraints_revision"`
		}
		pinRaw, pinsPresent := db["skill_usages"]
		row["skill_usage_export_present"] = pinsPresent
		if profile == "guided-full-v3" && (!pinsPresent || json.Unmarshal(pinRaw, &usagePins) != nil) {
			t.Fatal("complete actual skill usage export required")
		}
		if profile != "guided-full-v3" {
			_ = json.Unmarshal(pinRaw, &usagePins)
		}
		pins := []map[string]any{}
		matchedLoads := 0
		for _, pin := range usagePins {
			if pin.Run != outcome.Result.RunID {
				continue
			}
			pins = append(pins, map[string]any{"digest": pin.Digest, "task_id": pin.Task, "task_revision": pin.TaskRevision, "constraints_revision": pin.ConstraintsRevision})
			for _, digest := range loadedSkills {
				if pin.Digest == digest && pin.Task == outcome.Task.ID && pin.TaskRevision == outcome.Task.Revision && pin.ConstraintsRevision == outcome.Task.Constraints {
					matchedLoads++
					break
				}
			}
		}
		row["skill_usage_pins"] = pins
		row["completed_loads_with_matching_pin"] = matchedLoads
		rows = append(rows, row)
	}
	result := map[string]any{"version": "butler115.abc-post-exit-facts.v1", "plan_sha256": accountingPlan, "selected_control_slots": limit - 9, "full_control_slots": 48, "all_owning_outcomes_present": complete, "shared_and_arm_native_costs": groups, "cases": rows, "semantic_review_required": true, "independent_quality_qualified": false, "new_native_admission": false}
	if limit == 33 {
		result["all24_owning_outcomes_present"] = complete
	} else {
		result["all48_owning_outcomes_present"] = complete
	}
	f, e := os.OpenFile("/review-output/abc-facts.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal(e)
	}
	w := json.NewEncoder(f).Encode(result)
	s := f.Sync()
	c := f.Close()
	if w != nil || s != nil || c != nil {
		t.Fatal("facts receipt persistence failed")
	}
	t.Logf("ABC_FACTS_PROJECTION cases=%d all_outcomes=%v no inference/acceptance", len(rows), complete)
}
