package domain

import "fmt"

type RunID string
type SessionKey string
type ToolCallID string
type EventID string
type LeaseID string
type CredentialAlias string

type RunState string

const (
	RunStateCreated             RunState = "created"
	RunStateQueued              RunState = "queued"
	RunStateAcquired            RunState = "acquired"
	RunStatePreparing           RunState = "preparing"
	RunStateModelRunning        RunState = "model_running"
	RunStateToolPending         RunState = "tool_pending"
	RunStateAwaitingApproval    RunState = "awaiting_approval"
	RunStateToolRunning         RunState = "tool_running"
	RunStateAwaitingModelResume RunState = "awaiting_model_resume"
	RunStateFinalizing          RunState = "finalizing"
	RunStateCompleted           RunState = "completed"
	RunStateFailed              RunState = "failed"
	RunStateCancelled           RunState = "cancelled"
	RunStateTimedOut            RunState = "timed_out"
)

type ErrorClass string

const (
	ErrorClassValidation ErrorClass = "validation_error"
	ErrorClassTransport  ErrorClass = "transport_error"
	ErrorClassTool       ErrorClass = "tool_error"
	ErrorClassPolicy     ErrorClass = "policy_denied"
	ErrorClassCredential ErrorClass = "credential_error"
	ErrorClassApproval   ErrorClass = "approval_error"
	ErrorClassTimeout    ErrorClass = "timeout"
	ErrorClassCancelled  ErrorClass = "cancelled"
	ErrorClassInternal   ErrorClass = "internal_error"
)

type InputEventType string

const (
	InputEventTypeUserMessage             InputEventType = "user_message"
	InputEventTypeUIAction                InputEventType = "ui_action"
	InputEventTypeSystemDiagnosticTrigger InputEventType = "system_diagnostic_trigger"
	InputEventTypeScheduledInternal       InputEventType = "scheduled_internal_event"
	InputEventTypeResumeOrRetry           InputEventType = "resume_or_retry_event"
	InputEventTypeApprovalResponse        InputEventType = "approval_response_event"
)

type AutonomyMode string

const (
	AutonomyMode0 AutonomyMode = "mode_0"
	AutonomyMode1 AutonomyMode = "mode_1"
	AutonomyMode2 AutonomyMode = "mode_2"
	AutonomyMode3 AutonomyMode = "mode_3"
)

var allowedTransitions = map[RunState]map[RunState]struct{}{
	RunStateCreated: {
		RunStateQueued:    {},
		RunStateCancelled: {},
		RunStateTimedOut:  {},
	},
	RunStateQueued: {
		RunStateAcquired:  {},
		RunStateCancelled: {},
		RunStateTimedOut:  {},
	},
	RunStateAcquired: {
		RunStatePreparing: {},
		RunStateCancelled: {},
		RunStateTimedOut:  {},
	},
	RunStatePreparing: {
		RunStateModelRunning: {},
		RunStateFailed:       {},
		RunStateCancelled:    {},
		RunStateTimedOut:     {},
	},
	RunStateModelRunning: {
		RunStateToolPending: {},
		RunStateFinalizing:  {},
		RunStateFailed:      {},
		RunStateCancelled:   {},
		RunStateTimedOut:    {},
	},
	RunStateToolPending: {
		RunStateAwaitingApproval: {},
		RunStateToolRunning:      {},
		RunStateCancelled:        {},
		RunStateTimedOut:         {},
	},
	RunStateAwaitingApproval: {
		RunStateToolRunning: {},
		RunStateCancelled:   {},
		RunStateTimedOut:    {},
		RunStateFailed:      {},
	},
	RunStateToolRunning: {
		RunStateAwaitingModelResume: {},
		RunStateFailed:              {},
		RunStateCancelled:           {},
		RunStateTimedOut:            {},
	},
	RunStateAwaitingModelResume: {
		RunStateModelRunning: {},
		RunStateCancelled:    {},
		RunStateTimedOut:     {},
	},
	RunStateFinalizing: {
		RunStateCompleted: {},
		RunStateCancelled: {},
		RunStateTimedOut:  {},
	},
	RunStateCompleted: {},
	RunStateFailed:    {},
	RunStateCancelled: {},
	RunStateTimedOut:  {},
}

func ValidateRunStateTransition(from, to RunState) error {
	if from == to {
		return fmt.Errorf("run state transition %q -> %q is not allowed", from, to)
	}
	transitions, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("unknown source run state %q", from)
	}
	if _, ok := allowedTransitions[to]; !ok {
		return fmt.Errorf("unknown target run state %q", to)
	}
	if _, ok := transitions[to]; !ok {
		return fmt.Errorf("run state transition %q -> %q is not allowed", from, to)
	}
	return nil
}

func IsTerminalRunState(state RunState) bool {
	switch state {
	case RunStateCompleted, RunStateFailed, RunStateCancelled, RunStateTimedOut:
		return true
	default:
		return false
	}
}
