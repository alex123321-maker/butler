package task

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	ControlAmend  = "amend"
	ControlStop   = "stop"
	ControlPause  = "pause"
	ControlResume = "resume"
	ControlStatus = "status"
)

type ControlRequest struct {
	TaskID             string    `json:"task_id"`
	ExpectedRevision   int64     `json:"expected_revision"`
	Kind               string    `json:"kind"`
	Instruction        string    `json:"instruction"`
	Goal               *string   `json:"goal,omitempty"`
	Constraints        *[]string `json:"constraints,omitempty"`
	CompletionCriteria *[]string `json:"completion_criteria,omitempty"`
	ExecutionPolicy    string    `json:"execution_policy,omitempty"`
	Status             string    `json:"status,omitempty"`
	ResultID           string    `json:"result_id,omitempty"`
}

type ControlResult struct {
	ControlID           string    `json:"control_id"`
	Task                Record    `json:"task"`
	AffectedTaskIDs     []string  `json:"affected_task_ids"`
	InputID             string    `json:"input_id"`
	ContinuationInputID string    `json:"continuation_input_id,omitempty"`
	EffectBoundary      string    `json:"effect_boundary"`
	AppliedAt           time.Time `json:"applied_at"`
}

func ValidateControl(req ControlRequest) error {
	if req.TaskID == "" || req.ExpectedRevision < 0 || len(req.Instruction) > 64*1024 {
		return fmt.Errorf("%w: task identity, nonnegative expected revision and bounded instruction required", ErrInvalid)
	}
	switch req.Kind {
	case ControlAmend:
		if strings.TrimSpace(req.Instruction) == "" && req.Goal == nil && req.Constraints == nil && req.CompletionCriteria == nil && req.ExecutionPolicy == "" {
			return fmt.Errorf("%w: explicit amendment required", ErrInvalid)
		}
	case ControlStop, ControlPause, ControlResume:
	case ControlStatus:
		if req.Status == "" {
			return ErrInvalid
		}
	default:
		return fmt.Errorf("%w: unsupported owner control", ErrInvalid)
	}
	if req.Kind != ControlAmend && (req.Goal != nil || req.Constraints != nil || req.CompletionCriteria != nil || req.ExecutionPolicy != "") {
		return ErrInvalid
	}
	if req.ExecutionPolicy != "" && req.ExecutionPolicy != PolicyNormal && req.ExecutionPolicy != PolicyPrepareOnly {
		return ErrInvalid
	}
	if req.Goal != nil && strings.TrimSpace(*req.Goal) == "" {
		return ErrInvalid
	}
	if req.Constraints != nil {
		if err := validateStrings(*req.Constraints); err != nil {
			return err
		}
	}
	if req.CompletionCriteria != nil {
		if len(*req.CompletionCriteria) == 0 {
			return ErrInvalid
		}
		if err := validateStrings(*req.CompletionCriteria); err != nil {
			return err
		}
	}
	return nil
}

// ControlFingerprint compares complete delivery content, not just an instruction.
func ControlFingerprint(req ControlRequest) string {
	encoded, _ := json.Marshal(req)
	return string(encoded)
}

// ClassifyOwnerText only recognizes unambiguous restrictive controls. Other text
// remains conversation unless its adapter provides an explicit typed intent.
func ClassifyOwnerText(text string) (kind, policy string) {
	text = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(text)), " "))
	command := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(text, "стоп,"), "stop,"))
	prepare := strings.HasPrefix(command, "только подготовь") || strings.HasPrefix(command, "только черновик") || strings.HasPrefix(command, "не отправляй") || strings.HasPrefix(command, "prepare only") || strings.HasPrefix(command, "draft only")
	stop := text == "стоп" || text == "stop" || text == "/stop" || text == "отмени" || text == "cancel" || strings.HasPrefix(text, "стоп,") || strings.HasPrefix(text, "stop,")
	if prepare {
		return ControlAmend, PolicyPrepareOnly
	}
	if stop {
		return ControlStop, ""
	}
	if text == "пауза" || text == "/pause" || text == "pause" {
		return ControlPause, ""
	}
	if text == "продолжи" || text == "/resume" || text == "resume" {
		return ControlResume, ""
	}
	return "", ""
}
