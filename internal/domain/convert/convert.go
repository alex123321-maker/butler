// Package convert provides bidirectional conversions between Butler domain
// types (string enums in package domain) and their proto-generated
// equivalents (int32 enums in package commonv1).
//
// All domain↔proto mapping for RunState, AutonomyMode and ErrorClass lives
// here so that callers never need hand-rolled switch statements.
package convert

import (
	"fmt"

	"github.com/butler/butler/internal/domain"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
)

// ---------------------------------------------------------------------------
// RunState
// ---------------------------------------------------------------------------

var (
	runStateDomainToProto = map[domain.RunState]commonv1.RunState{
		domain.RunStateCreated:             commonv1.RunState_RUN_STATE_CREATED,
		domain.RunStateQueued:              commonv1.RunState_RUN_STATE_QUEUED,
		domain.RunStateAcquired:            commonv1.RunState_RUN_STATE_ACQUIRED,
		domain.RunStatePreparing:           commonv1.RunState_RUN_STATE_PREPARING,
		domain.RunStateModelRunning:        commonv1.RunState_RUN_STATE_MODEL_RUNNING,
		domain.RunStateToolPending:         commonv1.RunState_RUN_STATE_TOOL_PENDING,
		domain.RunStateAwaitingApproval:    commonv1.RunState_RUN_STATE_AWAITING_APPROVAL,
		domain.RunStateToolRunning:         commonv1.RunState_RUN_STATE_TOOL_RUNNING,
		domain.RunStateAwaitingModelResume: commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME,
		domain.RunStateFinalizing:          commonv1.RunState_RUN_STATE_FINALIZING,
		domain.RunStateCompleted:           commonv1.RunState_RUN_STATE_COMPLETED,
		domain.RunStateFailed:              commonv1.RunState_RUN_STATE_FAILED,
		domain.RunStateCancelled:           commonv1.RunState_RUN_STATE_CANCELLED,
		domain.RunStateTimedOut:            commonv1.RunState_RUN_STATE_TIMED_OUT,
	}

	runStateProtoToDomain = map[commonv1.RunState]domain.RunState{
		commonv1.RunState_RUN_STATE_CREATED:               domain.RunStateCreated,
		commonv1.RunState_RUN_STATE_QUEUED:                domain.RunStateQueued,
		commonv1.RunState_RUN_STATE_ACQUIRED:              domain.RunStateAcquired,
		commonv1.RunState_RUN_STATE_PREPARING:             domain.RunStatePreparing,
		commonv1.RunState_RUN_STATE_MODEL_RUNNING:         domain.RunStateModelRunning,
		commonv1.RunState_RUN_STATE_TOOL_PENDING:          domain.RunStateToolPending,
		commonv1.RunState_RUN_STATE_AWAITING_APPROVAL:     domain.RunStateAwaitingApproval,
		commonv1.RunState_RUN_STATE_TOOL_RUNNING:          domain.RunStateToolRunning,
		commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME: domain.RunStateAwaitingModelResume,
		commonv1.RunState_RUN_STATE_FINALIZING:            domain.RunStateFinalizing,
		commonv1.RunState_RUN_STATE_COMPLETED:             domain.RunStateCompleted,
		commonv1.RunState_RUN_STATE_FAILED:                domain.RunStateFailed,
		commonv1.RunState_RUN_STATE_CANCELLED:             domain.RunStateCancelled,
		commonv1.RunState_RUN_STATE_TIMED_OUT:             domain.RunStateTimedOut,
	}
)

// RunStateToProto converts a domain RunState to its proto equivalent.
// Returns RUN_STATE_UNSPECIFIED for unrecognised values.
func RunStateToProto(state domain.RunState) commonv1.RunState {
	if v, ok := runStateDomainToProto[state]; ok {
		return v
	}
	return commonv1.RunState_RUN_STATE_UNSPECIFIED
}

// RunStateStringToProto is a convenience wrapper that accepts a raw string.
func RunStateStringToProto(state string) commonv1.RunState {
	return RunStateToProto(domain.RunState(state))
}

// ProtoToRunState converts a proto RunState to its domain equivalent.
// Returns an error for UNSPECIFIED or unrecognised values.
func ProtoToRunState(state commonv1.RunState) (domain.RunState, error) {
	if v, ok := runStateProtoToDomain[state]; ok {
		return v, nil
	}
	return "", fmt.Errorf("unrecognised proto RunState %d", state)
}

// ---------------------------------------------------------------------------
// AutonomyMode
// ---------------------------------------------------------------------------

var (
	autonomyModeDomainToProto = map[domain.AutonomyMode]commonv1.AutonomyMode{
		domain.AutonomyMode0: commonv1.AutonomyMode_AUTONOMY_MODE_0,
		domain.AutonomyMode1: commonv1.AutonomyMode_AUTONOMY_MODE_1,
		domain.AutonomyMode2: commonv1.AutonomyMode_AUTONOMY_MODE_2,
		domain.AutonomyMode3: commonv1.AutonomyMode_AUTONOMY_MODE_3,
	}

	autonomyModeProtoToDomain = map[commonv1.AutonomyMode]domain.AutonomyMode{
		commonv1.AutonomyMode_AUTONOMY_MODE_0: domain.AutonomyMode0,
		commonv1.AutonomyMode_AUTONOMY_MODE_1: domain.AutonomyMode1,
		commonv1.AutonomyMode_AUTONOMY_MODE_2: domain.AutonomyMode2,
		commonv1.AutonomyMode_AUTONOMY_MODE_3: domain.AutonomyMode3,
	}
)

// AutonomyModeToProto converts a domain AutonomyMode to its proto equivalent.
// Returns AUTONOMY_MODE_UNSPECIFIED for unrecognised values.
func AutonomyModeToProto(mode domain.AutonomyMode) commonv1.AutonomyMode {
	if v, ok := autonomyModeDomainToProto[mode]; ok {
		return v
	}
	return commonv1.AutonomyMode_AUTONOMY_MODE_UNSPECIFIED
}

// AutonomyModeStringToProto is a convenience wrapper that accepts a raw string.
func AutonomyModeStringToProto(mode string) commonv1.AutonomyMode {
	return AutonomyModeToProto(domain.AutonomyMode(mode))
}

// ProtoToAutonomyMode converts a proto AutonomyMode to its domain equivalent.
// Returns an error for UNSPECIFIED or unrecognised values.
func ProtoToAutonomyMode(mode commonv1.AutonomyMode) (domain.AutonomyMode, error) {
	if v, ok := autonomyModeProtoToDomain[mode]; ok {
		return v, nil
	}
	return "", fmt.Errorf("unrecognised proto AutonomyMode %d", mode)
}

// ProtoToAutonomyModeString is a convenience wrapper returning a plain string.
func ProtoToAutonomyModeString(mode commonv1.AutonomyMode) (string, error) {
	v, err := ProtoToAutonomyMode(mode)
	return string(v), err
}

// ---------------------------------------------------------------------------
// ErrorClass
// ---------------------------------------------------------------------------

var (
	errorClassDomainToProto = map[domain.ErrorClass]commonv1.ErrorClass{
		domain.ErrorClassValidation: commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR,
		domain.ErrorClassTransport:  commonv1.ErrorClass_ERROR_CLASS_TRANSPORT_ERROR,
		domain.ErrorClassTool:       commonv1.ErrorClass_ERROR_CLASS_TOOL_ERROR,
		domain.ErrorClassPolicy:     commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED,
		domain.ErrorClassCredential: commonv1.ErrorClass_ERROR_CLASS_CREDENTIAL_ERROR,
		domain.ErrorClassApproval:   commonv1.ErrorClass_ERROR_CLASS_APPROVAL_ERROR,
		domain.ErrorClassTimeout:    commonv1.ErrorClass_ERROR_CLASS_TIMEOUT,
		domain.ErrorClassCancelled:  commonv1.ErrorClass_ERROR_CLASS_CANCELLED,
		domain.ErrorClassInternal:   commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR,
	}

	errorClassProtoToDomain = map[commonv1.ErrorClass]domain.ErrorClass{
		commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR: domain.ErrorClassValidation,
		commonv1.ErrorClass_ERROR_CLASS_TRANSPORT_ERROR:  domain.ErrorClassTransport,
		commonv1.ErrorClass_ERROR_CLASS_TOOL_ERROR:       domain.ErrorClassTool,
		commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED:    domain.ErrorClassPolicy,
		commonv1.ErrorClass_ERROR_CLASS_CREDENTIAL_ERROR: domain.ErrorClassCredential,
		commonv1.ErrorClass_ERROR_CLASS_APPROVAL_ERROR:   domain.ErrorClassApproval,
		commonv1.ErrorClass_ERROR_CLASS_TIMEOUT:          domain.ErrorClassTimeout,
		commonv1.ErrorClass_ERROR_CLASS_CANCELLED:        domain.ErrorClassCancelled,
		commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR:   domain.ErrorClassInternal,
	}
)

// ErrorClassToProto converts a domain ErrorClass to its proto equivalent.
// Returns ERROR_CLASS_INTERNAL_ERROR for unrecognised values (safe default).
func ErrorClassToProto(ec domain.ErrorClass) commonv1.ErrorClass {
	if v, ok := errorClassDomainToProto[ec]; ok {
		return v
	}
	return commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR
}

// ErrorClassStringToProto is a convenience wrapper that accepts a raw string.
// Returns ERROR_CLASS_UNSPECIFIED for empty strings, ERROR_CLASS_INTERNAL_ERROR
// for unrecognised non-empty values.
func ErrorClassStringToProto(ec string) commonv1.ErrorClass {
	if ec == "" {
		return commonv1.ErrorClass_ERROR_CLASS_UNSPECIFIED
	}
	return ErrorClassToProto(domain.ErrorClass(ec))
}

// ProtoToErrorClass converts a proto ErrorClass to its domain equivalent.
// Returns an error for UNSPECIFIED or unrecognised values.
func ProtoToErrorClass(ec commonv1.ErrorClass) (domain.ErrorClass, error) {
	if v, ok := errorClassProtoToDomain[ec]; ok {
		return v, nil
	}
	return "", fmt.Errorf("unrecognised proto ErrorClass %d", ec)
}

// ProtoToErrorClassString is a convenience wrapper returning a plain string.
func ProtoToErrorClassString(ec commonv1.ErrorClass) (string, error) {
	v, err := ProtoToErrorClass(ec)
	return string(v), err
}
