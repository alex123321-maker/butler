// Package task owns persistent user work, independently of model runs.
package task

import (
	"context"
	"errors"
	"fmt"
	"github.com/butler/butler/internal/actionauth"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("task not found")
	ErrConflict = errors.New("task revision or state conflict")
	ErrInvalid  = errors.New("invalid task request")
)

type WorkingState struct {
	Plan           []string       `json:"plan"`
	PendingActions []string       `json:"pending_actions"`
	Scratch        map[string]any `json:"scratch"`
}

type Record struct {
	TaskID              string         `json:"task_id"`
	SessionKey          string         `json:"session_key"`
	OwnerID             string         `json:"owner_id"`
	ParentTaskID        string         `json:"parent_task_id,omitempty"`
	Goal                string         `json:"goal"`
	Source              map[string]any `json:"source"`
	Status              string         `json:"status"`
	Revision            int64          `json:"revision"`
	ConstraintsRevision int64          `json:"constraints_revision"`
	ExecutionPolicy     string         `json:"execution_policy"`
	Constraints         []string       `json:"constraints"`
	CompletionCriteria  []string       `json:"completion_criteria"`
	Working             WorkingState   `json:"working"`
	WorkingRevision     int64          `json:"working_revision"`
	LastRunID           string         `json:"last_run_id,omitempty"`
	CompletionResultID  string         `json:"completion_result_id,omitempty"`
	Legacy              bool           `json:"legacy"`
	ResultDelivery      string         `json:"result_delivery"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	FinishedAt          *time.Time     `json:"finished_at,omitempty"`
}

type CreateRequest struct {
	// Only trusted owner/source configuration chooses a root mode. Delegation
	// inherits its parent's mode; model-facing proposals have no selector.
	ResultDelivery         string                 `json:"-"`
	InitialExecutionPolicy string                 `json:"-"`
	Authority              *actionauth.Assignment `json:"-"`
	SessionKey             string                 `json:"session_key"`
	OwnerID                string                 `json:"owner_id"`
	ParentTaskID           string                 `json:"parent_task_id,omitempty"`
	Goal                   string                 `json:"goal"`
	Source                 map[string]any         `json:"source"`
	Constraints            []string               `json:"constraints"`
	CompletionCriteria     []string               `json:"completion_criteria"`
}

type Result struct {
	ResultID         string    `json:"result_id"`
	TaskID           string    `json:"task_id"`
	TaskRevision     int64     `json:"task_revision"`
	RunID            string    `json:"run_id,omitempty"`
	Summary          string    `json:"summary"`
	EvidenceRefs     []string  `json:"evidence_refs"`
	VerifiedCriteria []string  `json:"verified_criteria"`
	Verdict          string    `json:"verdict"`
	Verifier         string    `json:"verifier"`
	CreatedAt        time.Time `json:"created_at"`
}

// Manager is the orchestration boundary. Run outcomes can save an unverified
// result but cannot mark a task completed or rewrite its acceptance criteria.
type Manager interface {
	Get(context.Context, string) (Record, error)
	GetByRun(context.Context, string) (Record, error)
	AttachRun(context.Context, string, int64, string, string) error
	SaveWorking(context.Context, string, int64, WorkingState) (Record, error)
	CheckpointRun(context.Context, string, int64, WorkingState) (Record, error)
	SaveRunOutcome(context.Context, string, string, string) error
}

func ValidateCreate(req CreateRequest) error {
	if req.ResultDelivery != "" && req.ResultDelivery != "attempt" && req.ResultDelivery != "verified_task" {
		return ErrInvalid
	}
	if err := ValidatePolicy(req.InitialExecutionPolicy); err != nil {
		return err
	}
	if strings.TrimSpace(req.SessionKey) == "" || strings.TrimSpace(req.OwnerID) == "" || strings.TrimSpace(req.Goal) == "" {
		return fmt.Errorf("%w: session_key, owner_id and goal are required", ErrInvalid)
	}
	if len(req.CompletionCriteria) == 0 {
		return fmt.Errorf("%w: explicit completion criteria are required", ErrInvalid)
	}
	return validateStrings(req.CompletionCriteria)
}

func validateStrings(values []string) error {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			return fmt.Errorf("%w: values must be nonempty and distinct", ErrInvalid)
		}
		seen[value] = true
	}
	return nil
}

func ValidateTransition(from, to string) error {
	if from == "completed" || from == "cancelled" || from == to {
		return ErrConflict
	}
	switch to {
	case "active", "waiting_for_user", "blocked", "paused", "cancelled", "completed":
		return nil
	default:
		return fmt.Errorf("%w: unsupported task status", ErrInvalid)
	}
}

func ValidateVerification(record Record, result Result) error {
	if result.TaskRevision != record.Revision || strings.TrimSpace(result.Summary) == "" || len(result.EvidenceRefs) == 0 || result.Verifier != "owner" {
		return fmt.Errorf("%w: verification requires current revision, saved result, evidence and owner verdict", ErrInvalid)
	}
	if err := validateStrings(result.EvidenceRefs); err != nil {
		return err
	}
	if err := validateStrings(result.VerifiedCriteria); err != nil {
		return err
	}
	if len(record.CompletionCriteria) == 0 || len(result.VerifiedCriteria) != len(record.CompletionCriteria) {
		return ErrInvalid
	}
	for _, criterion := range record.CompletionCriteria {
		found := false
		for _, checked := range result.VerifiedCriteria {
			if criterion == checked {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: not all completion criteria verified", ErrInvalid)
		}
	}
	return nil
}
