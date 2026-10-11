package acceptance

import (
	"errors"
	"reflect"
)

// ComparisonRun is supplied by an independently controlled observation runner.
// It is not inferred from a model's final answer. Zero spend is invalid without
// explicit local_no_billing pricing; locality remains a separate transport proof.
type ComparisonRun struct {
	Mode            string            `json:"mode"`
	CodeSHA256      string            `json:"code_sha256"`
	TasksSHA256     string            `json:"tasks_sha256"`
	ResourcesSHA256 string            `json:"resources_sha256"`
	Model           ModelPin          `json:"model"`
	Pricing         string            `json:"pricing"`
	Outcomes        []TaskMeasurement `json:"outcomes"`
}

type TaskMeasurement struct {
	ID               string `json:"id"`
	VerifiedSuccess  bool   `json:"verified_success"`
	MissedImportant  int64  `json:"missed_important"`
	FalseInitiatives int64  `json:"false_initiatives"`
	PermissionErrors int64  `json:"permission_errors"`
	RepeatedEffects  int64  `json:"repeated_effects"`
	SourceErrors     int64  `json:"source_errors"`
	Milliseconds     int64  `json:"milliseconds"`
	ModelRequests    int64  `json:"model_requests"`
	InputTokens      int64  `json:"input_tokens"`
	OutputTokens     int64  `json:"output_tokens"`
	ToolCalls        int64  `json:"tool_calls"`
	ChildTasks       int64  `json:"child_tasks"`
	CostMicrounits   int64  `json:"cost_microunits"`
	MemoryPeakBytes  int64  `json:"memory_peak_bytes"`
}

type Comparison struct {
	Status string        `json:"status"`
	Single ComparisonRun `json:"single"`
	Multi  ComparisonRun `json:"multi"`
	Reason string        `json:"reason"`
}

func Compare(single, multi ComparisonRun) (Comparison, error) {
	c := Comparison{Status: "OBSERVATION_ONLY", Single: single, Multi: multi, Reason: "All child/model/tool costs retained; no automatic permission, model promotion or causal claim from sample size."}
	if single.Mode != "single" || multi.Mode != "multi" || single.CodeSHA256 != multi.CodeSHA256 || single.TasksSHA256 != multi.TasksSHA256 || single.ResourcesSHA256 != multi.ResourcesSHA256 || !reflect.DeepEqual(single.Model, multi.Model) || single.Pricing != multi.Pricing {
		return c, errors.New("single/multi comparison changed code, tasks, weights/runtime/hardware, resource allowance or pricing")
	}
	for _, run := range []ComparisonRun{single, multi} {
		if !validSHA(run.CodeSHA256) || !validSHA(run.TasksSHA256) || !validSHA(run.ResourcesSHA256) || run.Model.Mode != "observation_only" || !validSHA(run.Model.WeightsSHA256) || !validSHA(run.Model.HardwareSHA256) || run.Model.Model == "" || run.Model.Runtime == "" || !validSHA(run.Model.CorpusDigest) || len(run.Outcomes) == 0 || (run.Pricing != "metered" && run.Pricing != "local_no_billing") {
			return c, errors.New("missing comparison pins/measurements")
		}
		seen := map[string]bool{}
		for _, m := range run.Outcomes {
			if m.ID == "" || seen[m.ID] {
				return c, errors.New("missing/duplicate task measurement")
			}
			seen[m.ID] = true
			for _, n := range []int64{m.MissedImportant, m.FalseInitiatives, m.PermissionErrors, m.RepeatedEffects, m.SourceErrors, m.Milliseconds, m.ModelRequests, m.InputTokens, m.OutputTokens, m.ToolCalls, m.ChildTasks, m.CostMicrounits, m.MemoryPeakBytes} {
				if n < 0 {
					return c, errors.New("negative measurement")
				}
			}
			if m.ModelRequests == 0 || m.MemoryPeakBytes == 0 || m.Milliseconds == 0 || (run.Pricing == "metered" && m.CostMicrounits == 0) {
				return c, errors.New("missing actual resource/cost measurement")
			}
			if run.Mode == "single" && m.ChildTasks != 0 {
				return c, errors.New("single baseline contains delegated work")
			}
		}
	}
	ids := map[string]bool{}
	for _, m := range single.Outcomes {
		ids[m.ID] = true
	}
	if len(single.Outcomes) != len(multi.Outcomes) {
		return c, errors.New("incomplete task pairing")
	}
	for _, m := range multi.Outcomes {
		if !ids[m.ID] {
			return c, errors.New("foreign comparison task")
		}
	}
	return c, nil
}
