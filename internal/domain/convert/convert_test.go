package convert

import (
	"testing"

	"github.com/butler/butler/internal/domain"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
)

// ---------------------------------------------------------------------------
// RunState round-trip
// ---------------------------------------------------------------------------

func TestRunStateRoundTrip(t *testing.T) {
	pairs := []struct {
		domain domain.RunState
		proto  commonv1.RunState
	}{
		{domain.RunStateCreated, commonv1.RunState_RUN_STATE_CREATED},
		{domain.RunStateQueued, commonv1.RunState_RUN_STATE_QUEUED},
		{domain.RunStateAcquired, commonv1.RunState_RUN_STATE_ACQUIRED},
		{domain.RunStatePreparing, commonv1.RunState_RUN_STATE_PREPARING},
		{domain.RunStateModelRunning, commonv1.RunState_RUN_STATE_MODEL_RUNNING},
		{domain.RunStateToolPending, commonv1.RunState_RUN_STATE_TOOL_PENDING},
		{domain.RunStateAwaitingApproval, commonv1.RunState_RUN_STATE_AWAITING_APPROVAL},
		{domain.RunStateToolRunning, commonv1.RunState_RUN_STATE_TOOL_RUNNING},
		{domain.RunStateAwaitingModelResume, commonv1.RunState_RUN_STATE_AWAITING_MODEL_RESUME},
		{domain.RunStateFinalizing, commonv1.RunState_RUN_STATE_FINALIZING},
		{domain.RunStateCompleted, commonv1.RunState_RUN_STATE_COMPLETED},
		{domain.RunStateFailed, commonv1.RunState_RUN_STATE_FAILED},
		{domain.RunStateCancelled, commonv1.RunState_RUN_STATE_CANCELLED},
		{domain.RunStateTimedOut, commonv1.RunState_RUN_STATE_TIMED_OUT},
	}

	for _, p := range pairs {
		t.Run(string(p.domain), func(t *testing.T) {
			// domain → proto
			got := RunStateToProto(p.domain)
			if got != p.proto {
				t.Fatalf("RunStateToProto(%q) = %v, want %v", p.domain, got, p.proto)
			}
			// proto → domain
			back, err := ProtoToRunState(p.proto)
			if err != nil {
				t.Fatalf("ProtoToRunState(%v) unexpected error: %v", p.proto, err)
			}
			if back != p.domain {
				t.Fatalf("ProtoToRunState(%v) = %q, want %q", p.proto, back, p.domain)
			}
		})
	}
}

func TestRunStateStringToProto(t *testing.T) {
	got := RunStateStringToProto("model_running")
	if got != commonv1.RunState_RUN_STATE_MODEL_RUNNING {
		t.Fatalf("RunStateStringToProto(\"model_running\") = %v, want MODEL_RUNNING", got)
	}
}

func TestRunStateToProtoUnknownReturnsUnspecified(t *testing.T) {
	got := RunStateToProto(domain.RunState("bogus"))
	if got != commonv1.RunState_RUN_STATE_UNSPECIFIED {
		t.Fatalf("RunStateToProto(bogus) = %v, want UNSPECIFIED", got)
	}
}

func TestProtoToRunStateUnspecifiedReturnsError(t *testing.T) {
	_, err := ProtoToRunState(commonv1.RunState_RUN_STATE_UNSPECIFIED)
	if err == nil {
		t.Fatal("expected error for UNSPECIFIED RunState")
	}
}

func TestProtoToRunStateUnknownIntReturnsError(t *testing.T) {
	_, err := ProtoToRunState(commonv1.RunState(999))
	if err == nil {
		t.Fatal("expected error for unknown RunState int")
	}
}

// ---------------------------------------------------------------------------
// AutonomyMode round-trip
// ---------------------------------------------------------------------------

func TestAutonomyModeRoundTrip(t *testing.T) {
	pairs := []struct {
		domain domain.AutonomyMode
		proto  commonv1.AutonomyMode
	}{
		{domain.AutonomyMode0, commonv1.AutonomyMode_AUTONOMY_MODE_0},
		{domain.AutonomyMode1, commonv1.AutonomyMode_AUTONOMY_MODE_1},
		{domain.AutonomyMode2, commonv1.AutonomyMode_AUTONOMY_MODE_2},
		{domain.AutonomyMode3, commonv1.AutonomyMode_AUTONOMY_MODE_3},
	}

	for _, p := range pairs {
		t.Run(string(p.domain), func(t *testing.T) {
			got := AutonomyModeToProto(p.domain)
			if got != p.proto {
				t.Fatalf("AutonomyModeToProto(%q) = %v, want %v", p.domain, got, p.proto)
			}
			back, err := ProtoToAutonomyMode(p.proto)
			if err != nil {
				t.Fatalf("ProtoToAutonomyMode(%v) unexpected error: %v", p.proto, err)
			}
			if back != p.domain {
				t.Fatalf("ProtoToAutonomyMode(%v) = %q, want %q", p.proto, back, p.domain)
			}
		})
	}
}

func TestAutonomyModeStringToProto(t *testing.T) {
	got := AutonomyModeStringToProto("mode_2")
	if got != commonv1.AutonomyMode_AUTONOMY_MODE_2 {
		t.Fatalf("AutonomyModeStringToProto(\"mode_2\") = %v, want AUTONOMY_MODE_2", got)
	}
}

func TestProtoToAutonomyModeString(t *testing.T) {
	s, err := ProtoToAutonomyModeString(commonv1.AutonomyMode_AUTONOMY_MODE_1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "mode_1" {
		t.Fatalf("got %q, want \"mode_1\"", s)
	}
}

func TestAutonomyModeUnknownReturnsUnspecified(t *testing.T) {
	got := AutonomyModeToProto(domain.AutonomyMode("bogus"))
	if got != commonv1.AutonomyMode_AUTONOMY_MODE_UNSPECIFIED {
		t.Fatalf("AutonomyModeToProto(bogus) = %v, want UNSPECIFIED", got)
	}
}

func TestProtoToAutonomyModeUnspecifiedReturnsError(t *testing.T) {
	_, err := ProtoToAutonomyMode(commonv1.AutonomyMode_AUTONOMY_MODE_UNSPECIFIED)
	if err == nil {
		t.Fatal("expected error for UNSPECIFIED AutonomyMode")
	}
}

// ---------------------------------------------------------------------------
// ErrorClass round-trip
// ---------------------------------------------------------------------------

func TestErrorClassRoundTrip(t *testing.T) {
	pairs := []struct {
		domain domain.ErrorClass
		proto  commonv1.ErrorClass
	}{
		{domain.ErrorClassValidation, commonv1.ErrorClass_ERROR_CLASS_VALIDATION_ERROR},
		{domain.ErrorClassTransport, commonv1.ErrorClass_ERROR_CLASS_TRANSPORT_ERROR},
		{domain.ErrorClassTool, commonv1.ErrorClass_ERROR_CLASS_TOOL_ERROR},
		{domain.ErrorClassPolicy, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED},
		{domain.ErrorClassCredential, commonv1.ErrorClass_ERROR_CLASS_CREDENTIAL_ERROR},
		{domain.ErrorClassApproval, commonv1.ErrorClass_ERROR_CLASS_APPROVAL_ERROR},
		{domain.ErrorClassTimeout, commonv1.ErrorClass_ERROR_CLASS_TIMEOUT},
		{domain.ErrorClassCancelled, commonv1.ErrorClass_ERROR_CLASS_CANCELLED},
		{domain.ErrorClassInternal, commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR},
	}

	for _, p := range pairs {
		t.Run(string(p.domain), func(t *testing.T) {
			got := ErrorClassToProto(p.domain)
			if got != p.proto {
				t.Fatalf("ErrorClassToProto(%q) = %v, want %v", p.domain, got, p.proto)
			}
			back, err := ProtoToErrorClass(p.proto)
			if err != nil {
				t.Fatalf("ProtoToErrorClass(%v) unexpected error: %v", p.proto, err)
			}
			if back != p.domain {
				t.Fatalf("ProtoToErrorClass(%v) = %q, want %q", p.proto, back, p.domain)
			}
		})
	}
}

func TestErrorClassStringToProto(t *testing.T) {
	got := ErrorClassStringToProto("transport_error")
	if got != commonv1.ErrorClass_ERROR_CLASS_TRANSPORT_ERROR {
		t.Fatalf("ErrorClassStringToProto(\"transport_error\") = %v, want TRANSPORT_ERROR", got)
	}
}

func TestErrorClassStringToProtoEmptyReturnsUnspecified(t *testing.T) {
	got := ErrorClassStringToProto("")
	if got != commonv1.ErrorClass_ERROR_CLASS_UNSPECIFIED {
		t.Fatalf("ErrorClassStringToProto(\"\") = %v, want UNSPECIFIED", got)
	}
}

func TestErrorClassToProtoUnknownReturnsInternalError(t *testing.T) {
	got := ErrorClassToProto(domain.ErrorClass("bogus"))
	if got != commonv1.ErrorClass_ERROR_CLASS_INTERNAL_ERROR {
		t.Fatalf("ErrorClassToProto(bogus) = %v, want INTERNAL_ERROR", got)
	}
}

func TestProtoToErrorClassUnspecifiedReturnsError(t *testing.T) {
	_, err := ProtoToErrorClass(commonv1.ErrorClass_ERROR_CLASS_UNSPECIFIED)
	if err == nil {
		t.Fatal("expected error for UNSPECIFIED ErrorClass")
	}
}

func TestProtoToErrorClassString(t *testing.T) {
	s, err := ProtoToErrorClassString(commonv1.ErrorClass_ERROR_CLASS_TOOL_ERROR)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s != "tool_error" {
		t.Fatalf("got %q, want \"tool_error\"", s)
	}
}

// ---------------------------------------------------------------------------
// Coverage: all 14 RunState values are covered by the map
// ---------------------------------------------------------------------------

func TestRunStateMapsAreComplete(t *testing.T) {
	if len(runStateDomainToProto) != 14 {
		t.Fatalf("runStateDomainToProto has %d entries, want 14", len(runStateDomainToProto))
	}
	if len(runStateProtoToDomain) != 14 {
		t.Fatalf("runStateProtoToDomain has %d entries, want 14", len(runStateProtoToDomain))
	}
}

func TestAutonomyModeMapsAreComplete(t *testing.T) {
	if len(autonomyModeDomainToProto) != 4 {
		t.Fatalf("autonomyModeDomainToProto has %d entries, want 4", len(autonomyModeDomainToProto))
	}
	if len(autonomyModeProtoToDomain) != 4 {
		t.Fatalf("autonomyModeProtoToDomain has %d entries, want 4", len(autonomyModeProtoToDomain))
	}
}

func TestErrorClassMapsAreComplete(t *testing.T) {
	if len(errorClassDomainToProto) != 9 {
		t.Fatalf("errorClassDomainToProto has %d entries, want 9", len(errorClassDomainToProto))
	}
	if len(errorClassProtoToDomain) != 9 {
		t.Fatalf("errorClassProtoToDomain has %d entries, want 9", len(errorClassProtoToDomain))
	}
}
